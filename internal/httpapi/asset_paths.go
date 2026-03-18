package httpapi

import (
	"fmt"

	"post-go/internal/assets"
)

func isReservedAssetPath(path string) bool {
	return assets.IsReservedEmbeddedAssetPath("/" + path)
}

func reservedAssetPathError(path string) error {
	return fmt.Errorf("path %q is reserved for built-in assets", path)
}
