package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeFullSource struct {
	fakeTrendSource
	mixes []EnergyMix
}

func (f fakeFullSource) energyMixes(_ context.Context, start, end string) ([]EnergyMix, error) {
	return inPeriodRange(f.mixes, start, end, func(m EnergyMix) string { return m.Period }), nil
}

func TestSnapshotRoundTrip(t *testing.T) {
	source := fakeFullSource{
		fakeTrendSource: chatTestSource(),
		mixes:           []EnergyMix{{Period: "2025-12", Solar: 5}, {Period: "2026-01", Solar: 6}},
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	created := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)

	if err := exportSnapshot(context.Background(), source, path, created); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}

	if !loaded.snapshot.CreatedAt.Equal(created) {
		t.Fatalf("createdAt = %v", loaded.snapshot.CreatedAt)
	}
	mixes, _ := loaded.energyMixes(context.Background(), "2026-01", "2026-01")
	if len(mixes) != 1 || mixes[0].Solar != 6 {
		t.Fatalf("mixes = %+v", mixes)
	}
	forecast, _ := loaded.energyForecast(context.Background())
	if forecast.Vintage != "2026-02" || len(forecast.Points) != 1 {
		t.Fatalf("forecast = %+v", forecast)
	}

	rec := httptest.NewRecorder()
	newMux(loaded, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/energy/trends?start=2025-12&end=2026-01", nil))
	if points := decode[[]TrendPoint](t, rec); len(points) != 2 {
		t.Fatalf("trends from snapshot = %+v", points)
	}
}

func TestExportSnapshotRejectsEmptyData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	os.WriteFile(path, []byte("previous"), 0o644)

	if err := exportSnapshot(context.Background(), fakeFullSource{}, path, time.Now()); err == nil {
		t.Fatal("expected an error for an empty snapshot")
	}
	if data, _ := os.ReadFile(path); string(data) != "previous" {
		t.Fatalf("existing snapshot was overwritten: %q", data)
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/chat", nil)
	r.RemoteAddr = "10.0.0.5:5555"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")

	if got := clientIP(r, false); got != "10.0.0.5" {
		t.Fatalf("untrusted clientIP = %q", got)
	}
	if got := clientIP(r, true); got != "203.0.113.9" {
		t.Fatalf("trusted clientIP = %q", got)
	}
}

func TestStaticHandler(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644)
	os.Mkdir(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "assets", "app-123.js"), []byte("js"), 0o644)

	mux := newMux(fakeTrendSource{}, nil)
	mux.Handle("GET /", staticHandler(dir))
	serve := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	if rec := serve("/"); rec.Code != http.StatusOK || rec.Body.String() != "<html>app</html>" {
		t.Fatalf("/ = %d %q", rec.Code, rec.Body.String())
	}
	if rec := serve("/assets/app-123.js"); rec.Header().Get("Cache-Control") == "" {
		t.Fatal("assets should be cacheable")
	}
	if rec := serve("/healthz"); rec.Body.String() != "ok" {
		t.Fatalf("/healthz = %q", rec.Body.String())
	}
	if rec := serve("/api/energy/milestones"); rec.Code != http.StatusOK {
		t.Fatalf("API route shadowed by static files: %d", rec.Code)
	}
}
