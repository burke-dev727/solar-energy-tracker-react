package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	dbsql "github.com/databricks/databricks-sql-go"
)

const (
	defaultDatabricksSchema = "workspace.energy"
	databricksCacheTTL      = time.Hour
)

var schemaNamePattern = regexp.MustCompile(`^\w+(\.\w+)?$`)

type databricksConfig struct {
	host     string
	httpPath string
	token    string
	schema   string
}

func databricksConfigFromEnv() (databricksConfig, error) {
	host := strings.TrimPrefix(os.Getenv("DATABRICKS_HOST"), "https://")
	cfg := databricksConfig{
		host:     strings.TrimSuffix(host, "/"),
		httpPath: os.Getenv("DATABRICKS_HTTP_PATH"),
		token:    os.Getenv("DATABRICKS_TOKEN"),
		schema:   os.Getenv("DATABRICKS_SCHEMA"),
	}
	if cfg.schema == "" {
		cfg.schema = defaultDatabricksSchema
	}

	var missing []string
	for _, required := range []struct{ name, value string }{
		{"DATABRICKS_HOST", cfg.host},
		{"DATABRICKS_HTTP_PATH", cfg.httpPath},
		{"DATABRICKS_TOKEN", cfg.token},
	} {
		if required.value == "" {
			missing = append(missing, required.name)
		}
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("DATA_SOURCE=databricks requires %s", strings.Join(missing, ", "))
	}
	if !schemaNamePattern.MatchString(cfg.schema) {
		return cfg, fmt.Errorf("DATABRICKS_SCHEMA %q must look like catalog.schema", cfg.schema)
	}
	return cfg, nil
}

// databricksSource serves the gold tables built by the Databricks energy-etl job.
type databricksSource struct {
	mixes      *cache[[]EnergyMix]
	trends     *cache[[]TrendPoint]
	milestones *cache[[]Milestone]
	forecast   *cache[Forecast]
}

func newDatabricksSource(cfg databricksConfig) (*databricksSource, error) {
	connector, err := dbsql.NewConnector(
		dbsql.WithServerHostname(cfg.host),
		dbsql.WithPort(443),
		dbsql.WithHTTPPath(cfg.httpPath),
		dbsql.WithAccessToken(cfg.token),
	)
	if err != nil {
		return nil, fmt.Errorf("configuring Databricks connection: %w", err)
	}
	db := sql.OpenDB(connector)

	table := func(name string) string { return quoteIdentifier(cfg.schema + "." + name) }
	mixesQuery := fmt.Sprintf(
		"SELECT period, coal, natural_gas, nuclear, hydro, wind, solar, petroleum, other FROM %s ORDER BY period",
		table("gold_energy_mix_monthly"),
	)
	trendsQuery := fmt.Sprintf(
		"SELECT period, source, generation_twh, share, share_12mo_avg, yoy_change_pts FROM %s ORDER BY period, source",
		table("gold_energy_trends"),
	)
	milestonesQuery := fmt.Sprintf(
		"SELECT period, source, passed_source, share, description FROM %s ORDER BY period, source",
		table("gold_energy_milestones"),
	)
	forecastQuery := fmt.Sprintf(
		"SELECT vintage, period, source, generation_twh, share, share_12mo_avg, yoy_change_pts FROM %s ORDER BY period, source",
		table("gold_energy_forecast"),
	)

	return &databricksSource{
		mixes: &cache[[]EnergyMix]{
			ttl:  databricksCacheTTL,
			load: func(ctx context.Context) ([]EnergyMix, error) { return queryEnergyMixes(ctx, db, mixesQuery) },
		},
		trends: &cache[[]TrendPoint]{
			ttl:  databricksCacheTTL,
			load: func(ctx context.Context) ([]TrendPoint, error) { return queryTrends(ctx, db, trendsQuery) },
		},
		milestones: &cache[[]Milestone]{
			ttl:  databricksCacheTTL,
			load: func(ctx context.Context) ([]Milestone, error) { return queryMilestones(ctx, db, milestonesQuery) },
		},
		forecast: &cache[Forecast]{
			ttl:  databricksCacheTTL,
			load: func(ctx context.Context) (Forecast, error) { return queryForecast(ctx, db, forecastQuery) },
		},
	}, nil
}

