package byok

import "testing"

func TestNormalizeBaseURL_LinkAPIBareOrigin(t *testing.T) {
	got := NormalizeBaseURL("custom", "https://linkapi.ai")
	if got != "https://linkapi.ai/v1" {
		t.Fatalf("got %q, want https://linkapi.ai/v1", got)
	}
}

func TestNormalizeBaseURL_LinkAPIAlreadyV1(t *testing.T) {
	got := NormalizeBaseURL("linkapi", "https://api.linkapi.ai/v1")
	if got != "https://api.linkapi.ai/v1" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeBaseURL_OpenAIUnchanged(t *testing.T) {
	raw := "https://api.openai.com/v1"
	if got := NormalizeBaseURL("openai", raw); got != raw {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeBaseURL_EmptyUsesProviderDefault(t *testing.T) {
	got := NormalizeBaseURL("linkapi", "")
	if got != "https://linkapi.ai/v1" {
		t.Fatalf("got %q", got)
	}
}
