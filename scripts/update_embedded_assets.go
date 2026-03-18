package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type assetManifestEntry struct {
	Key         string `json:"key"`
	RoutePath   string `json:"route_path"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SourceLink  string `json:"source_link"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "update embedded assets: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	manifestPath := filepath.Join("internal", "assets", "manifest.json")
	outputDir := filepath.Join("internal", "assets", "files")

	entries, err := loadManifest(manifestPath)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("manifest %s is empty", manifestPath)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	for _, entry := range entries {
		if entry.Key == "" || entry.FileName == "" || entry.SourceLink == "" {
			return fmt.Errorf("manifest entry must include key, file_name, and source_link: %+v", entry)
		}
		if err := downloadAsset(client, outputDir, entry); err != nil {
			return err
		}
	}

	return nil
}

func loadManifest(path string) ([]assetManifestEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var entries []assetManifestEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return entries, nil
}

func downloadAsset(client *http.Client, outputDir string, entry assetManifestEntry) error {
	response, err := client.Get(entry.SourceLink)
	if err != nil {
		return fmt.Errorf("download %s: %w", entry.Key, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %d", entry.Key, response.StatusCode)
	}

	destination := filepath.Join(outputDir, entry.FileName)
	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create %s: %w", destination, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, response.Body); err != nil {
		return fmt.Errorf("write %s: %w", destination, err)
	}

	fmt.Printf("updated %s -> %s\n", entry.Key, destination)
	return nil
}
