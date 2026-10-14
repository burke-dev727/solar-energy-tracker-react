package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
)

// maxMonthlyValues caps months × sources returned by one get_monthly_data call.
const maxMonthlyValues = 480

var sourceKeys = []string{"coal", "naturalGas", "nuclear", "hydro", "wind", "solar", "petroleum", "other"}

// chatTools exposes the dashboard's data to the chat model as function tools.
type chatTools struct {
	data trendSource
}

type toolArgumentError struct{ message string }

func (e toolArgumentError) Error() string { return e.message }

func (t chatTools) definitions() []functionTool {
	sourcesParam := map[string]any{
		"type":        []string{"array", "null"},
		"items":       map[string]any{"type": "string", "enum": sourceKeys},
		"description": "Sources to include; null for all sources.",
	}
	tools := []functionTool{
		{
			Name: "get_monthly_data",
			Description: "Actual monthly U.S. electric power sector generation by source: generation (TWh), share of total (%), " +
				"12-month average share (%), and year-over-year change in share (percentage points). " +
				fmt.Sprintf("months × sources is limited to %d; use get_annual_summary for long-term trends.", maxMonthlyValues),
			Parameters: objectSchema(map[string]any{
				"start":   map[string]any{"type": "string", "description": "First month, YYYY-MM."},
				"end":     map[string]any{"type": "string", "description": "Last month, YYYY-MM."},
				"sources": sourcesParam,
			}),
		},
		{
			Name: "get_annual_summary",
			Description: "Calendar-year totals of actual generation by source (TWh) and each source's share of the year's total (%). " +
				"actualMonths shows how many months of the year have data, so the latest year may be partial.",
			Parameters: objectSchema(map[string]any{
				"start_year": map[string]any{"type": "integer"},
				"end_year":   map[string]any{"type": "integer"},
			}),
		},
		{
			Name: "get_eia_forecast",
			Description: "The U.S. EIA Short-Term Energy Outlook forecast for the months after the latest actual data, in the same " +
				"monthly format as get_monthly_data, plus calendar-year totals that combine actual and forecast months.",
			Parameters: objectSchema(map[string]any{"sources": sourcesParam}),
		},
		{
			Name:        "get_milestones",
			Description: "Months when one source's share first overtook another's and stayed ahead for at least 12 months.",
			Parameters:  objectSchema(map[string]any{}),
		},
	}
	for i := range tools {
		tools[i].Type, tools[i].Strict = "function", true
	}
	return tools
}

