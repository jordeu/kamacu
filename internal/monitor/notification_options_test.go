package monitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type notificationTransport func(*http.Request) (*http.Response, error)

func (f notificationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestNotificationGenerationLimitsDoNotChangeMonitoringDefaults(t *testing.T) {
	for _, backend := range []string{"anthropic_api", "openai_compatible"} {
		t.Run(backend, func(t *testing.T) {
			observed := []map[string]any{}
			b := Backend{Client: &http.Client{Transport: notificationTransport(func(r *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				observed = append(observed, body)
				response := `{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`
				if backend == "anthropic_api" {
					response = `{"content":[{"type":"text","text":"{\"ok\":true}"}]}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
			})}}
			profile := Profile{ID: 1, Backend: backend, Model: "chosen-model", Endpoint: "https://model.invalid"}
			for _, ctx := range []context.Context{context.Background(), WithGenerationOptions(context.Background(), GenerationOptions{OutputTokens: 16000, Timeout: 300 * time.Second}), context.Background()} {
				if _, err := b.GenerateValidatedJSON(ctx, profile, "Return JSON", map[string]any{}, nil); err != nil {
					t.Fatal(err)
				}
			}
			if backend == "anthropic_api" {
				for i, want := range []float64{4096, 16000, 4096} {
					if observed[i]["max_tokens"] != want {
						t.Fatal(observed)
					}
				}
			} else {
				if observed[0]["max_completion_tokens"] != nil || observed[1]["max_completion_tokens"] != float64(16000) || observed[2]["max_completion_tokens"] != nil {
					t.Fatal(observed)
				}
			}
		})
	}
}
