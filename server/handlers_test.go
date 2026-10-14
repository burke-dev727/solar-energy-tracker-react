package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeSource struct {
	records []eiaRecord
}

func (f fakeSource) fetch(_ context.Context, start, _ string) ([]eiaRecord, error) {
	if f.records != nil {
		return f.records, nil
	}
	return []eiaRecord{
		{Period: start, FuelTypeID: "ALL", Generation: "100"},
		{Period: start, FuelTypeID: "COW", Generation: "25"},
	}, nil
}

func get(t *testing.T, records recordFetcher, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	newMux(eiaSource{records: records}, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
	return value
}

func TestTestRoute(t *testing.T) {
	rec := get(t, fakeSource{}, "/api/energy/test")

	if rec.Code != http.StatusOK || rec.Body.String() != "Energy API is working" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestTestMixRoute(t *testing.T) {
	rec := get(t, fakeSource{}, "/api/energy/mix/test")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if mix := decode[EnergyMix](t, rec); mix.NaturalGas != 15.4 {
		t.Fatalf("naturalGas = %v, want 15.4", mix.NaturalGas)
	}
}

func TestRealRouteAggregates(t *testing.T) {
	rec := get(t, fakeSource{}, "/api/energy/real/2024-01")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	mix := decode[EnergyMix](t, rec)
	if mix.Coal != 25 || mix.Other != 75 {
		t.Fatalf("coal = %v, other = %v; want 25, 75", mix.Coal, mix.Other)
	}
}

func TestRealRouteRejectsInvalidPeriod(t *testing.T) {
	rec := get(t, fakeSource{}, "/api/energy/real/2024-13")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != invalidPeriodMessage {
		t.Fatalf("body = %q", body)
	}
}

func TestRangeRouteGroupsAndSortsByPeriod(t *testing.T) {
	source := fakeSource{records: []eiaRecord{
		{Period: "2024-02", FuelTypeID: "ALL", Generation: "200"},
		{Period: "2024-02", FuelTypeID: "SUN", Generation: "50"},
		{Period: "2024-01", FuelTypeID: "ALL", Generation: "100"},
		{Period: "2024-01", FuelTypeID: "NG", Generation: "40"},
	}}

	rec := get(t, source, "/api/energy/range?start=2024-01&end=2024-02")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	mixes := decode[[]EnergyMix](t, rec)
	if len(mixes) != 2 || mixes[0].Period != "2024-01" || mixes[1].Period != "2024-02" {
		t.Fatalf("periods = %+v", mixes)
	}
	if mixes[0].NaturalGas != 40 || mixes[1].Solar != 25 {
		t.Fatalf("naturalGas = %v, solar = %v; want 40, 25", mixes[0].NaturalGas, mixes[1].Solar)
	}
}

func TestRangeRouteRejectsInvalidPeriod(t *testing.T) {
	rec := get(t, fakeSource{}, "/api/energy/range?start=2024-1&end=2024-02")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}
