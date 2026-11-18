package tagging

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"firstrole/internal/models"
)

func geminiFixture(t *testing.T, finish string, usage any) map[string]any {
	t.Helper()
	tags, err := json.Marshal(valid())
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{"candidates": []any{map[string]any{"finishReason": finish, "content": map[string]any{"parts": []any{map[string]any{"thought": true, "text": "Reasoning is separate from JSON."}, map[string]any{"text": string(tags)}}}}}, "usageMetadata": usage}
}

func TestGeminiRequestAndCost(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" || r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" {
			t.Error("incorrect native authentication")
		}
		if writes.Load() != 1 {
			t.Error("attempt was not persisted before request")
		}
		var body struct {
			System   struct{ Parts []struct{ Text string } } `json:"systemInstruction"`
			Contents []struct {
				Role  string
				Parts []struct{ Text string }
			}
			Generation map[string]json.RawMessage `json:"generationConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.System.Parts) != 1 || body.System.Parts[0].Text != Prompt || len(body.Contents) != 1 || body.Contents[0].Role != "user" {
			t.Error("missing system instruction or user posting")
		}
		if len(body.Contents) == 1 && (len(body.Contents[0].Parts) != 1 || !json.Valid([]byte(body.Contents[0].Parts[0].Text))) {
			t.Error("posting must be JSON text")
		}
		if string(body.Generation["maxOutputTokens"]) != "2048" || string(body.Generation["responseMimeType"]) != `"application/json"` || string(body.Generation["thinkingConfig"]) != `{"thinkingLevel":"MINIMAL"}` || body.Generation["temperature"] != nil {
			t.Error("incorrect generation settings", body.Generation)
		}
		var schema struct {
			AdditionalProperties bool
			Required             []string
			Properties           map[string]any
		}
		if json.Unmarshal(body.Generation["responseJsonSchema"], &schema) != nil || schema.AdditionalProperties || len(schema.Required) != 7 || schema.Properties["role_evidence"] == nil {
			t.Error("missing strict tagging schema")
		}
		json.NewEncoder(w).Encode(geminiFixture(t, "STOP", map[string]int{"promptTokenCount": 100, "cachedContentTokenCount": 40, "candidatesTokenCount": 50, "thoughtsTokenCount": 25, "totalTokenCount": 175}))
	}))
	defer server.Close()
	in, out, cached := 0.3, 2.5, 0.03
	client := Client{Provider: "gemini", URL: server.URL, Model: "gemini-3.5-flash-lite", Key: "test-key", InputRate: &in, OutputRate: &out, CachedInputRate: &cached, HTTP: server.Client()}
	var row models.Usage
	tags, err := client.Tag(context.Background(), source(), "run", "job", "key", func(_ context.Context, u models.Usage) error {
		writes.Add(1)
		row = u
		return nil
	})
	if err != nil || tags.Role != "Software Engineering" || writes.Load() != 2 {
		t.Fatal(tags, err, writes.Load())
	}
	if row.Status != "ok" || row.FailureReason != "" || row.HTTPStatus != http.StatusOK || row.OutputTokens == nil || *row.OutputTokens != 75 || row.ThinkingTokens == nil || *row.ThinkingTokens != 25 || row.CachedInputTokens == nil || *row.CachedInputTokens != 40 || row.CostUSD == nil || math.Abs(*row.CostUSD-0.0002067) > 1e-12 {
		t.Fatalf("incorrect billed usage: %+v", row)
	}
}

func TestGeminiHTTPRejectionRecorded(t *testing.T) {
	const key = "private-test-key"
	const detail = "provider-private-detail"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": detail, "key": key})
	}))
	defer server.Close()
	client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: key}
	var writes []models.Usage
	_, err := client.Tag(context.Background(), source(), "run", "job", "key", func(_ context.Context, u models.Usage) error {
		writes = append(writes, u)
		return nil
	})
	if err == nil || calls != 1 || len(writes) != 2 {
		t.Fatal("rejection accepted, retried, or not persisted", err, calls, len(writes))
	}
	if writes[0].HTTPStatus != 0 || writes[1].HTTPStatus != http.StatusForbidden || writes[1].Status != "http_error" {
		t.Fatal("incorrect rejection status", writes)
	}
	for i, row := range writes {
		data, err := json.Marshal(row)
		if err != nil || strings.Contains(string(data), key) || strings.Contains(string(data), detail) {
			t.Fatal("usage retained private response content", i, err)
		}
		if i == 0 && strings.Contains(string(data), "http_status") {
			t.Fatal("unobserved response status was serialized")
		}
	}
}

func TestGeminiUnspecifiedLocationFallback(t *testing.T) {
	for _, tc := range []struct {
		name, status, evidence, roleEvidence string
		locations, cities                    []string
		wantStatus, wantEvidence, failure    string
	}{
		{name: "unspecified explanation", status: "unspecified", evidence: "No location supplied", cities: []string{}, wantStatus: "unspecified"},
		{name: "unsupported region", status: "region", evidence: "Unverified region", cities: []string{}, wantStatus: "unspecified"},
		{name: "region without evidence", status: "region", cities: []string{}, wantStatus: "unspecified"},
		{name: "already unspecified", status: "unspecified", cities: []string{}, wantStatus: "unspecified"},
		{name: "supported country", status: "region", evidence: "India", cities: []string{}, wantStatus: "region", wantEvidence: "India"},
		{name: "supported country code", status: "region", evidence: "IN", cities: []string{}, wantStatus: "region", wantEvidence: "IN"},
		{name: "explicit source location", status: "region", evidence: "Unverified region", locations: []string{"Gurugram"}, cities: []string{}, failure: "validation_location_evidence_absent"},
		{name: "unsupported city", status: "city", evidence: "Jaipur", cities: []string{"Jaipur"}, failure: "validation_location_evidence_absent"},
		{name: "unsupported remote", status: "remote", evidence: "Remote anywhere", cities: []string{}, failure: "validation_location_evidence_absent"},
		{name: "invalid status", status: "unknown", evidence: "Unverified region", cities: []string{}, failure: "validation_taxonomy"},
		{name: "invalid role evidence", status: "region", evidence: "Unverified region", roleEvidence: "Unverified role", cities: []string{}, failure: "validation_role_evidence_absent"},
		{name: "invalid city status", status: "region", evidence: "Unverified region", cities: []string{"Jaipur"}, failure: "validation_city_status"},
		{name: "null cities", status: "region", evidence: "Unverified region", failure: "tag_json_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := models.RawPosting{Title: "Hostess", Description: "Seat guests. No experience required.", Country: "IN", Locations: tc.locations}
			tags := models.Tags{Role: "Business and Operations", Cities: tc.cities, LocationStatus: tc.status, LocationEvidence: tc.evidence, RoleEvidence: p.Title, WorkMode: "unspecified", EmploymentType: "full-time"}
			if tc.roleEvidence != "" {
				tags.RoleEvidence = tc.roleEvidence
			}
			encoded, err := json.Marshal(tags)
			if err != nil {
				t.Fatal(err)
			}
			body := map[string]any{"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"parts": []any{map[string]string{"text": string(encoded)}}}}}, "usageMetadata": map[string]int{"promptTokenCount": 100, "candidatesTokenCount": 50, "totalTokenCount": 150}}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			rate := 1.0
			client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: "test", InputRate: &rate, OutputRate: &rate}
			var row models.Usage
			got, err := client.Tag(context.Background(), p, "run", "job", "key", func(_ context.Context, u models.Usage) error { row = u; return nil })
			if calls.Load() != 1 || row.HTTPStatus != http.StatusOK || row.InputTokens == nil || *row.InputTokens != 100 || row.OutputTokens == nil || *row.OutputTokens != 50 || row.CostUSD == nil || math.Abs(*row.CostUSD-0.00015) > 1e-12 {
				t.Fatalf("attempt or usage changed: %+v, calls %d", row, calls.Load())
			}
			if tc.failure != "" {
				if err == nil || row.Status != "invalid_output" || row.FailureReason != tc.failure {
					t.Fatalf("unsupported prediction accepted or reason changed: %+v, %v", row, err)
				}
				return
			}
			if err != nil || row.Status != "ok" || row.FailureReason != "" || got.LocationStatus != tc.wantStatus || got.LocationEvidence != tc.wantEvidence || len(got.Cities) != 0 || got.Role != tags.Role || got.RoleEvidence != tags.RoleEvidence {
				t.Fatalf("incorrect location fallback: %+v, %+v, %v", got, row, err)
			}
		})
	}
}

func TestGeminiOutputRejectedWithUsage(t *testing.T) {
	for _, kind := range []string{"truncated", "safety", "blocked", "no candidates", "multiple candidates", "empty", "thought only", "tool", "null text"} {
		t.Run(kind, func(t *testing.T) {
			body := geminiFixture(t, "STOP", map[string]int{"promptTokenCount": 100, "candidatesTokenCount": 50, "thoughtsTokenCount": 25, "totalTokenCount": 175})
			candidates := body["candidates"].([]any)
			candidate := candidates[0].(map[string]any)
			switch kind {
			case "truncated":
				candidate["finishReason"] = "MAX_TOKENS"
			case "safety":
				candidate["finishReason"] = "SAFETY"
			case "blocked":
				body["promptFeedback"] = map[string]string{"blockReason": "SAFETY"}
			case "no candidates":
				body["candidates"] = []any{}
			case "multiple candidates":
				body["candidates"] = append(candidates, candidate)
			default:
				part := map[string]any{"text": ""}
				switch kind {
				case "thought only":
					part = map[string]any{"thought": true, "text": "not an answer"}
				case "tool":
					part = map[string]any{"functionCall": map[string]any{"name": "unexpected"}}
				case "null text":
					part["text"] = nil
				}
				candidate["content"] = map[string]any{"parts": []any{part}}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { json.NewEncoder(w).Encode(body) }))
			defer server.Close()
			rate := 1.0
			client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: "test", InputRate: &rate, OutputRate: &rate}
			var row models.Usage
			_, err := client.Tag(context.Background(), source(), "run", "job", "key", func(_ context.Context, u models.Usage) error { row = u; return nil })
			if err == nil || row.Status != "invalid_output" || row.FailureReason != "response_invalid" || row.OutputTokens == nil || *row.OutputTokens != 75 || row.CostUSD == nil {
				t.Fatalf("response accepted or usage lost: %+v, %v", row, err)
			}
		})
	}
}

func TestGeminiUsageAccounting(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		invalid   bool
		costKnown bool
	}{
		{"missing", `null`, false, false},
		{"empty", `{}`, false, false},
		{"partial", `{"promptTokenCount":100}`, false, false},
		{"omitted zero components", `{"promptTokenCount":100,"candidatesTokenCount":50,"totalTokenCount":150}`, false, true},
		{"cache rate unknown", `{"promptTokenCount":100,"candidatesTokenCount":50,"cachedContentTokenCount":20}`, false, false},
		{"negative input", `{"promptTokenCount":-1,"candidatesTokenCount":50}`, true, false},
		{"negative output", `{"promptTokenCount":100,"candidatesTokenCount":-1}`, true, false},
		{"negative thinking", `{"promptTokenCount":100,"candidatesTokenCount":50,"thoughtsTokenCount":-1}`, true, false},
		{"negative cached", `{"promptTokenCount":100,"candidatesTokenCount":50,"cachedContentTokenCount":-1}`, true, false},
		{"excess cached", `{"promptTokenCount":100,"candidatesTokenCount":50,"cachedContentTokenCount":101}`, true, false},
		{"incorrect total", `{"promptTokenCount":100,"candidatesTokenCount":50,"thoughtsTokenCount":25,"totalTokenCount":150}`, true, false},
		{"string", `{"promptTokenCount":"100","candidatesTokenCount":50}`, true, false},
		{"fraction", `{"promptTokenCount":100.5,"candidatesTokenCount":50}`, true, false},
		{"null count", `{"promptTokenCount":100,"candidatesTokenCount":50,"thoughtsTokenCount":null}`, true, false},
		{"overflow", `{"promptTokenCount":100,"candidatesTokenCount":` + strconv.Itoa(math.MaxInt) + `,"thoughtsTokenCount":1}`, true, false},
		{"unexpected tools", `{"promptTokenCount":100,"candidatesTokenCount":50,"toolUsePromptTokenCount":2}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(geminiFixture(t, "STOP", json.RawMessage(tc.raw)))
			rate := 1.0
			u := models.Usage{InputUSDPerMillion: &rate, OutputUSDPerMillion: &rate}
			_, err := geminiResponse(body, &u)
			if errors.Is(err, errInvalidUsage) != tc.invalid {
				t.Fatal("incorrect usage validation", err)
			}
			if !tc.invalid && (usageCost(u) != nil) != tc.costKnown {
				t.Fatalf("incorrect cost availability: %+v", u)
			}
		})
	}
}

