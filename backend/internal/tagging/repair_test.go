package tagging

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"firstrole/internal/models"
)

func writeRepairResponse(t *testing.T, w http.ResponseWriter, provider string, tags models.Tags) {
	t.Helper()
	encoded, err := json.Marshal(tags)
	if err != nil {
		t.Fatal(err)
	}
	var body any
	if provider == "gemini" {
		body = map[string]any{"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"parts": []any{map[string]string{"text": string(encoded)}}}}}, "usageMetadata": map[string]int{"promptTokenCount": 100, "candidatesTokenCount": 50, "totalTokenCount": 150}}
	} else {
		body = map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(encoded)}}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 50}}
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Error(err)
	}
}

func repairRequestText(t *testing.T, r *http.Request, provider string) (string, string) {
	t.Helper()
	var body struct {
		SystemInstruction struct{ Parts []struct{ Text string } }
		Contents          []struct{ Parts []struct{ Text string } }
		Messages          []struct{ Role, Content string }
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if provider == "gemini" {
		return body.SystemInstruction.Parts[0].Text, body.Contents[0].Parts[0].Text
	}
	return body.Messages[0].Content, body.Messages[1].Content
}

func TestValidationRepairPreservesEvidenceAndUsage(t *testing.T) {
	for _, provider := range []string{"gemini", "compatible"} {
		for _, reason := range []string{"validation_role_evidence_absent", "validation_internship_type"} {
			t.Run(provider+"/"+reason, func(t *testing.T) {
				t.Parallel()
				p := source()
				p.EmploymentType = "Intern"
				bad, good := valid(), valid()
				if reason == "validation_role_evidence_absent" {
					bad.RoleEvidence = "untrusted-provider-text-451"
				} else {
					bad.EmploymentType = "apprenticeship"
				}
				calls := 0
				originalContent := ""
				rows := []models.Usage{}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					instruction, content := repairRequestText(t, r, provider)
					if calls == 1 {
						if instruction != Prompt {
							t.Error("base request changed")
						}
						originalContent = content
						writeRepairResponse(t, w, provider, bad)
						return
					}
					if content != originalContent || instruction != repairInstruction(reason) || strings.Contains(instruction+content, "untrusted-provider-text-451") {
						t.Error("repair changed source data or included uncontrolled feedback")
					}
					if len(rows) != 3 || rows[1].Status != "invalid_output" || rows[1].FailureReason != reason || rows[1].CostUSD == nil || rows[2].Status != "in_flight" {
						t.Error("rejected usage not persisted before repair")
					}
					writeRepairResponse(t, w, provider, good)
				}))
				defer server.Close()
				rate := 1.0
				client := Client{Provider: provider, URL: server.URL, Model: "test", Key: "test-key", HTTP: server.Client(), InputRate: &rate, OutputRate: &rate}
				key := client.CacheKey(p)
				got, err := client.Tag(context.Background(), p, "run", "job", key, func(_ context.Context, usage models.Usage) error { rows = append(rows, usage); return nil })
				if err != nil || !reflect.DeepEqual(got, good) {
					t.Fatal("valid repair rejected", got, err)
				}
				if calls != 2 || len(rows) != 4 || rows[3].Status != "ok" || rows[3].RepairPolicy != RepairPolicyVersion || rows[3].RepairReason != reason || rows[0].RepairPolicy != "" || rows[0].ID == rows[2].ID || client.CacheKey(p) != key {
					t.Fatal("repair provenance, cap or cache changed", calls, rows)
				}
				if math.Abs(*rows[1].CostUSD+*rows[3].CostUSD-0.0003) > 1e-12 {
					t.Fatal("failed attempt cost lost")
				}
			})
		}
	}
}

