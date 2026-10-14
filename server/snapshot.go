package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const defaultSnapshotPath = "snapshot.json"

// Snapshot is a point-in-time copy of every dataset the API serves, so the
// deployed site doesn't depend on Databricks being awake.
type Snapshot struct {
	CreatedAt  time.Time    `json:"createdAt"`
	Mixes      []EnergyMix  `json:"mixes"`
	Trends     []TrendPoint `json:"trends"`
	Milestones []Milestone  `json:"milestones"`
	Forecast   Forecast     `json:"forecast"`
}

type snapshotSource struct {
	snapshot Snapshot
}

func loadSnapshot(path string) (*snapshotSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("decoding snapshot %s: %w", path, err)
	}
	if err := snapshot.validate(); err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", path, err)
	}
	return &snapshotSource{snapshot: snapshot}, nil
}

func (s Snapshot) validate() error {
	if len(s.Mixes) == 0 || len(s.Trends) == 0 {
		return errors.New("snapshot has no energy mixes or trends")
	}
	return nil
}

func (s *snapshotSource) energyMixes(_ context.Context, start, end string) ([]EnergyMix, error) {
	return inPeriodRange(s.snapshot.Mixes, start, end, func(m EnergyMix) string { return m.Period }), nil
}

func (s *snapshotSource) energyTrends(_ context.Context, start, end string) ([]TrendPoint, error) {
	return inPeriodRange(s.snapshot.Trends, start, end, func(p TrendPoint) string { return p.Period }), nil
}

func (s *snapshotSource) energyMilestones(context.Context) ([]Milestone, error) {
	return s.snapshot.Milestones, nil
}

func (s *snapshotSource) energyForecast(context.Context) (Forecast, error) {
	return s.snapshot.Forecast, nil
}

type fullSource interface {
	energySource
	trendSource
}

// exportSnapshot reads every dataset from source and writes it to path,
// replacing any existing file only once the new one is complete.
func exportSnapshot(ctx context.Context, source fullSource, path string, now time.Time) error {
	snapshot := Snapshot{CreatedAt: now.UTC()}
	var err error
	if snapshot.Mixes, err = source.energyMixes(ctx, "0000-01", "9999-12"); err != nil {
		return err
	}
	if snapshot.Trends, err = source.energyTrends(ctx, "0000-01", "9999-12"); err != nil {
		return err
	}
	if snapshot.Milestones, err = source.energyMilestones(ctx); err != nil {
		return err
	}
	if snapshot.Forecast, err = source.energyForecast(ctx); err != nil {
		return err
	}
	if err := snapshot.validate(); err != nil {
		return err
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encoding snapshot: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
