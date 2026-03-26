package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"post-go/internal/storage"
	"post-go/internal/utils"

	"github.com/redis/go-redis/v9"
)

var errDeleteNotFound = errors.New("delete target not found")

type deleteItemResult struct {
	Path        string
	StoredValue storage.StoredValue
}

func deleteResultResponse(result deleteItemResult, isExport bool) DeleteResponse {
	return DeleteResponse{
		Deleted: result.Path,
		Type:    result.StoredValue.Type,
		Title:   result.StoredValue.Title,
		Created: responseCreatedValue(result.StoredValue.Created),
		Content: responseContent(result.StoredValue.Type, result.StoredValue.Content, isExport),
	}
}

func isDeleteValidationError(err error) bool {
	if err == nil {
		return false
	}
	switch err.Error() {
	case topicHomeManagedError, "topic does not exist", "`topic` and `path` must match", "`path` is required", "`path` must not be \"/\" when `topic` is provided", "`path` must not contain empty topic members":
		return true
	default:
		return false
	}
}

func buildBulkDeleteError(path string, err error) BulkDeleteError {
	switch {
	case err == nil:
		return BulkDeleteError{Path: path}
	case err == errDeleteNotFound:
		return BulkDeleteError{
			Path:    path,
			Code:    "not_found",
			Message: "path \"" + path + "\" not found",
		}
	case isDeleteValidationError(err):
		return BulkDeleteError{
			Path:    path,
			Code:    "invalid_request",
			Message: err.Error(),
		}
	default:
		message := "Internal server error"
		if err != nil {
			message = err.Error()
		}
		return BulkDeleteError{
			Path:    path,
			Code:    "internal",
			Message: message,
		}
	}
}

func (h *Handler) deleteItem(ctx context.Context, rdb redisStore, pathVal, topicVal string, typeInfo requestTypeInfo) (deleteItemResult, error) {
	pathVal, topicVal = normalizePathAndTopic(pathVal, topicVal)
	if typeInfo.InputType == topicType {
		return h.deleteTopic(ctx, rdb, pathVal)
	}
	resolvedPath, err := h.resolveTopicPath(ctx, rdb, topicVal, pathVal)
	if err != nil {
		return deleteItemResult{}, err
	}
	if resolvedPath.IsTopicItem {
		return h.deleteTopicItem(ctx, rdb, resolvedPath)
	}
	return h.deleteStandaloneItem(ctx, rdb, pathVal)
}

func (h *Handler) deleteTopic(ctx context.Context, rdb redisStore, topicName string) (deleteItemResult, error) {
	exists, err := h.topicExists(ctx, rdb, topicName)
	if err != nil {
		return deleteItemResult{}, err
	}
	if !exists {
		return deleteItemResult{}, errDeleteNotFound
	}
	storedValue, err := h.getTopicStoredValue(ctx, rdb, topicName)
	if err != nil {
		return deleteItemResult{}, err
	}
	count, err := countTopicItems(ctx, rdb, topicName)
	if err != nil {
		return deleteItemResult{}, err
	}
	if err := rdb.Del(ctx, storage.LinksPrefix+topicName, topicItemsKey(topicName)).Err(); err != nil {
		return deleteItemResult{}, err
	}
	return deleteItemResult{
		Path: topicName,
		StoredValue: storage.StoredValue{
			Type:    topicType,
			Title:   topicDisplayTitle(topicName, storedValue),
			Created: storedValue.Created,
			Content: topicCountString(count),
		},
	}, nil
}

func (h *Handler) deleteStandaloneItem(ctx context.Context, rdb redisStore, pathVal string) (deleteItemResult, error) {
	if exists, err := h.topicExists(ctx, rdb, pathVal); err == nil && exists {
		return deleteItemResult{}, errors.New(topicHomeManagedError)
	}
	key := storage.LinksPrefix + pathVal
	stored, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return deleteItemResult{}, errDeleteNotFound
	}
	if err != nil {
		return deleteItemResult{}, err
	}
	if err := rdb.Del(ctx, key).Err(); err != nil {
		return deleteItemResult{}, err
	}
	if err := h.deps.clearFileCache(ctx, rdb, pathVal); err != nil {
		requestLogger{}.Warnf("clear file cache failed: %s (%v)", pathVal, err)
	}
	return deleteItemResult{Path: pathVal, StoredValue: storage.ParseStoredValue(stored)}, nil
}

