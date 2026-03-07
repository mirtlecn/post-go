package s3

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
}

func (c Config) IsConfigured() bool {
	return c.Endpoint != "" && c.AccessKeyID != "" && c.SecretAccessKey != "" && c.Bucket != ""
}

// Client wraps minio client and config.
type Client struct {
	mc   *minio.Client
	conf Config
}

// NewClient creates S3 client from config.
func NewClient(conf Config) (*Client, error) {
	if !conf.IsConfigured() {
		return nil, errors.New("S3 service is not configured")
	}
	secure := true
	endpoint := conf.Endpoint
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		u, err := url.Parse(endpoint)
		if err == nil {
			endpoint = u.Host
			secure = u.Scheme == "https"
		}
	}
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(conf.AccessKeyID, conf.SecretAccessKey, ""),
		Secure: secure,
		Region: conf.Region,
	})
	if err != nil {
		return nil, err
	}
	return &Client{mc: mc, conf: conf}, nil
}

// UploadFile streams file into S3 and returns object key.
func (c *Client) UploadFile(ctx context.Context, filename string, size int64, contentType string, r io.Reader, ttlSeconds int64) (string, error) {
	ext := strings.ToLower(path.Ext(filename))
	objKey := objectKeyPrefix(ttlSeconds) + randomUUID() + ext
	_, err := c.mc.PutObject(ctx, c.conf.Bucket, objKey, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}
	return objKey, nil
}

// GetObject fetches object as stream.
func (c *Client) GetObject(ctx context.Context, objectKey string) (*minio.Object, minio.ObjectInfo, error) {
	obj, err := c.mc.GetObject(ctx, c.conf.Bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, minio.ObjectInfo{}, err
	}
	info, err := obj.Stat()
	if err != nil {
		return nil, minio.ObjectInfo{}, err
	}
	return obj, info, nil
}

// DeleteObject removes object from S3.
func (c *Client) DeleteObject(ctx context.Context, objectKey string) error {
	return c.mc.RemoveObject(ctx, c.conf.Bucket, objectKey, minio.RemoveObjectOptions{})
}

func objectKeyPrefix(ttlSeconds int64) string {
	if ttlSeconds <= 0 {
		return "post/default/"
	}
	oneDay := int64(24 * time.Hour / time.Second)
	oneWeek := oneDay * 7
	oneMonth := oneDay * 30
	oneYear := oneDay * 365
	if ttlSeconds <= oneDay {
		return "post/tmp/1day/"
	}
	if ttlSeconds <= oneWeek {
		return "post/tmp/1week/"
	}
	if ttlSeconds <= oneMonth {
		return "post/tmp/1month/"
	}
	if ttlSeconds <= oneYear {
		return "post/tmp/1year/"
	}
	return "post/default/"
}

func randomUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