func TestGeminiMalformedUsageRecorded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(geminiFixture(t, "STOP", map[string]int{"promptTokenCount": 100, "candidatesTokenCount": 50, "thoughtsTokenCount": -1}))
	}))
	defer server.Close()
	client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: "test"}
	var row models.Usage
	_, err := client.Tag(context.Background(), source(), "run", "job", "key", func(_ context.Context, u models.Usage) error { row = u; return nil })
	if !errors.Is(err, errInvalidUsage) || row.Status != "invalid_usage" || row.FailureReason != "" || row.InputTokens != nil || row.OutputTokens != nil || row.CostUSD != nil {
		t.Fatalf("malformed usage was accepted: %+v %v", row, err)
	}
}

func TestGeminiFailureReasons(t *testing.T) {
	const key = "private-test-key"
	const detail = "provider-private-detail"
	for _, tc := range []struct{ kind, reason string }{
		{"response", "response_invalid"},
		{"tag json", "tag_json_invalid"},
		{"role evidence", "validation_role_evidence_absent"},
		{"location evidence", "validation_location_evidence_absent"},
		{"city evidence", "validation_city_evidence_missing"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			tags := valid()
			switch tc.kind {
			case "role evidence":
				tags.RoleEvidence = detail
			case "location evidence":
				tags.LocationEvidence = detail
			case "city evidence":
				tags.Cities = []string{"Mumbai"}
			}
			output, err := json.Marshal(tags)
			if err != nil {
				t.Fatal(err)
			}
			if tc.kind == "tag json" {
				output = append(output[:len(output)-1], []byte(`,"`+detail+`":"`+key+`"}`)...)
			}
			body := geminiFixture(t, "STOP", map[string]int{"promptTokenCount": 100, "candidatesTokenCount": 50, "totalTokenCount": 150})
			body["private"] = key
			body["candidates"].([]any)[0].(map[string]any)["content"] = map[string]any{"parts": []any{map[string]string{"text": string(output)}}}
			if tc.kind == "response" {
				body["candidates"] = detail
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: key}
			var writes []models.Usage
			_, err = client.Tag(context.Background(), source(), "run", "job", "key", func(_ context.Context, u models.Usage) error {
				writes = append(writes, u)
				return nil
			})
			if err == nil || calls != 1 || len(writes) != 2 {
				t.Fatal("invalid output accepted, retried, or not persisted", err, calls, len(writes))
			}
			row := writes[1]
			if writes[0].FailureReason != "" || row.Status != "invalid_output" || row.HTTPStatus != http.StatusOK || row.FailureReason != tc.reason {
				t.Fatal("incorrect failure category", writes)
			}
			if tc.kind != "response" && (row.InputTokens == nil || *row.InputTokens != 100 || row.OutputTokens == nil || *row.OutputTokens != 50) {
				t.Fatal("failure classification lost token usage", row)
			}
			data, err := json.Marshal(writes)
			if err != nil || strings.Contains(string(data), key) || strings.Contains(string(data), detail) {
				t.Fatal("usage retained raw response or credentials", err)
			}
		})
	}
	if validationFailureReason(errors.New(detail)) != "validation_failed" {
		t.Fatal("unknown validation error must use the safe fallback")
	}
}

