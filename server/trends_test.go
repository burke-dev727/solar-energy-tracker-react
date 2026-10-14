package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeTrendSource struct {
	points     []TrendPoint
	milestones []Milestone
	forecast   Forecast
}

func (f fakeTrendSource) energyMixes(context.Context, string, string) ([]EnergyMix, error) {
	return []EnergyMix{}, nil
}

func (f fakeTrendSource) energyTrends(_ context.Context, start, end string) ([]TrendPoint, error) {
	return inPeriodRange(f.points, start, end, func(p TrendPoint) string { return p.Period }), nil
}

func (f fakeTrendSource) energyMilestones(context.Context) ([]Milestone, error) {
	return f.milestones, nil
}

func (f fakeTrendSource) energyForecast(context.Context) (Forecast, error) {
	return f.forecast, nil
}

func TestGroupTrendRows(t *testing.T) {
	rolling := 40.5
	rows := []trendRow{
		{period: "2024-01", source: "coal", trend: SourceTrend{Share: 20}},
		{period: "2024-01", source: "natural_gas", trend: SourceTrend{Share: 40, RollingShare: &rolling}},
		{period: "2024-02", source: "coal", trend: SourceTrend{Share: 18}},
	}

	points := groupTrendRows(rows)

	if len(points) != 2 || points[0].Period != "2024-01" || points[1].Period != "2024-02" {
		t.Fatalf("points = %+v", points)
	}
	gas, ok := points[0].Sources["naturalGas"]
	if !ok || gas.Share != 40 || gas.RollingShare == nil || *gas.RollingShare != 40.5 {
		t.Fatalf("naturalGas = %+v, present %v", gas, ok)
	}
	if len(points[1].Sources) != 1 {
		t.Fatalf("2024-02 sources = %+v", points[1].Sources)
	}
}

func TestTrendRoutesUnavailableForEIASource(t *testing.T) {
	for _, path := range []string{
		"/api/energy/trends?start=2024-01&end=2024-02",
		"/api/energy/milestones",
		"/api/energy/forecast",
	} {
		if rec := get(t, fakeSource{}, path); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestTrendRoutes(t *testing.T) {
	source := fakeTrendSource{
		points: []TrendPoint{
			{Period: "2024-01", Sources: map[string]SourceTrend{"solar": {Share: 4}}},
			{Period: "2024-02", Sources: map[string]SourceTrend{"solar": {Share: 5}}},
		},
		milestones: []Milestone{{Period: "2024-02", Source: "solar", PassedSource: "hydro", Description: "Solar passes hydro"}},
		forecast: Forecast{
			Vintage: "2024-02",
			Points:  []TrendPoint{{Period: "2024-03", Sources: map[string]SourceTrend{"solar": {Share: 6}}}},
		},
	}
	serve := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		newMux(source, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	rec := serve("/api/energy/trends?start=2024-02&end=2024-02")
	if rec.Code != http.StatusOK {
		t.Fatalf("trends status = %d", rec.Code)
	}
	if points := decode[[]TrendPoint](t, rec); len(points) != 1 || points[0].Sources["solar"].Share != 5 {
		t.Fatalf("points = %+v", points)
	}

	if rec := serve("/api/energy/trends?start=bad&end=2024-02"); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid trends range status = %d", rec.Code)
	}

	rec = serve("/api/energy/milestones")
	if rec.Code != http.StatusOK {
		t.Fatalf("milestones status = %d", rec.Code)
	}
	if milestones := decode[[]Milestone](t, rec); len(milestones) != 1 || milestones[0].PassedSource != "hydro" {
		t.Fatalf("milestones = %+v", milestones)
	}

	rec = serve("/api/energy/forecast")
	if rec.Code != http.StatusOK {
		t.Fatalf("forecast status = %d", rec.Code)
	}
	forecast := decode[Forecast](t, rec)
	if forecast.Vintage != "2024-02" || len(forecast.Points) != 1 || forecast.Points[0].Sources["solar"].Share != 6 {
		t.Fatalf("forecast = %+v", forecast)
	}
}
