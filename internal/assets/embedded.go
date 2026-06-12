package assets

import (
	"fmt"
	"path"

	gfmit "github.com/mirtlecn/gfm-it"
)

type md2HTMLAsset struct {
	Key         string `json:"key"`
	RoutePath   string `json:"route_path"`
	FileName    string `json:"file_name"`
	File        string `json:"file"`
	ContentType string `json:"content_type"`
	content     []byte
}

const assetRoutePrefix = "/asset/"

var (
	md2HTMLAssetsByKey   map[string]md2HTMLAsset
	md2HTMLAssetsByRoute map[string]md2HTMLAsset
	initErr              error
)

func init() {
	assetsByKey, assetsByRoute, err := loadMD2HTMLAssets()
	if err != nil {
		initErr = err
		return
	}
	md2HTMLAssetsByKey = assetsByKey
	md2HTMLAssetsByRoute = assetsByRoute
}

func InitError() error {
	return initErr
}

func loadMD2HTMLAssets() (map[string]md2HTMLAsset, map[string]md2HTMLAsset, error) {
	gfmAssets := gfmit.Assets()
	if len(gfmAssets) == 0 {
		return nil, nil, fmt.Errorf("gfm-it asset manifest is empty")
	}

	assetsByKey := make(map[string]md2HTMLAsset, len(gfmAssets))
	assetsByRoute := make(map[string]md2HTMLAsset, len(gfmAssets))
	for _, gfmAsset := range gfmAssets {
		if gfmAsset.Key == "" || gfmAsset.File == "" || gfmAsset.ContentType == "" {
			return nil, nil, fmt.Errorf("gfm-it asset manifest entry is incomplete: %+v", gfmAsset)
		}
		content, err := gfmit.ReadEmbeddedAssetContent(gfmAsset.Key)
		if err != nil {
			return nil, nil, fmt.Errorf("read gfm-it embedded asset %s: %w", gfmAsset.Key, err)
		}

		asset := md2HTMLAsset{
			Key:         gfmAsset.Key,
			RoutePath:   assetRoutePrefix + gfmAsset.Key,
			FileName:    path.Base(gfmAsset.File),
			File:        gfmAsset.File,
			ContentType: gfmAsset.ContentType,
			content:     content,
		}
		if _, exists := assetsByKey[asset.Key]; exists {
			return nil, nil, fmt.Errorf("duplicate embedded asset key: %s", asset.Key)
		}
		if _, exists := assetsByRoute[asset.RoutePath]; exists {
			return nil, nil, fmt.Errorf("duplicate embedded asset route: %s", asset.RoutePath)
		}
		if asset.RoutePath == assetRoutePrefix {
			return nil, nil, fmt.Errorf("asset manifest entry is incomplete: %+v", asset)
		}
		assetsByKey[asset.Key] = asset
		assetsByRoute[asset.RoutePath] = asset
	}

	return assetsByKey, assetsByRoute, nil
}

func MustAssetURL(key string) string {
	if initErr != nil {
		panic(fmt.Sprintf("embedded assets are not ready: %v; check github.com/mirtlecn/gfm-it", initErr))
	}
	asset, ok := md2HTMLAssetsByKey[key]
	if !ok {
		panic(fmt.Sprintf("embedded asset key not found: %s", key))
	}
	return asset.RoutePath
}

func LookupEmbeddedAsset(routePath string) ([]byte, string, bool) {
	if initErr != nil {
		return nil, "", false
	}
	asset, ok := md2HTMLAssetsByRoute[routePath]
	if !ok {
		return nil, "", false
	}
	return asset.content, asset.ContentType, true
}

func IsReservedEmbeddedAssetPath(routePath string) bool {
	if initErr != nil {
		return false
	}
	_, ok := md2HTMLAssetsByRoute[routePath]
	return ok
}