func TestGeminiRefusesRedirects(t *testing.T) {
	var forwarded atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer server.Close()
	client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: "test-key", HTTP: server.Client()}
	_, err := client.Tag(context.Background(), source(), "run", "job", "key", func(context.Context, models.Usage) error { return nil })
	if err == nil || forwarded.Load() != 0 || client.HTTP.CheckRedirect != nil {
		t.Fatal("redirect followed or supplied client mutated", err, forwarded.Load())
	}
}

func TestGeminiRetryCancellation(t *testing.T) {
	for _, kind := range []string{"header", "native"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if kind == "header" {
					w.Header().Set("Retry-After", "5")
				}
				w.WriteHeader(http.StatusTooManyRequests)
				if kind == "native" {
					w.Write([]byte(`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"5s"}]}}`))
				}
			}))
			defer server.Close()
			client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: "test"}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			rows := 0
			_, err := client.Tag(ctx, source(), "run", "job", "key", func(context.Context, models.Usage) error { rows++; return nil })
			if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 || rows != 2 {
				t.Fatal("retry did not cancel without an extra attempt", err, calls.Load(), rows)
			}
			client.paceMu.Lock()
			remaining := time.Until(client.nextRequest)
			client.paceMu.Unlock()
			if remaining < 4*time.Second {
				t.Fatal("provider delay was not preserved", remaining)
			}
		})
	}
}
