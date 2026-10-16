package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := loadEnvFile(".env"); err != nil {
		log.Fatalf("loading .env: %v", err)
	}

	if len(os.Args) > 1 && os.Args[1] == "export-snapshot" {
		path := defaultSnapshotPath
		if len(os.Args) > 2 {
			path = os.Args[2]
		}
		if err := runExportSnapshot(path); err != nil {
			log.Fatalf("exporting snapshot: %v", err)
		}
		return
	}

	source, err := energySourceFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	mux := newMux(source, chatAgentFromEnv(source))
	if dir := os.Getenv("STATIC_DIR"); dir != "" {
		mux.Handle("GET /", staticHandler(dir))
		log.Printf("Serving frontend from %s", dir)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port
	log.Printf("Energy API listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// runExportSnapshot copies the Databricks gold tables into a snapshot file.
func runExportSnapshot(path string) error {
	cfg, err := databricksConfigFromEnv()
	if err != nil {
		return err
	}
	source, err := newDatabricksSource(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := exportSnapshot(ctx, source, path, time.Now()); err != nil {
		return err
	}
	log.Printf("Wrote snapshot of %s to %s", cfg.schema, path)
	return nil
}

// staticHandler serves the built frontend. Vite fingerprints files in
// /assets/, so they can be cached indefinitely. Paths with no file, such as
// /electricity, receive index.html so client-side routes work after a refresh.
func staticHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if !staticFileExists(dir, r.URL.Path) {
			if strings.HasPrefix(r.URL.Path, "/assets/") || path.Ext(r.URL.Path) != "" {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}

func staticFileExists(dir, urlPath string) bool {
	clean := path.Clean("/" + urlPath)
	if clean == "/" {
		return true
	}
	full := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	rel, err := filepath.Rel(dir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	info, err := os.Stat(full)
	return err == nil && !info.IsDir()
}

// chatAgentFromEnv returns the chat assistant, or nil if it isn't configured.
func chatAgentFromEnv(source energySource) *chatAgent {
	cfg, ok := openAIConfigFromEnv()
	trends, hasTrends := source.(trendSource)
	switch {
	case !ok:
		log.Println("Chat assistant disabled: OPENAI_API_KEY is not set")
		return nil
	case !hasTrends:
		log.Println("Chat assistant disabled: it requires DATA_SOURCE=databricks or snapshot")
		return nil
	}
	log.Printf("Chat assistant enabled with model %s", cfg.model)
	chat := newChatAgent(cfg, trends)
	chat.trustProxyHeaders = os.Getenv("TRUST_PROXY_HEADERS") == "true"
	return chat
}

func energySourceFromEnv() (energySource, error) {
	switch dataSource := os.Getenv("DATA_SOURCE"); dataSource {
	case "", "eia":
		apiKey := os.Getenv("EIA_API_KEY")
		if apiKey == "" {
			log.Println("warning: EIA_API_KEY is not set; requests to the EIA API will fail")
		}
		log.Println("Data source: EIA API (live)")
		return eiaSource{records: &eiaClient{
			apiKey: apiKey,
			http:   &http.Client{Timeout: 30 * time.Second},
		}}, nil

	case "databricks":
		cfg, err := databricksConfigFromEnv()
		if err != nil {
			return nil, err
		}
		source, err := newDatabricksSource(cfg)
		if err != nil {
			return nil, err
		}
		log.Printf("Data source: Databricks schema %s", cfg.schema)
		return source, nil

	case "snapshot":
		path := os.Getenv("SNAPSHOT_PATH")
		if path == "" {
			path = defaultSnapshotPath
		}
		source, err := loadSnapshot(path)
		if err != nil {
			return nil, err
		}
		log.Printf("Data source: snapshot %s created %s", path, source.snapshot.CreatedAt.Format(time.RFC3339))
		return source, nil

	default:
		return nil, fmt.Errorf(`unknown DATA_SOURCE %q (expected "eia", "databricks", or "snapshot")`, dataSource)
	}
}
