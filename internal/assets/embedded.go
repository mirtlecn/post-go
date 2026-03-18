package assets

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
)

type md2HTMLAsset struct {
	Key         string `json:"key"`
	RoutePath   string `json:"route_path"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SourceLink  string `json:"source_link"`
	content     []byte
}

//go:generate go run ../../scripts/update_embedded_assets.go

//go:embed manifest.json files/*
var embeddedAssetFS embed.FS

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
	data, err := embeddedAssetFS.ReadFile("manifest.json")
	if err != nil {
		return nil, nil, fmt.Errorf("read asset manifest: %w", err)
	}

	var manifest []md2HTMLAsset
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, nil, fmt.Errorf("parse asset manifest: %w", err)
	}

	assetsByKey := make(map[string]md2HTMLAsset, len(manifest))
	assetsByRoute := make(map[string]md2HTMLAsset, len(manifest))
	for _, asset := range manifest {
		if asset.Key == "" || asset.RoutePath == "" || asset.FileName == "" || asset.ContentType == "" {
			return nil, nil, fmt.Errorf("asset manifest entry is incomplete: %+v", asset)
		}
		content, err := embeddedAssetFS.ReadFile(path.Join("files", asset.FileName))
		if err != nil {
			return nil, nil, fmt.Errorf("read asset file %s: %w", asset.FileName, err)
		}
		asset.content = content
		assetsByKey[asset.Key] = asset
		assetsByRoute[asset.RoutePath] = asset
	}

	return assetsByKey, assetsByRoute, nil
}

func MustAssetURL(key string) string {
	if initErr != nil {
		panic(fmt.Sprintf("embedded assets are not ready: %v; run `go run ./scripts/update_embedded_assets.go`", initErr))
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
