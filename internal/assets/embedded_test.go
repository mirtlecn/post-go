package assets

import (
	"bytes"
	"testing"

	gfmaddons "github.com/mirtlecn/gfm-addons"
)

func TestGFMAddonAssetsRegisteredAsInternalRoutes(t *testing.T) {
	for _, addonAsset := range gfmaddons.Assets() {
		routePath := "/asset/" + addonAsset.Key
		if got := MustAssetURL(addonAsset.Key); got != routePath {
			t.Fatalf("MustAssetURL(%q) = %q, want %q", addonAsset.Key, got, routePath)
		}
		if !IsReservedEmbeddedAssetPath(routePath) {
			t.Fatalf("%s should be reserved", routePath)
		}

		body, contentType, ok := LookupEmbeddedAsset(routePath)
		if !ok {
			t.Fatalf("LookupEmbeddedAsset(%q) did not find asset", routePath)
		}
		if contentType != addonAsset.ContentType {
			t.Fatalf("%s content type = %q, want %q", routePath, contentType, addonAsset.ContentType)
		}

		expectedBody, _, err := gfmaddons.ReadAsset(addonAsset.Key)
		if err != nil {
			t.Fatalf("ReadAsset(%q) error = %v", addonAsset.Key, err)
		}
		if !bytes.Equal(body, expectedBody) {
			t.Fatalf("%s body differs from gfm-addons content", routePath)
		}
	}
}

func TestLegacyHashedAssetPathIsNotReserved(t *testing.T) {
	if IsReservedEmbeddedAssetPath("/asset/md-base-7f7c1c5a.css") {
		t.Fatal("legacy hashed base CSS path should not be reserved")
	}
}