// objectSchema builds a strict-mode JSON schema in which every property is required.
func objectSchema(properties map[string]any) map[string]any {
	required := slices.Sorted(maps.Keys(properties))
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

// call runs the named tool and returns its JSON output. Failures are reported
// to the model as {"error": ...} so it can correct itself or tell the user.
func (t chatTools) call(ctx context.Context, name, arguments string) string {
	result, err := t.run(ctx, name, arguments)
	if err != nil {
		var argErr toolArgumentError
		message := err.Error()
		if !errors.As(err, &argErr) {
			log.Printf("chat tool %s failed: %v", name, err)
			message = "The data source is temporarily unavailable."
		}
		result = map[string]string{"error": message}
	}
	var output strings.Builder
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return `{"error":"failed to encode tool result"}`
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func (t chatTools) run(ctx context.Context, name, arguments string) (any, error) {
	switch name {
	case "get_monthly_data":
		var args struct {
			Start, End string
			Sources    []string
		}
		if err := decodeArguments(arguments, &args); err != nil {
			return nil, err
		}
		return t.monthlyData(ctx, args.Start, args.End, args.Sources)
	case "get_annual_summary":
		var args struct {
			StartYear int `json:"start_year"`
			EndYear   int `json:"end_year"`
		}
		if err := decodeArguments(arguments, &args); err != nil {
			return nil, err
		}
		return t.annualSummary(ctx, args.StartYear, args.EndYear)
	case "get_eia_forecast":
		var args struct{ Sources []string }
		if err := decodeArguments(arguments, &args); err != nil {
			return nil, err
		}
		return t.forecast(ctx, args.Sources)
	case "get_milestones":
		return t.data.energyMilestones(ctx)
	default:
		return nil, toolArgumentError{fmt.Sprintf("unknown tool %q", name)}
	}
}

func decodeArguments(arguments string, dest any) error {
	if err := json.Unmarshal([]byte(arguments), dest); err != nil {
		return toolArgumentError{"arguments must be a JSON object matching the tool schema"}
	}
	return nil
}

func validSources(sources []string) ([]string, error) {
	if len(sources) == 0 {
		return sourceKeys, nil
	}
	for _, source := range sources {
		if !slices.Contains(sourceKeys, source) {
			return nil, toolArgumentError{fmt.Sprintf("unknown source %q", source)}
		}
	}
	return sources, nil
}

func (t chatTools) monthlyData(ctx context.Context, start, end string, sources []string) (any, error) {
	if !validPeriod(start) || !validPeriod(end) || start > end {
		return nil, toolArgumentError{"start and end must be YYYY-MM with start <= end"}
	}
	sources, err := validSources(sources)
	if err != nil {
		return nil, err
	}
	points, err := t.data.energyTrends(ctx, start, end)
	if err != nil {
		return nil, err
	}
	if len(points)*len(sources) > maxMonthlyValues {
		return nil, toolArgumentError{fmt.Sprintf(
			"%d months × %d sources exceeds %d values; narrow the range or sources, or use get_annual_summary",
			len(points), len(sources), maxMonthlyValues,
		)}
	}
	return map[string]any{"months": monthlyRows(points, sources)}, nil
}

func (t chatTools) annualSummary(ctx context.Context, startYear, endYear int) (any, error) {
	if startYear > endYear {
		return nil, toolArgumentError{"start_year must be <= end_year"}
	}
	points, err := t.data.energyTrends(ctx, fmt.Sprintf("%04d-01", startYear), fmt.Sprintf("%04d-12", endYear))
	if err != nil {
		return nil, err
	}
	return map[string]any{"years": annualSummaries(points, nil, sourceKeys)}, nil
}

func (t chatTools) forecast(ctx context.Context, sources []string) (any, error) {
	sources, err := validSources(sources)
	if err != nil {
		return nil, err
	}
	forecast, err := t.data.energyForecast(ctx)
	if err != nil {
		return nil, err
	}
	if len(forecast.Points) == 0 {
		return map[string]any{"error": "no EIA forecast is available"}, nil
	}

	first, last := forecast.Points[0].Period, forecast.Points[len(forecast.Points)-1].Period
	actual, err := t.data.energyTrends(ctx, first[:4]+"-01", first)
	if err != nil {
		return nil, err
	}
	actual = slices.DeleteFunc(actual, func(p TrendPoint) bool { return p.Period >= first })

	return map[string]any{
		"source":         "U.S. EIA Short-Term Energy Outlook",
		"vintage":        forecast.Vintage,
		"forecastPeriod": first + " to " + last,
		"months":         monthlyRows(forecast.Points, sources),
		"annualTotals":   annualSummaries(actual, forecast.Points, sources),
	}, nil
}

type monthlySource struct {
	TWh          float64  `json:"twh"`
	Share        float64  `json:"share"`
	Share12moAvg *float64 `json:"share12moAvg,omitempty"`
	YoYChangePts *float64 `json:"yoyChangePts,omitempty"`
}

func monthlyRows(points []TrendPoint, sources []string) []map[string]any {
	rows := make([]map[string]any, 0, len(points))
	for _, point := range points {
		row := map[string]any{"period": point.Period}
		for _, source := range sources {
			trend, ok := point.Sources[source]
			if !ok {
				continue
			}
			row[source] = monthlySource{
				TWh:          round(trend.Generation, 2),
				Share:        round(trend.Share, 2),
				Share12moAvg: roundPtr(trend.RollingShare, 2),
				YoYChangePts: roundPtr(trend.YoYChange, 2),
			}
		}
		rows = append(rows, row)
	}
	return rows
}

type sourceTotal struct {
	TWh   float64 `json:"twh"`
	Share float64 `json:"share"`
}

type annualTotal struct {
	Year           int                    `json:"year"`
	ActualMonths   int                    `json:"actualMonths"`
	ForecastMonths int                    `json:"forecastMonths,omitempty"`
	TotalTWh       float64                `json:"totalTwh"`
	Sources        map[string]sourceTotal `json:"sources"`
}

// annualSummaries totals generation by calendar year, listing only the given
// sources but computing shares from all of them. Points in forecast are counted
// as forecast months; both slices must be sorted by period.
func annualSummaries(actual, forecast []TrendPoint, sources []string) []annualTotal {
	var years []annualTotal
	generation := map[int]map[string]float64{}

	add := func(point TrendPoint, isForecast bool) {
		year, err := strconv.Atoi(point.Period[:4])
		if err != nil {
			return
		}
		if len(years) == 0 || years[len(years)-1].Year != year {
			years = append(years, annualTotal{Year: year})
			generation[year] = map[string]float64{}
		}
		total := &years[len(years)-1]
		if isForecast {
			total.ForecastMonths++
		} else {
			total.ActualMonths++
		}
		for source, trend := range point.Sources {
			generation[year][source] += trend.Generation
		}
	}
	for _, point := range actual {
		add(point, false)
	}
	for _, point := range forecast {
		add(point, true)
	}

	for i := range years {
		bySource := generation[years[i].Year]
		var total float64
		for _, twh := range bySource {
			total += twh
		}
		years[i].TotalTWh = round(total, 1)
		years[i].Sources = map[string]sourceTotal{}
		for source, twh := range bySource {
			if !slices.Contains(sources, source) {
				continue
			}
			share := 0.0
			if total > 0 {
				share = twh / total * 100
			}
			years[i].Sources[source] = sourceTotal{TWh: round(twh, 1), Share: round(share, 2)}
		}
	}
	if years == nil {
		return []annualTotal{}
	}
	return years
}

// dataCoverage describes the data available to the chat model.
type dataCoverage struct {
	firstPeriod, lastActualPeriod string
	forecastVintage, forecastEnd  string
}

func (t chatTools) coverage(ctx context.Context) (dataCoverage, error) {
	var c dataCoverage
	points, err := t.data.energyTrends(ctx, "0000-01", "9999-12")
	if err != nil {
		return c, err
	}
	if len(points) > 0 {
		c.firstPeriod, c.lastActualPeriod = points[0].Period, points[len(points)-1].Period
	}
	forecast, err := t.data.energyForecast(ctx)
	if err != nil {
		return c, err
	}
	if len(forecast.Points) > 0 {
		c.forecastVintage, c.forecastEnd = forecast.Vintage, forecast.Points[len(forecast.Points)-1].Period
	}
	return c, nil
}

func round(value float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(value*scale) / scale
}

func roundPtr(value *float64, places int) *float64 {
	if value == nil {
		return nil
	}
	rounded := round(*value, places)
	return &rounded
}