func (s *databricksSource) energyMixes(ctx context.Context, start, end string) ([]EnergyMix, error) {
	mixes, err := s.mixes.get(ctx)
	if err != nil {
		return nil, err
	}
	return inPeriodRange(mixes, start, end, func(m EnergyMix) string { return m.Period }), nil
}

func (s *databricksSource) energyTrends(ctx context.Context, start, end string) ([]TrendPoint, error) {
	points, err := s.trends.get(ctx)
	if err != nil {
		return nil, err
	}
	return inPeriodRange(points, start, end, func(p TrendPoint) string { return p.Period }), nil
}

func (s *databricksSource) energyMilestones(ctx context.Context) ([]Milestone, error) {
	return s.milestones.get(ctx)
}

func (s *databricksSource) energyForecast(ctx context.Context) (Forecast, error) {
	return s.forecast.get(ctx)
}

// quoteIdentifier backtick-quotes each part of a dotted name made of word characters.
func quoteIdentifier(name string) string {
	parts := strings.Split(name, ".")
	for i, part := range parts {
		parts[i] = "`" + part + "`"
	}
	return strings.Join(parts, ".")
}

// queryRows runs query and calls scan for each row.
func queryRows(ctx context.Context, db *sql.DB, query string, scan func(*sql.Rows) error) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("querying Databricks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("reading Databricks row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading Databricks rows: %w", err)
	}
	return nil
}

func queryEnergyMixes(ctx context.Context, db *sql.DB, query string) ([]EnergyMix, error) {
	mixes := []EnergyMix{}
	err := queryRows(ctx, db, query, func(rows *sql.Rows) error {
		var m EnergyMix
		if err := rows.Scan(&m.Period, &m.Coal, &m.NaturalGas, &m.Nuclear, &m.Hydro, &m.Wind, &m.Solar, &m.Petroleum, &m.Other); err != nil {
			return err
		}
		mixes = append(mixes, m)
		return nil
	})
	return mixes, err
}

func queryTrends(ctx context.Context, db *sql.DB, query string) ([]TrendPoint, error) {
	var trendRows []trendRow
	err := queryRows(ctx, db, query, func(rows *sql.Rows) error {
		r, err := scanTrendRow(rows)
		trendRows = append(trendRows, r)
		return err
	})
	return groupTrendRows(trendRows), err
}

func queryForecast(ctx context.Context, db *sql.DB, query string) (Forecast, error) {
	var forecast Forecast
	var trendRows []trendRow
	err := queryRows(ctx, db, query, func(rows *sql.Rows) error {
		r, err := scanTrendRow(rows, &forecast.Vintage)
		trendRows = append(trendRows, r)
		return err
	})
	forecast.Points = groupTrendRows(trendRows)
	return forecast, err
}

// scanTrendRow reads period, source, generation, share, rolling share, and
// year-over-year change, after any leading columns scanned into dest.
func scanTrendRow(rows *sql.Rows, dest ...any) (trendRow, error) {
	var r trendRow
	var rolling, yoy sql.NullFloat64
	dest = append(dest, &r.period, &r.source, &r.trend.Generation, &r.trend.Share, &rolling, &yoy)
	if err := rows.Scan(dest...); err != nil {
		return r, err
	}
	r.trend.RollingShare = nullableFloat(rolling)
	r.trend.YoYChange = nullableFloat(yoy)
	return r, nil
}

func queryMilestones(ctx context.Context, db *sql.DB, query string) ([]Milestone, error) {
	milestones := []Milestone{}
	err := queryRows(ctx, db, query, func(rows *sql.Rows) error {
		var m Milestone
		if err := rows.Scan(&m.Period, &m.Source, &m.PassedSource, &m.Share, &m.Description); err != nil {
			return err
		}
		m.Source, m.PassedSource = apiSourceKey(m.Source), apiSourceKey(m.PassedSource)
		milestones = append(milestones, m)
		return nil
	})
	return milestones, err
}

func nullableFloat(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}
