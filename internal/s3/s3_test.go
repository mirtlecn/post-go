package s3

import (
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestBucketLookupForConfigUsesPathStyleWhenForced(t *testing.T) {
	got := bucketLookupForConfig(Config{ForcePathStyle: true})

	if got != minio.BucketLookupPath {
		t.Fatalf("expected path-style bucket lookup, got %v", got)
	}
}

func TestBucketLookupForConfigKeepsAutoLookupByDefault(t *testing.T) {
	got := bucketLookupForConfig(Config{})

	if got != minio.BucketLookupAuto {
		t.Fatalf("expected auto bucket lookup, got %v", got)
	}
}
