package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestCurrentReturnsDefaultsWhenBuildInfoUnavailable(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	originalReadBuildInfo := readBuildInfo
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
		readBuildInfo = originalReadBuildInfo
	})

	Version = "dev"
	Commit = "unknown"
	BuildDate = "unknown"
	readBuildInfo = func() (info *debug.BuildInfo, ok bool) {
		return nil, false
	}

	info := Current()

	if info.Version != "dev" {
		t.Fatalf("expected default version, got %q", info.Version)
	}
	if info.Commit != "unknown" {
		t.Fatalf("expected default commit, got %q", info.Commit)
	}
	if info.BuildDate != "unknown" {
		t.Fatalf("expected default build date, got %q", info.BuildDate)
	}
}

func TestCurrentUsesRuntimeBuildMetadataWhenUnset(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	originalReadBuildInfo := readBuildInfo
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
		readBuildInfo = originalReadBuildInfo
	})

	Version = "dev"
	Commit = "unknown"
	BuildDate = "unknown"
	readBuildInfo = func() (info *debug.BuildInfo, ok bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "v1.2.3"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
				{Key: "vcs.time", Value: "2026-03-14T10:00:00Z"},
			},
		}, true
	}

	info := Current()

	if info.Version != "v1.2.3" {
		t.Fatalf("expected runtime version, got %q", info.Version)
	}
	if info.Commit != "abc123" {
		t.Fatalf("expected runtime commit, got %q", info.Commit)
	}
	if info.BuildDate != "2026-03-14T10:00:00Z" {
		t.Fatalf("expected runtime build date, got %q", info.BuildDate)
	}
}

func TestCurrentKeepsInjectedValues(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	originalReadBuildInfo := readBuildInfo
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
		readBuildInfo = originalReadBuildInfo
	})

	Version = "v9.9.9"
	Commit = "injected-commit"
	BuildDate = "2026-03-14T12:00:00Z"
	readBuildInfo = func() (info *debug.BuildInfo, ok bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "v1.2.3"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
				{Key: "vcs.time", Value: "2026-03-14T10:00:00Z"},
			},
		}, true
	}

	info := Current()

	if info.Version != "v9.9.9" {
		t.Fatalf("expected injected version, got %q", info.Version)
	}
	if info.Commit != "injected-commit" {
		t.Fatalf("expected injected commit, got %q", info.Commit)
	}
	if info.BuildDate != "2026-03-14T12:00:00Z" {
		t.Fatalf("expected injected build date, got %q", info.BuildDate)
	}
}
