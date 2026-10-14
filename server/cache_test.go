package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCacheLoadsOnceWithinTTL(t *testing.T) {
	loads := 0
	c := &cache[int]{
		ttl: time.Hour,
		load: func(context.Context) (int, error) {
			loads++
			return 42, nil
		},
	}

	for range 2 {
		if value, err := c.get(context.Background()); err != nil || value != 42 {
			t.Fatalf("got %d, %v", value, err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}
}

func TestCacheServesStaleValueWhenReloadFails(t *testing.T) {
	fail := false
	c := &cache[int]{
		ttl: 0,
		load: func(context.Context) (int, error) {
			if fail {
				return 0, errors.New("warehouse unavailable")
			}
			return 42, nil
		},
	}

	if _, err := c.get(context.Background()); err != nil {
		t.Fatal(err)
	}
	fail = true
	if value, err := c.get(context.Background()); err != nil || value != 42 {
		t.Fatalf("got %d, %v; want cached 42", value, err)
	}
}

func TestCacheReturnsErrorWithoutCachedValue(t *testing.T) {
	c := &cache[int]{
		ttl: time.Hour,
		load: func(context.Context) (int, error) {
			return 0, errors.New("warehouse unavailable")
		},
	}

	if _, err := c.get(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestInPeriodRange(t *testing.T) {
	mixes := []EnergyMix{{Period: "2024-01"}, {Period: "2024-02"}, {Period: "2024-03"}}

	got := inPeriodRange(mixes, "2024-02", "2024-03", func(m EnergyMix) string { return m.Period })

	if len(got) != 2 || got[0].Period != "2024-02" || got[1].Period != "2024-03" {
		t.Fatalf("got %+v", got)
	}
}

func TestQuoteIdentifier(t *testing.T) {
	if got := quoteIdentifier("workspace.energy.gold_energy_trends"); got != "`workspace`.`energy`.`gold_energy_trends`" {
		t.Fatalf("got %s", got)
	}
	if schemaNamePattern.MatchString("workspace.energy; DROP TABLE x") {
		t.Fatal("pattern accepted an unsafe schema name")
	}
}
