package core

import (
	"context"

	"post-go/internal/config"
	"post-go/internal/s3"
)

// AppConfig holds runtime configuration.
type AppConfig struct {
	SecretKey        string
	RedisURL         string
	MaxContentKB     int
	MaxFileMB        int
	S3Endpoint       string
	S3AccessKeyID    string
	S3SecretAccess   string
	S3Bucket         string
	S3Region         string
	EnableFileUpload bool
}

// LoadConfig reads config from environment.
func LoadConfig(env config.Env) AppConfig {
	return AppConfig{
		SecretKey:        env.String("SECRET_KEY", ""),
		RedisURL:         env.String("LINKS_REDIS_URL", ""),
		MaxContentKB:     env.Int("MAX_CONTENT_SIZE_KB", 500),
		MaxFileMB:        env.Int("MAX_FILE_SIZE_MB", 10),
		S3Endpoint:       env.String("S3_ENDPOINT", ""),
		S3AccessKeyID:    env.String("S3_ACCESS_KEY_ID", ""),
		S3SecretAccess:   env.String("S3_SECRET_ACCESS_KEY", ""),
		S3Bucket:         env.String("S3_BUCKET_NAME", ""),
		S3Region:         env.String("S3_REGION", "auto"),
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
	}
}

// Context returns a background context for now.
func (c AppConfig) Context() context.Context { return context.Background() }
