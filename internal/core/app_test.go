package core

import (
	"testing"

	"post-go/internal/config"
)

func TestLoadConfigUsesPostNodeAlignedDefaults(t *testing.T) {
	clearConfigEnv(t)

	cfg := LoadConfig(config.Env{})

	if cfg.MaxContentKB != 1024 {
		t.Fatalf("expected max content default 1024KB, got %d", cfg.MaxContentKB)
	}
	if cfg.MaxFileMB != 3.5 {
		t.Fatalf("expected max file default 3.5MB, got %v", cfg.MaxFileMB)
	}
	if cfg.S3Region != "auto" {
		t.Fatalf("expected S3 region default auto, got %q", cfg.S3Region)
	}
	if cfg.S3ForcePathStyle {
		t.Fatalf("expected path-style disabled without S3 endpoint")
	}
}

func TestLoadConfigDefaultsS3PathStyleWhenEndpointIsConfigured(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("S3_ENDPOINT", " https://s3.example.com ")

	cfg := LoadConfig(config.Env{})

	if cfg.S3Endpoint != "https://s3.example.com" {
		t.Fatalf("expected trimmed S3 endpoint, got %q", cfg.S3Endpoint)
	}
	if !cfg.S3ForcePathStyle {
		t.Fatalf("expected path-style enabled when S3 endpoint is configured")
	}
	if !cfg.S3Config().ForcePathStyle {
		t.Fatalf("expected S3 config to carry path-style setting")
	}
}

func TestLoadConfigAllowsExplicitS3PathStyleOverride(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("S3_ENDPOINT", "https://s3.example.com")
	t.Setenv("S3_FORCE_PATH_STYLE", "false")

	cfg := LoadConfig(config.Env{})

	if cfg.S3ForcePathStyle {
		t.Fatalf("expected explicit false to disable path-style")
	}
}

func TestLoadConfigTreatsZeroLimitsAsDefaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("MAX_CONTENT_SIZE_KB", "0")
	t.Setenv("MAX_FILE_SIZE_MB", "0")

	cfg := LoadConfig(config.Env{})

	if cfg.MaxContentKB != 1024 {
		t.Fatalf("expected zero max content to use default 1024KB, got %d", cfg.MaxContentKB)
	}
	if cfg.MaxFileMB != 3.5 {
		t.Fatalf("expected zero max file to use default 3.5MB, got %v", cfg.MaxFileMB)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"SECRET_KEY",
		"LINKS_REDIS_URL",
		"MAX_CONTENT_SIZE_KB",
		"MAX_FILE_SIZE_MB",
		"BASE_DOMAIN",
		"S3_ENDPOINT",
		"S3_ACCESS_KEY_ID",
		"S3_SECRET_ACCESS_KEY",
		"S3_BUCKET_NAME",
		"S3_REGION",
		"S3_FORCE_PATH_STYLE",
	} {
		t.Setenv(key, "")
	}
}
