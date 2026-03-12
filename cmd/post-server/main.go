package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"post-go/internal/httpapi"
)

func main() {
	loadEnv()

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	h := httpapi.NewHandler()
	if h.Cfg.SecretKey == "" || h.Cfg.RedisURL == "" {
		fmt.Println("Error: Missing required environment variables: LINKS_REDIS_URL, SECRET_KEY")
		fmt.Println("Please create a .env.local file. See .env.example for reference.")
		os.Exit(1)
	}

	addr := ":" + port
	log.Printf("env: PORT=%s LINKS_REDIS_URL=%s", port, h.Cfg.RedisURL)
	fmt.Printf("\n✅  Server running at http://localhost:%s\n", port)
	fmt.Print("    Press Ctrl+C to stop.\n\n")
	_ = http.ListenAndServe(addr, h)
}

// loadEnv loads .env.local then .env, without overriding existing values.
func loadEnv() {
	for _, file := range []string{".env.local", ".env"} {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			idx := strings.Index(line, "=")
			if idx < 0 {
				continue
			}
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, "\"'")
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
		fmt.Println("Loaded env from:", file)
		return
	}
}