func (h *Handler) deleteTopicItem(ctx context.Context, rdb redisStore, resolvedPath resolvedTopicPath) (deleteItemResult, error) {
	fullPath := resolvedPath.FullPath
	key := storage.LinksPrefix + fullPath
	existingStoredValue, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return deleteItemResult{}, errDeleteNotFound
	}
	if err != nil {
		return deleteItemResult{}, err
	}
	existingTTL, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		return deleteItemResult{}, err
	}
	parsedValue := storage.ParseStoredValue(existingStoredValue)
	if err := rdb.Del(ctx, key).Err(); err != nil {
		return deleteItemResult{}, err
	}
	if err := h.deps.clearFileCache(ctx, rdb, fullPath); err != nil {
		requestLogger{}.Warnf("clear file cache failed: %s (%v)", fullPath, err)
	}
	if err := rdb.ZRem(ctx, topicItemsKey(resolvedPath.TopicName), resolvedPath.RelativePath).Err(); err != nil {
		if restoreErr := restoreStoredValueWithTTL(ctx, rdb, key, existingStoredValue, existingTTL); restoreErr != nil {
			requestLogger{}.Errorf("topic delete restore failed: %v", restoreErr)
		}
		return deleteItemResult{}, err
	}
	if err := h.syncTopicIndex(ctx, rdb, resolvedPath.TopicName); err != nil {
		if zaddErr := rdb.ZAdd(ctx, topicItemsKey(resolvedPath.TopicName), redis.Z{
			Score:  float64(time.Now().Unix()),
			Member: resolvedPath.RelativePath,
		}).Err(); zaddErr != nil {
			requestLogger{}.Errorf("topic delete rollback zadd failed: %v", zaddErr)
		}
		if restoreErr := restoreStoredValueWithTTL(ctx, rdb, key, existingStoredValue, existingTTL); restoreErr != nil {
			requestLogger{}.Errorf("topic delete restore failed: %v", restoreErr)
		}
		return deleteItemResult{}, err
	}
	return deleteItemResult{Path: fullPath, StoredValue: parsedValue}, nil
}

func writeDeleteResult(w http.ResponseWriter, r *http.Request, result deleteItemResult) {
	isExport := isExportRequest(r)
	utils.JSON(w, http.StatusOK, deleteResultResponse(result, isExport))
}

func (h *Handler) deleteFileObjectBestEffort(ctx context.Context, result deleteItemResult) {
	if result.StoredValue.Type != "file" {
		return
	}
	conf := h.Cfg.S3Config()
	if !conf.IsConfigured() {
		return
	}
	client, err := h.deps.newFileStore(conf)
	if err != nil {
		return
	}
	if err := client.DeleteObject(ctx, result.StoredValue.Content); err != nil {
		requestLogger{}.Errorf("s3 delete failed: %s (%v)", result.StoredValue.Content, err)
	}
}

func (h *Handler) deleteItemsByPrefix(ctx context.Context, rdb redisStore, prefix, topicVal string, typeInfo requestTypeInfo, isExport bool) (BulkDeleteResponse, error) {
	response := BulkDeleteResponse{
		Deleted: []DeleteResponse{},
		Errors:  []BulkDeleteError{},
	}
	if typeInfo.InputType == topicType {
		topicNames, err := scanTopicNamesByPrefix(ctx, rdb, prefix)
		if err != nil {
			return BulkDeleteResponse{}, err
		}
		for _, topicName := range topicNames {
			result, err := h.deleteTopic(ctx, rdb, topicName)
			if err != nil {
				if !isDeleteValidationError(err) && err != errDeleteNotFound {
					requestLogger{}.Errorf("wildcard topic delete failed: path=%s err=%v", topicName, err)
				}
				response.Errors = append(response.Errors, buildBulkDeleteError(topicName, err))
				continue
			}
			response.Deleted = append(response.Deleted, deleteResultResponse(result, isExport))
		}
		return response, nil
	}

	keys, err := scanAllKeys(ctx, rdb, storage.LinksPrefix+prefix+"*")
	if err != nil {
		return BulkDeleteResponse{}, err
	}
	storedValues, err := batchGetStoredValues(ctx, rdb, keys)
	if err != nil {
		return BulkDeleteResponse{}, err
	}
	for _, key := range keys {
		path := strings.TrimPrefix(key, storage.LinksPrefix)
		storedValue, exists := storedValues[key]
		if !exists || storedValue.Type == topicType {
			continue
		}
		result, err := h.deleteItem(ctx, rdb, path, topicVal, typeInfo)
		if err != nil {
			if !isDeleteValidationError(err) && err != errDeleteNotFound {
				requestLogger{}.Errorf("wildcard delete failed: path=%s topic=%s err=%v", path, topicVal, err)
			}
			response.Errors = append(response.Errors, buildBulkDeleteError(path, err))
			continue
		}
		h.deleteFileObjectBestEffort(ctx, result)
		response.Deleted = append(response.Deleted, deleteResultResponse(result, isExport))
	}
	return response, nil
}
