package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const (
	eiaURL        = "https://api.eia.gov/v2/electricity/electric-power-operational-data/data/"
	eiaPageLength = 5000
)

type eiaRecord struct {
	Period              string `json:"period"`
	FuelTypeID          string `json:"fueltypeid"`
	FuelTypeDescription string `json:"fuelTypeDescription"`
	Generation          string `json:"generation"`
}

type eiaResponse struct {
	Response struct {
		Total string      `json:"total"`
		Data  []eiaRecord `json:"data"`
	} `json:"response"`
}

type eiaClient struct {
	apiKey string
	http   *http.Client
}

// fetch returns every EIA generation record between start and end (inclusive),
// following the API's pagination until all records are collected.
func (c *eiaClient) fetch(ctx context.Context, start, end string) ([]eiaRecord, error) {
	var records []eiaRecord
	for offset := 0; ; offset += eiaPageLength {
		page, err := c.getPage(ctx, start, end, offset)
		if err != nil {
			return nil, err
		}
		records = append(records, page.Response.Data...)

		total, err := strconv.Atoi(page.Response.Total)
		if err != nil {
			return nil, fmt.Errorf("parsing EIA total %q: %w", page.Response.Total, err)
		}
		if len(records) >= total || len(page.Response.Data) == 0 {
			return records, nil
		}
	}
}

func (c *eiaClient) getPage(ctx context.Context, start, end string, offset int) (*eiaResponse, error) {
	params := url.Values{
		"api_key":            {c.apiKey},
		"frequency":          {"monthly"},
		"data[]":             {"generation"},
		"facets[location][]": {"US"},
		"facets[sectorid][]": {"98"},
		"start":              {start},
		"end":                {end},
		"length":             {strconv.Itoa(eiaPageLength)},
		"offset":             {strconv.Itoa(offset)},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, eiaURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error embeds the request URL, which contains the API key.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("EIA request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("EIA API returned %s", resp.Status)
	}

	var page eiaResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decoding EIA response: %w", err)
	}
	return &page, nil
}
