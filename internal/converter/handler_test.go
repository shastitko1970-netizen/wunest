package converter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// M51 Sprint 3 wave 3 — integration smoke tests for the LLM-call wiring.
//
// We're not testing the LLM itself — that'd be a fool's errand. We test
// that callLLM correctly:
//   - sends a streaming request
//   - parses an OpenAI-format SSE transcript
//   - accumulates `choices[].delta.content` deltas into one string
//   - reads `usage` token counters
//   - terminates on `[DONE]`
//
// Mock upstream is an `httptest.Server` that returns a canned SSE
// response. The Handler is built with just `WuApi` populated — other
// fields (Repo, Users, etc.) aren't touched by callLLM directly.

// fakeSSE writes an OpenAI-shaped SSE response with the given content
// chunks and final usage block. Mirrors what Anthropic/OpenAI/Gemini
// emit for /v1/chat/completions when stream=true + include_usage=true.
func fakeSSE(w http.ResponseWriter, deltas []string, promptToks, completionToks int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	for _, d := range deltas {
		// Each delta event encodes one fragment of content. Real
		// providers stream JSON-escaped strings; for the test we use
		// plain ASCII to keep the fixtures readable.
		fmt.Fprintf(w, `data: {"choices":[{"delta":{"content":%q}}]}`+"\n\n", d)
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Final usage event (some providers send this in a separate frame
	// after content is done, others inline it on the last delta).
	fmt.Fprintf(w, `data: {"choices":[],"usage":{"prompt_tokens":%d,"completion_tokens":%d}}`+"\n\n",
		promptToks, completionToks)
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func TestCallLLM_AccumulatesContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the wire — POST + bearer + JSON body — without
		// asserting the entire request shape (that's the WuApi
		// client's responsibility, not ours).
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing Bearer auth header")
		}
		if !strings.Contains(r.URL.Path, "/v1/chat/completions") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		fakeSSE(w, []string{
			`{"name":"Test"`,
			`,"custom_css":""`,
			`}`,
		}, 100, 50)
	}))
	defer srv.Close()

	h := &Handler{
		WuApi: wuapi.NewClient(wuapi.Config{
			BaseURL: srv.URL,
			Timeout: 5 * time.Second,
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	got, usage, err := h.callLLM(ctx, callUpstream{APIKey: "fake-token"}, wuapi.ChatCompletionRequest{
		Model: "test/model",
		Messages: []map[string]any{
			{"role": "user", "content": "convert this"},
		},
	})
	if err != nil {
		t.Fatalf("callLLM error: %v", err)
	}
	want := `{"name":"Test","custom_css":""}`
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if usage.In != 100 {
		t.Fatalf("expected prompt_tokens=100, got %d", usage.In)
	}
	if usage.Out != 50 {
		t.Fatalf("expected completion_tokens=50, got %d", usage.Out)
	}
}

func TestCallLLM_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream model unavailable")
	}))
	defer srv.Close()

	h := &Handler{
		WuApi: wuapi.NewClient(wuapi.Config{
			BaseURL: srv.URL,
			Timeout: 5 * time.Second,
		}),
	}

	_, _, err := h.callLLM(context.Background(), callUpstream{APIKey: "fake"}, wuapi.ChatCompletionRequest{
		Model: "test/model",
	})
	if err == nil {
		t.Fatal("expected error for 502 upstream, got nil")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected '502' in error, got: %v", err)
	}
}

func TestCallLLM_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// No content events; just [DONE].
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	h := &Handler{
		WuApi: wuapi.NewClient(wuapi.Config{
			BaseURL: srv.URL,
			Timeout: 5 * time.Second,
		}),
	}

	_, _, err := h.callLLM(context.Background(), callUpstream{APIKey: "fake"}, wuapi.ChatCompletionRequest{
		Model: "test/model",
	})
	if err == nil {
		t.Fatal("expected error for empty response, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected 'empty' in error, got: %v", err)
	}
}
