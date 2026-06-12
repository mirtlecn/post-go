package assets

import (
	"bytes"
	"testing"

	gfmit "github.com/mirtlecn/gfm-it"
)

func TestGFMItAssetsRegisteredAsInternalRoutes(t *testing.T) {
	for _, gfmAsset := range gfmit.Assets() {
		routePath := "/asset/" + gfmAsset.Key
		if got := MustAssetURL(gfmAsset.Key); got != routePath {
			t.Fatalf("MustAssetURL(%q) = %q, want %q", gfmAsset.Key, got, routePath)
		}
		if !IsReservedEmbeddedAssetPath(routePath) {
			t.Fatalf("%s should be reserved", routePath)
		}

		body, contentType, ok := LookupEmbeddedAsset(routePath)
		if !ok {
			t.Fatalf("LookupEmbeddedAsset(%q) did not find asset", routePath)
		}
		if contentType != gfmAsset.ContentType {
			t.Fatalf("%s content type = %q, want %q", routePath, contentType, gfmAsset.ContentType)
		}

		expectedBody, _, err := gfmit.ReadAsset(gfmAsset.Key)
		if err != nil {
			t.Fatalf("ReadAsset(%q) error = %v", gfmAsset.Key, err)
		}
		if !bytes.Equal(body, expectedBody) {
			t.Fatalf("%s body differs from gfm-it content", routePath)
		}
	}
}

func TestLegacyHashedAssetPathIsNotReserved(t *testing.T) {
	if IsReservedEmbeddedAssetPath("/asset/md-base-7f7c1c5a.css") {
		t.Fatal("legacy hashed base CSS path should not be reserved")
	}
}
