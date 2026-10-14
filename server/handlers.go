package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

const (
	invalidPeriodMessage     = "Period must be in YYYY-MM format"
	trendsUnavailableMessage = "Trends, milestones, and forecasts require DATA_SOURCE=databricks or snapshot"
)

// newMux builds the API routes. chat may be nil, which disables the chat assistant.
func newMux(source energySource, chat *chatAgent) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /api/energy/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("Energy API is working"))
	})

	mux.HandleFunc("GET /api/energy/mix/test", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, testEnergyMix)
	})

	mux.HandleFunc("GET /api/energy/real/{period}", func(w http.ResponseWriter, r *http.Request) {
		period := r.PathValue("period")
		if !validPeriod(period) {
			http.Error(w, invalidPeriodMessage, http.StatusBadRequest)
			return
		}

		mixes, err := source.energyMixes(r.Context(), period, period)
		if err != nil {
			writeUpstreamError(w, err)
			return
		}
		if len(mixes) == 0 {
			writeJSON(w, EnergyMix{Period: period})
			return
		}
		writeJSON(w, mixes[0])
	})

	mux.HandleFunc("GET /api/energy/range", func(w http.ResponseWriter, r *http.Request) {
		start, end, ok := periodRange(w, r)
		if !ok {
			return
		}

		mixes, err := source.energyMixes(r.Context(), start, end)
		if err != nil {
			writeUpstreamError(w, err)
			return
		}
		writeJSON(w, mixes)
	})

	trends, hasTrends := source.(trendSource)

	mux.HandleFunc("GET /api/energy/trends", func(w http.ResponseWriter, r *http.Request) {
		if !hasTrends {
			http.Error(w, trendsUnavailableMessage, http.StatusNotFound)
			return
		}
		start, end, ok := periodRange(w, r)
		if !ok {
			return
		}

		points, err := trends.energyTrends(r.Context(), start, end)
		if err != nil {
			writeUpstreamError(w, err)
			return
		}
		writeJSON(w, points)
	})

	mux.HandleFunc("GET /api/energy/milestones", func(w http.ResponseWriter, r *http.Request) {
		if !hasTrends {
			http.Error(w, trendsUnavailableMessage, http.StatusNotFound)
			return
		}

		milestones, err := trends.energyMilestones(r.Context())
		if err != nil {
			writeUpstreamError(w, err)
			return
		}
		writeJSON(w, milestones)
	})

	mux.HandleFunc("GET /api/energy/forecast", func(w http.ResponseWriter, r *http.Request) {
		if !hasTrends {
			http.Error(w, trendsUnavailableMessage, http.StatusNotFound)
			return
		}

		forecast, err := trends.energyForecast(r.Context())
		if err != nil {
			writeUpstreamError(w, err)
			return
		}
		writeJSON(w, forecast)
	})

	mux.HandleFunc("GET /api/chat/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"enabled": chat != nil})
	})

	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		if chat == nil {
			http.Error(w, chatUnavailableMessage, http.StatusServiceUnavailable)
			return
		}
		chat.handle(w, r)
	})

	return mux
}

// periodRange reads and validates the start and end query parameters,
// writing a 400 response if either is invalid.
func periodRange(w http.ResponseWriter, r *http.Request) (start, end string, ok bool) {
	start = r.URL.Query().Get("start")
	end = r.URL.Query().Get("end")
	if !validPeriod(start) || !validPeriod(end) {
		http.Error(w, invalidPeriodMessage, http.StatusBadRequest)
		return "", "", false
	}
	return start, end, true
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encoding response: %v", err)
	}
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	log.Printf("energy data request failed: %v", err)
	http.Error(w, "Failed to load energy data", http.StatusBadGateway)
}
