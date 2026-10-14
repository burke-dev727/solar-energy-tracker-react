package main

import "context"

type SourceTrend struct {
	Generation   float64  `json:"generation"`   // terawatt-hours
	Share        float64  `json:"share"`        // percent of total generation
	RollingShare *float64 `json:"rollingShare"` // 12-month average share; null for the first 11 months
	YoYChange    *float64 `json:"yoyChange"`    // percentage points vs. the same month a year earlier
}

type TrendPoint struct {
	Period  string                 `json:"period"`
	Sources map[string]SourceTrend `json:"sources"`
}

type Milestone struct {
	Period       string  `json:"period"`
	Source       string  `json:"source"`
	PassedSource string  `json:"passedSource"`
	Share        float64 `json:"share"`
	Description  string  `json:"description"`
}

// Forecast holds projected months following the latest actual data.
type Forecast struct {
	Vintage string       `json:"vintage"` // YYYY-MM the forecast was retrieved
	Points  []TrendPoint `json:"points"`
}

// trendSource is implemented by data sources that provide precomputed trends and forecasts.
type trendSource interface {
	energyTrends(ctx context.Context, start, end string) ([]TrendPoint, error)
	energyMilestones(ctx context.Context) ([]Milestone, error)
	energyForecast(ctx context.Context) (Forecast, error)
}

type trendRow struct {
	period string
	source string
	trend  SourceTrend
}

// apiSourceKey converts a gold table source name to the JSON key used by EnergyMix.
func apiSourceKey(source string) string {
	if source == "natural_gas" {
		return "naturalGas"
	}
	return source
}

// groupTrendRows folds rows sorted by period into one TrendPoint per period.
func groupTrendRows(rows []trendRow) []TrendPoint {
	points := []TrendPoint{}
	for _, row := range rows {
		if len(points) == 0 || points[len(points)-1].Period != row.period {
			points = append(points, TrendPoint{Period: row.period, Sources: map[string]SourceTrend{}})
		}
		points[len(points)-1].Sources[apiSourceKey(row.source)] = row.trend
	}
	return points
}