func TestInternshipRepairSchemaPreservesOtherConstraints(t *testing.T) {
	for _, provider := range []string{"gemini", "compatible"} {
		t.Run(provider, func(t *testing.T) {
			client := Client{Provider: provider, Model: "test"}
			schemaFrom := func(body []byte) map[string]any {
				var value map[string]any
				if err := json.Unmarshal(body, &value); err != nil {
					t.Fatal(err)
				}
				if provider == "gemini" {
					return value["generationConfig"].(map[string]any)["responseJsonSchema"].(map[string]any)
				}
				return value["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"].(map[string]any)
			}
			base, err := client.requestBody("unchanged original input")
			if err != nil {
				t.Fatal(err)
			}
			expected := schemaFrom(base)
			expected["properties"].(map[string]any)["employment_type"] = map[string]any{"type": "string", "enum": []any{"internship"}}
			repair, err := client.repairBody("unchanged original input", "validation_internship_type")
			if err != nil || !reflect.DeepEqual(schemaFrom(repair), expected) {
				t.Fatal("repair changed unrelated schema constraints", err)
			}
			other, err := client.repairBody("unchanged original input", "validation_role_evidence_absent")
			if err != nil || !reflect.DeepEqual(schemaFrom(other), schemaFrom(base)) {
				t.Fatal("other repairs restricted employment type", err)
			}
			after, err := client.requestBody("unchanged original input")
			if err != nil || string(after) != string(base) {
				t.Fatal("repair mutated base request", err)
			}
		})
	}
}

func TestValidationRepairSharesAttemptLimit(t *testing.T) {
	for _, httpRetry := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid only", true: "transport then invalid"}[httpRetry], func(t *testing.T) {
			t.Parallel()
			calls := 0
			bad := valid()
			bad.RoleEvidence = "unsupported role quote"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if calls == 1 && httpRetry {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				writeRepairResponse(t, w, "gemini", bad)
			}))
			defer server.Close()
			client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: "test"}
			rows := []models.Usage{}
			_, err := client.Tag(context.Background(), source(), "run", "job", "key", func(_ context.Context, usage models.Usage) error { rows = append(rows, usage); return nil })
			if err == nil || calls != 3 || len(rows) != 6 || rows[5].Status != "invalid_output" {
				t.Fatal("invalid evidence accepted or shared request cap exceeded", calls, err, rows)
			}
		})
	}
}

func TestValidationRepairStopsOnCancellationAndLedgerFailure(t *testing.T) {
	for _, mode := range []string{"cancel", "ledger"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			calls := 0
			bad := valid()
			bad.RoleEvidence = "unsupported role quote"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				writeRepairResponse(t, w, "gemini", bad)
			}))
			defer server.Close()
			client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: "test"}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := client.Tag(ctx, source(), "run", "job", "key", func(_ context.Context, usage models.Usage) error {
				if usage.Status == "invalid_output" {
					if mode == "ledger" {
						return io.ErrClosedPipe
					}
					cancel()
				}
				return nil
			})
			want := io.ErrClosedPipe
			if mode == "cancel" {
				want = context.Canceled
			}
			if !errors.Is(err, want) || calls != 1 {
				t.Fatal("repair continued after cancellation or failed ledger write", calls, err)
			}
		})
	}
}

func TestRepairFeedbackRejectsUnknownReasons(t *testing.T) {
	for _, reason := range []string{"response_invalid", "invalid_usage", "private-test-key", "validation_failed"} {
		if repairInstruction(reason) != "" {
			t.Fatal("uncontrolled repair reason accepted", reason)
		}
	}
}

func TestInvalidUsageStopsWithoutRepair(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		json.NewEncoder(w).Encode(geminiFixture(t, "STOP", map[string]int{"promptTokenCount": -1, "candidatesTokenCount": 50}))
	}))
	defer server.Close()
	client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: "test"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := client.Tag(ctx, source(), "run", "job", "key", func(context.Context, models.Usage) error { return nil })
	if !errors.Is(err, errInvalidUsage) || calls != 1 {
		t.Fatal("invalid usage retried", calls, err)
	}
}

func TestExhaustedRetriesPreserveSharedCooldown(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := Client{Provider: "gemini", URL: server.URL, Model: "test", Key: "test"}
	_, err := client.Tag(context.Background(), source(), "run", "job", "key", func(context.Context, models.Usage) error { return nil })
	client.paceMu.Lock()
	remaining := time.Until(client.nextRequest)
	client.paceMu.Unlock()
	if err == nil || calls != 3 || remaining < 2500*time.Millisecond {
		t.Fatal("final response lost shared provider cooldown", err, calls, remaining)
	}
}
