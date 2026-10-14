package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultOpenAIModel   = "gpt-6.1-sol"
)

type openAIConfig struct {
	apiKey          string
	baseURL         string
	model           string
	reasoningEffort string
}

// openAIConfigFromEnv returns the OpenAI settings, or false when OPENAI_API_KEY is unset.
func openAIConfigFromEnv() (openAIConfig, bool) {
	cfg := openAIConfig{
		apiKey:          os.Getenv("OPENAI_API_KEY"),
		baseURL:         strings.TrimSuffix(os.Getenv("OPENAI_BASE_URL"), "/"),
		model:           os.Getenv("OPENAI_MODEL"),
		reasoningEffort: os.Getenv("OPENAI_REASONING_EFFORT"),
	}
	if cfg.baseURL == "" {
		cfg.baseURL = defaultOpenAIBaseURL
	}
	if cfg.model == "" {
		cfg.model = defaultOpenAIModel
	}
	if cfg.reasoningEffort == "" {
		cfg.reasoningEffort = "low"
	}
	return cfg, cfg.apiKey != ""
}

// responsesRequest is the subset of the Responses API request body this server uses.
type responsesRequest struct {
	Model              string         `json:"model"`
	Instructions       string         `json:"instructions,omitempty"`
	Input              []any          `json:"input"`
	Tools              []functionTool `json:"tools,omitempty"`
	PreviousResponseID string         `json:"previous_response_id,omitempty"`
	MaxOutputTokens    int            `json:"max_output_tokens,omitempty"`
	Reasoning          *reasoning     `json:"reasoning,omitempty"`
	Stream             bool           `json:"stream"`
}

type reasoning struct {
	Effort string `json:"effort"`
}

type inputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type functionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

type functionTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict"`
}

type functionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// responseEvent is one server-sent event from a streamed response.
type responseEvent struct {
	Type     string          `json:"type"`
	Delta    string          `json:"delta"`
	Item     *functionCall   `json:"item"`
	Response *responseObject `json:"response"`
	Message  string          `json:"message"`
}

type responseObject struct {
	ID    string `json:"id"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

// responseStreamer streams a model response, calling onEvent for each event in order.
type responseStreamer interface {
	streamResponse(ctx context.Context, req responsesRequest, onEvent func(responseEvent) error) error
}

type openAIClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func (c *openAIClient) streamResponse(ctx context.Context, req responsesRequest, onEvent func(responseEvent) error) error {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encoding OpenAI request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("calling OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiError struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&apiError)
		return fmt.Errorf("OpenAI returned %s: %s", resp.Status, apiError.Error.Message)
	}

	return readServerSentEvents(resp.Body, func(data []byte) error {
		var event responseEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("decoding OpenAI event: %w", err)
		}
		return onEvent(event)
	})
}

// readServerSentEvents calls onData with the data field of each event in a
// text/event-stream body.
func readServerSentEvents(r io.Reader, onData func([]byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)

	var data []byte
	dispatch := func() error {
		if len(data) == 0 || string(data) == "[DONE]" {
			data = nil
			return nil
		}
		err := onData(data)
		data = nil
		return err
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if value, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, bytes.TrimPrefix(value, []byte(" "))...)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading OpenAI stream: %w", err)
	}
	return dispatch()
}
