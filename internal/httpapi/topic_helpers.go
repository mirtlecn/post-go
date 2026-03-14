package httpapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"post-go/internal/storage"
	"post-go/internal/topic"

	"github.com/redis/go-redis/v9"
)

const topicType = "topic"
const topicPlaceholderMember = "__topic_placeholder__"

type requestTypeInfo struct {
	InputType string
	StoreType string
}

type resolvedTopicPath struct {
	IsTopicItem   bool
	TopicName     string
	RelativePath  string
	FullPath      string
	ExistingTopic bool
}

func normalizeTypeAlias(body map[string]any) (requestTypeInfo, error) {
	inputType, _ := storage.MustString(body, "type")
	convertVal, _ := storage.MustString(body, "convert")
	if inputType != "" && convertVal != "" && inputType != convertVal {
		return requestTypeInfo{}, errors.New("`type` and `convert` must match when both are provided")
	}
	if inputType == "" {
		inputType = convertVal
	}
	storeType := inputType
	switch inputType {
	case "md2html":
		storeType = "html"
	case "qrcode":
		storeType = "text"
	}
	return requestTypeInfo{InputType: inputType, StoreType: storeType}, nil
}

func topicItemsKey(topicName string) string {
	return "topic:" + topicName + ":items"
}

func topicNameFromItemsKey(key string) string {
	return strings.TrimSuffix(strings.TrimPrefix(key, "topic:"), ":items")
}

func (h *Handler) topicExists(ctx context.Context, rdb redisStore, topicName string) (bool, error) {
	if topicName == "" {
		return false, nil
	}
	stored, err := rdb.Get(ctx, storage.LinksPrefix+topicName).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return storage.ParseStoredValue(stored).Type == topicType, nil
}

func (h *Handler) resolveTopicPath(ctx context.Context, rdb redisStore, topicName, pathVal string) (resolvedTopicPath, error) {
	resolved := resolvedTopicPath{FullPath: pathVal}
	if pathVal == "" {
		return resolved, nil
	}
	if topicName != "" {
		exists, err := h.topicExists(ctx, rdb, topicName)
		if err != nil {
			return resolved, err
		}
		if !exists {
			return resolved, errors.New("topic does not exist")
		}
		if strings.Contains(pathVal, "/") {
			expectedPrefix := topicName + "/"
			if !strings.HasPrefix(pathVal, expectedPrefix) {
				return resolved, errors.New("`topic` and `path` must match")
			}
			pathVal = strings.TrimPrefix(pathVal, expectedPrefix)
		}
		return resolvedTopicPath{
			IsTopicItem:   true,
			TopicName:     topicName,
			RelativePath:  pathVal,
			FullPath:      topicName + "/" + pathVal,
			ExistingTopic: true,
		}, nil
	}
	parts := strings.SplitN(pathVal, "/", 2)
	if len(parts) == 2 {
		exists, err := h.topicExists(ctx, rdb, parts[0])
		if err != nil {
			return resolved, err
		}
		if exists {
			return resolvedTopicPath{
				IsTopicItem:   true,
				TopicName:     parts[0],
				RelativePath:  parts[1],
				FullPath:      pathVal,
				ExistingTopic: true,
			}, nil
		}
	}
	return resolved, nil
}

func (h *Handler) rebuildTopicIndex(ctx context.Context, rdb redisStore, topicName string) error {
	items, err := rdb.ZRevRangeWithScores(ctx, topicItemsKey(topicName), 0, -1).Result()
	if err != nil {
		return err
	}
	indexItems := make([]topic.Item, 0, len(items))
	for _, item := range items {
		member, ok := item.Member.(string)
		if !ok || member == "" {
			continue
		}
		if member == topicPlaceholderMember {
			continue
		}
		stored, err := rdb.Get(ctx, storage.LinksPrefix+topicName+"/"+member).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return err
		}
		storedValue := storage.ParseStoredValue(stored)
		indexItems = append(indexItems, topic.Item{
			Path:      member,
			FullPath:  topicName + "/" + member,
			Type:      storedValue.Type,
			Title:     storedValue.Title,
			UpdatedAt: time.Unix(int64(item.Score), 0),
		})
	}
	html, err := topic.RenderIndexHTML(topicName, indexItems)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, storage.LinksPrefix+topicName, storage.BuildStoredValue(storage.StoredValue{
		Type:    topicType,
		Content: html,
		Title:   topicName,
	}), 0).Err()
}

func ensureTopicItemsKey(ctx context.Context, rdb redisStore, topicName string) error {
	return rdb.ZAdd(ctx, topicItemsKey(topicName), redis.Z{
		Score:  0,
		Member: topicPlaceholderMember,
	}).Err()
}

func countTopicItems(ctx context.Context, rdb redisStore, topicName string) (int64, error) {
	count, err := rdb.ZCard(ctx, topicItemsKey(topicName)).Result()
	if err != nil {
		return 0, err
	}
	if count > 0 {
		count--
	}
	return count, nil
}

func (h *Handler) adoptTopicItems(ctx context.Context, rdb redisStore, topicName string) error {
	var cursor uint64
	now := float64(time.Now().Unix())
	pattern := storage.LinksPrefix + topicName + "/*"
	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		for _, key := range keys {
			fullPath := strings.TrimPrefix(key, storage.LinksPrefix)
			relativePath := strings.TrimPrefix(fullPath, topicName+"/")
			if relativePath == "" {
				continue
			}
			if err := rdb.ZAdd(ctx, topicItemsKey(topicName), redis.Z{
				Score:  now,
				Member: relativePath,
			}).Err(); err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}

func topicCountString(count int64) string {
	return itoa(int(count))
}

func topicContentPreview(path string, count int64, isExport bool) string {
	if isExport {
		return path
	}
	return topicCountString(count)
}
