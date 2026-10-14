package main

import (
	"context"
	"math"
	"regexp"
	"slices"
	"strconv"
)

type EnergyMix struct {
	Period     string  `json:"period"`
	Coal       float64 `json:"coal"`
	NaturalGas float64 `json:"naturalGas"`
	Nuclear    float64 `json:"nuclear"`
	Hydro      float64 `json:"hydro"`
	Wind       float64 `json:"wind"`
	Solar      float64 `json:"solar"`
	Petroleum  float64 `json:"petroleum"`
	Other      float64 `json:"other"`
}

// energySource returns monthly energy mixes for periods between start and end
// (inclusive, YYYY-MM), sorted by period.
type energySource interface {
	energyMixes(ctx context.Context, start, end string) ([]EnergyMix, error)
}

type recordFetcher interface {
	fetch(ctx context.Context, start, end string) ([]eiaRecord, error)
}

// eiaSource computes energy mixes on demand from raw EIA records.
type eiaSource struct {
	records recordFetcher
}

func (s eiaSource) energyMixes(ctx context.Context, start, end string) ([]EnergyMix, error) {
	records, err := s.records.fetch(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return buildEnergyMixes(records), nil
}

var periodPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

var fuelTypes = []string{"COW", "NG", "NUC", "HYC", "WND", "SUN", "PET"}

var testEnergyMix = EnergyMix{
	Period:     "2000-01",
	Coal:       51.2,
	NaturalGas: 15.4,
	Nuclear:    20.1,
	Hydro:      7.3,
	Wind:       0.1,
	Solar:      0.01,
	Petroleum:  3.0,
	Other:      2.89,
}

func validPeriod(period string) bool {
	return periodPattern.MatchString(period)
}

// generationByPeriod indexes records as period -> fuel type -> generation,
// keeping the first record seen for each pair.
func generationByPeriod(records []eiaRecord) map[string]map[string]float64 {
	byPeriod := make(map[string]map[string]float64)
	for _, r := range records {
		fuels, ok := byPeriod[r.Period]
		if !ok {
			fuels = make(map[string]float64)
			byPeriod[r.Period] = fuels
		}
		if _, seen := fuels[r.FuelTypeID]; seen {
			continue
		}
		generation, err := strconv.ParseFloat(r.Generation, 64)
		if err != nil {
			generation = 0
		}
		fuels[r.FuelTypeID] = generation
	}
	return byPeriod
}

func buildEnergyMix(fuels map[string]float64, period string) EnergyMix {
	total := fuels["ALL"]
	other := total
	for _, fuel := range fuelTypes {
		other -= fuels[fuel]
	}

	percentage := func(value float64) float64 {
		if total == 0 {
			return 0
		}
		return math.Round(value/total*100*1000) / 1000
	}

	return EnergyMix{
		Period:     period,
		Coal:       percentage(fuels["COW"]),
		NaturalGas: percentage(fuels["NG"]),
		Nuclear:    percentage(fuels["NUC"]),
		Hydro:      percentage(fuels["HYC"]),
		Wind:       percentage(fuels["WND"]),
		Solar:      percentage(fuels["SUN"]),
		Petroleum:  percentage(fuels["PET"]),
		Other:      percentage(other),
	}
}

func buildEnergyMixes(records []eiaRecord) []EnergyMix {
	byPeriod := generationByPeriod(records)

	periods := make([]string, 0, len(byPeriod))
	for period := range byPeriod {
		periods = append(periods, period)
	}
	slices.Sort(periods)

	mixes := make([]EnergyMix, 0, len(periods))
	for _, period := range periods {
		mixes = append(mixes, buildEnergyMix(byPeriod[period], period))
	}
	return mixes
}
