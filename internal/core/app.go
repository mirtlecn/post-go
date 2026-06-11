package core

import (
	"context"
	"strings"

	"post-go/internal/config"
	"post-go/internal/s3"
)

// AppConfig holds runtime configuration.
type AppConfig struct {
	SecretKey        string
	RedisURL         string
	MaxContentKB     int
	MaxFileMB        float64
	BaseDomain       string
	S3Endpoint       string
	S3AccessKeyID    string
	S3SecretAccess   string
	S3Bucket         string
	S3Region         string
	S3ForcePathStyle bool
	EnableFileUpload bool
}

// LoadConfig reads config from environment.
func LoadConfig(env config.Env) AppConfig {
	s3Endpoint := strings.TrimSpace(env.String("S3_ENDPOINT", ""))
	maxContentKB := env.Int("MAX_CONTENT_SIZE_KB", 1024)
	if maxContentKB == 0 {
		maxContentKB = 1024
	}
	maxFileMB := env.Float("MAX_FILE_SIZE_MB", 3.5)
	if maxFileMB == 0 {
		maxFileMB = 3.5
	}

	return AppConfig{
		SecretKey:        env.String("SECRET_KEY", ""),
		RedisURL:         env.String("LINKS_REDIS_URL", ""),
		MaxContentKB:     maxContentKB,
		MaxFileMB:        maxFileMB,
		BaseDomain:       env.String("BASE_DOMAIN", ""),
		S3Endpoint:       s3Endpoint,
		S3AccessKeyID:    env.String("S3_ACCESS_KEY_ID", ""),
		S3SecretAccess:   env.String("S3_SECRET_ACCESS_KEY", ""),
		S3Bucket:         env.String("S3_BUCKET_NAME", ""),
		S3Region:         env.String("S3_REGION", "auto"),
		S3ForcePathStyle: env.Bool("S3_FORCE_PATH_STYLE", s3Endpoint != ""),
		EnableFileUpload: true,
	}
}

// S3Config builds S3 config.
func (c AppConfig) S3Config() s3.Config {
	return s3.Config{
		Endpoint:        c.S3Endpoint,
		AccessKeyID:     c.S3AccessKeyID,
		SecretAccessKey: c.S3SecretAccess,
		Bucket:          c.S3Bucket,
		Region:          c.S3Region,
		ForcePathStyle:  c.S3ForcePathStyle,
	}
}

// Context returns a background context for now.
func (c AppConfig) Context() context.Context { return context.Background() }
