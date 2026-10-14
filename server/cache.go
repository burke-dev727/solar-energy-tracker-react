package main

import (
	"context"
	"log"
	"sync"
	"time"
)

const cacheLoadTimeout = 2 * time.Minute

// cache keeps the result of load in memory, reloading it once the TTL expires.
// If a reload fails, the previous value is served until the next attempt.
type cache[T any] struct {
	load func(ctx context.Context) (T, error)
	ttl  time.Duration

	mu       sync.Mutex
	value    T
	loadedAt time.Time
}

func (c *cache[T]) get(ctx context.Context) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.loadedAt.IsZero() && time.Since(c.loadedAt) < c.ttl {
		return c.value, nil
	}

	// Detached from the request's cancellation: a client disconnecting shouldn't
	// abort a query that may be waiting for the warehouse to start.
	loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheLoadTimeout)
	defer cancel()

	value, err := c.load(loadCtx)
	if err != nil {
		if c.loadedAt.IsZero() {
			return value, err
		}
		log.Printf("refreshing cached data failed, serving previous copy: %v", err)
		return c.value, nil
	}

	c.value, c.loadedAt = value, time.Now()
	return c.value, nil
}

// inPeriodRange returns the items whose YYYY-MM period is between start and end, inclusive.
func inPeriodRange[T any](items []T, start, end string, period func(T) string) []T {
	filtered := []T{}
	for _, item := range items {
		if p := period(item); p >= start && p <= end {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
