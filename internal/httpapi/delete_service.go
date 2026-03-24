package httpapi

import (
	"context"
	"errors"
	"net/http"
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
	utils.JSON(w, http.StatusOK, DeleteResponse{
		Deleted: result.Path,
		Type:    result.StoredValue.Type,
		Title:   result.StoredValue.Title,
		Created: responseCreatedValue(result.StoredValue.Created),
		Content: responseContent(result.StoredValue.Type, result.StoredValue.Content, isExport),
	})
}
