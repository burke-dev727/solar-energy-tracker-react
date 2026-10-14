package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeModel replays one scripted list of events per model request.
type fakeModel struct {
	rounds   [][]responseEvent
	requests []responsesRequest
}

func (f *fakeModel) streamResponse(_ context.Context, req responsesRequest, onEvent func(responseEvent) error) error {
	f.requests = append(f.requests, req)
	for _, event := range f.rounds[len(f.requests)-1] {
		if err := onEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func completed(id string) responseEvent {
	return responseEvent{Type: "response.completed", Response: &responseObject{ID: id}}
}

func chatTestSource() fakeTrendSource {
	return fakeTrendSource{
		points: []TrendPoint{
			{Period: "2025-12", Sources: map[string]SourceTrend{"solar": {Generation: 10, Share: 5}, "coal": {Generation: 190, Share: 95}}},
			{Period: "2026-01", Sources: map[string]SourceTrend{"solar": {Generation: 12, Share: 6}, "coal": {Generation: 188, Share: 94}}},
		},
		forecast: Forecast{
			Vintage: "2026-02",
			Points: []TrendPoint{
				{Period: "2026-02", Sources: map[string]SourceTrend{"solar": {Generation: 20, Share: 10}, "coal": {Generation: 180, Share: 90}}},
			},
		},
	}
}

func testChatAgent(model *fakeModel) *chatAgent {
	return &chatAgent{
		model:           model,
		modelName:       "test-model",
		reasoningEffort: "low",
		tools:           chatTools{data: chatTestSource()},
		limiter:         newRateLimiter(100, time.Minute),
		now:             func() time.Time { return time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC) },
	}
}

func TestChatAgentRunsToolsAndStreamsText(t *testing.T) {
	model := &fakeModel{rounds: [][]responseEvent{
		{
			{Type: "response.output_item.done", Item: &functionCall{Type: "function_call", CallID: "call_1", Name: "get_eia_forecast", Arguments: `{"sources":["solar"]}`}},
			completed("resp_1"),
		},
		{
			{Type: "response.output_text.delta", Delta: "Solar reaches "},
			{Type: "response.output_text.delta", Delta: "10%."},
			completed("resp_2"),
		},
	}}

	var events []chatEvent
	req := chatRequest{
		Messages: []chatMessage{{Role: "user", Content: "What does EIA expect for solar?"}},
		View:     &chatView{Period: "2026-02", Forecast: true},
	}
	err := testChatAgent(model).run(context.Background(), req, func(e chatEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []chatEvent{{Type: "tool", Tool: "get_eia_forecast"}, {Type: "text", Delta: "Solar reaches "}, {Type: "text", Delta: "10%."}}
	if len(events) != len(want) {
		t.Fatalf("events = %+v", events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("event %d = %+v, want %+v", i, events[i], want[i])
		}
	}

	first := model.requests[0]
	if !strings.Contains(first.Instructions, "2025-12 through 2026-01") ||
		!strings.Contains(first.Instructions, "viewing 2026-02 (an EIA forecast month)") {
		t.Fatalf("instructions missing coverage or view:\n%s", first.Instructions)
	}
	if len(first.Tools) != 4 || first.PreviousResponseID != "" {
		t.Fatalf("first request: %d tools, previous %q", len(first.Tools), first.PreviousResponseID)
	}

	second := model.requests[1]
	if second.PreviousResponseID != "resp_1" || len(second.Input) != 1 {
		t.Fatalf("second request: previous %q, input %+v", second.PreviousResponseID, second.Input)
	}
	output, ok := second.Input[0].(functionCallOutput)
	if !ok || output.CallID != "call_1" || !strings.Contains(output.Output, `"vintage":"2026-02"`) {
		t.Fatalf("tool output = %+v", second.Input[0])
	}
	if strings.Contains(output.Output, `"coal"`) {
		t.Fatalf("tool output includes unrequested sources: %s", output.Output)
	}
}

func TestChatAgentStopsAfterMaxToolRounds(t *testing.T) {
	round := []responseEvent{
		{Type: "response.output_item.done", Item: &functionCall{Type: "function_call", CallID: "c", Name: "get_milestones", Arguments: "{}"}},
		completed("r"),
	}
	model := &fakeModel{}
	for range maxToolRounds {
		model.rounds = append(model.rounds, round)
	}

	req := chatRequest{Messages: []chatMessage{{Role: "user", Content: "hi"}}}
	if err := testChatAgent(model).run(context.Background(), req, func(chatEvent) error { return nil }); err == nil {
		t.Fatal("expected an error after too many tool rounds")
	}
}

func postChat(chat *chatAgent, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	newMux(chatTestSource(), chat).ServeHTTP(rec, req)
	return rec
}

func TestChatRouteStreamsEvents(t *testing.T) {
	model := &fakeModel{rounds: [][]responseEvent{{{Type: "response.output_text.delta", Delta: "Hello"}, completed("r")}}}

	rec := postChat(testChatAgent(model), `{"messages":[{"role":"user","content":"Hi"}]}`)

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, content type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	want := "data: {\"type\":\"text\",\"delta\":\"Hello\"}\n\ndata: {\"type\":\"done\"}\n\n"
	if rec.Body.String() != want {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestChatRouteRejectsInvalidRequests(t *testing.T) {
	long := strings.Repeat("a", maxUserMessageLength+1)
	for _, body := range []string{
		`not json`,
		`{"messages":[]}`,
		`{"messages":[{"role":"system","content":"x"}]}`,
		`{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}`,
		`{"messages":[{"role":"user","content":"   "}]}`,
		`{"messages":[{"role":"user","content":"` + long + `"}]}`,
		`{"messages":[{"role":"user","content":"hi"}],"view":{"period":"2026-13"}}`,
	} {
		if rec := postChat(testChatAgent(&fakeModel{}), body); rec.Code != http.StatusBadRequest {
			t.Errorf("%.60s: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestChatRouteRateLimits(t *testing.T) {
	chat := testChatAgent(&fakeModel{rounds: [][]responseEvent{{completed("r")}}})
	chat.limiter = newRateLimiter(1, time.Minute)
	body := `{"messages":[{"role":"user","content":"Hi"}]}`

	if rec := postChat(chat, body); rec.Code != http.StatusOK {
		t.Fatalf("first request status = %d", rec.Code)
	}
	if rec := postChat(chat, body); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", rec.Code)
	}
}

func TestChatDisabled(t *testing.T) {
	if rec := postChat(nil, `{"messages":[{"role":"user","content":"Hi"}]}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}

	for _, tc := range []struct {
		chat *chatAgent
		want bool
	}{{nil, false}, {testChatAgent(&fakeModel{}), true}} {
		rec := httptest.NewRecorder()
		newMux(chatTestSource(), tc.chat).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chat/status", nil))
		if status := decode[map[string]bool](t, rec); status["enabled"] != tc.want {
			t.Fatalf("status = %+v, want enabled %v", status, tc.want)
		}
	}
}

func TestRateLimiterWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := newRateLimiter(2, time.Minute)
	limiter.now = func() time.Time { return now }

	if !limiter.allow("a") || !limiter.allow("a") || limiter.allow("a") {
		t.Fatal("expected two requests allowed, then blocked")
	}
	if !limiter.allow("b") {
		t.Fatal("other clients should have their own limit")
	}
	now = now.Add(time.Minute)
	if !limiter.allow("a") {
		t.Fatal("limit should reset after the window")
	}
}

func TestOpenAIClientStreamsEvents(t *testing.T) {
	var gotAuth string
	var gotBody responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n" +
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_9\"}}\n\n"))
	}))
	defer server.Close()

	client := &openAIClient{apiKey: "sk-test", baseURL: server.URL + "/v1", http: server.Client()}
	var events []responseEvent
	err := client.streamResponse(context.Background(), responsesRequest{Model: "m", Input: []any{inputMessage{Role: "user", Content: "Hi"}}},
		func(e responseEvent) error {
			events = append(events, e)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sk-test" || !gotBody.Stream || gotBody.Model != "m" {
		t.Fatalf("auth %q, body %+v", gotAuth, gotBody)
	}
	if len(events) != 2 || events[0].Delta != "Hi" || events[1].Response == nil || events[1].Response.ID != "resp_9" {
		t.Fatalf("events = %+v", events)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Incorrect API key provided"}}`))
	}))
	defer failing.Close()
	client.baseURL = failing.URL
	err = client.streamResponse(context.Background(), responsesRequest{}, func(responseEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Incorrect API key") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadServerSentEvents(t *testing.T) {
	stream := "event: response.created\ndata: {\"a\":1}\n\n: comment\ndata: {\"b\":\ndata: 2}\n\ndata: [DONE]\n\ndata: {\"c\":3}"
	var got []string
	err := readServerSentEvents(strings.NewReader(stream), func(data []byte) error {
		got = append(got, string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`{"a":1}`, "{\"b\":\n2}", `{"c":3}`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestChatToolsAnnualTotalsCombineActualAndForecast(t *testing.T) {
	output := chatTools{data: chatTestSource()}.call(context.Background(), "get_eia_forecast", `{"sources":null}`)

	var result struct {
		AnnualTotals []annualTotal `json:"annualTotals"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decoding %s: %v", output, err)
	}
	if len(result.AnnualTotals) != 1 {
		t.Fatalf("annualTotals = %+v", result.AnnualTotals)
	}
	year := result.AnnualTotals[0]
	if year.Year != 2026 || year.ActualMonths != 1 || year.ForecastMonths != 1 || year.TotalTWh != 400 {
		t.Fatalf("2026 total = %+v", year)
	}
	if solar := year.Sources["solar"]; solar.TWh != 32 || solar.Share != 8 {
		t.Fatalf("solar = %+v", solar)
	}
}

func TestToolSchemasAreStrict(t *testing.T) {
	for _, tool := range (chatTools{}).definitions() {
		data, err := json.Marshal(tool.Parameters)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties           map[string]any `json:"properties"`
			Required             []string       `json:"required"`
			AdditionalProperties *bool          `json:"additionalProperties"`
		}
		json.Unmarshal(data, &schema)
		if schema.Required == nil || len(schema.Required) != len(schema.Properties) ||
			schema.AdditionalProperties == nil || *schema.AdditionalProperties {
			t.Errorf("%s schema is not strict: %s", tool.Name, data)
		}
	}
}

func TestChatToolsReportArgumentErrors(t *testing.T) {
	tools := chatTools{data: chatTestSource()}
	for _, tc := range []struct{ name, arguments, want string }{
		{"get_monthly_data", `{"start":"2026-02","end":"2026-01","sources":null}`, "start <= end"},
		{"get_monthly_data", `{"start":"2025-12","end":"2026-01","sources":["sunshine"]}`, "unknown source"},
		{"get_annual_summary", `{"start_year":2026,"end_year":2020}`, "start_year"},
		{"get_prices", `{}`, "unknown tool"},
	} {
		if output := tools.call(context.Background(), tc.name, tc.arguments); !strings.Contains(output, tc.want) {
			t.Errorf("%s(%s) = %s, want error containing %q", tc.name, tc.arguments, output, tc.want)
		}
	}

	output := tools.call(context.Background(), "get_monthly_data", `{"start":"2025-12","end":"2026-01","sources":["solar"]}`)
	if !strings.Contains(output, `"2026-01"`) || strings.Contains(output, "coal") {
		t.Fatalf("monthly data = %s", output)
	}
}
