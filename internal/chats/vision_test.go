package chats

import (
	"testing"

	"github.com/shastitko1970-netizen/wunest/internal/presets"
)

func TestVisionCapableModel(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  bool
	}{
		{"empty", "", false},
		{"unknown gpt-3.5", "gpt-3.5-turbo", false},
		{"gpt-4o", "gpt-4o-2024-11-20", true},
		{"gpt-4.1", "gpt-4.1-mini", true},
		{"gpt-4 turbo", "gpt-4-turbo-preview", true},
		{"o1 reasoning", "o1-preview", true},
		{"claude 3 opus", "claude-3-opus-20240229", true},
		{"claude 3.5 sonnet", "claude-3-5-sonnet-20241022", true},
		{"claude 4 opus", "claude-4-opus", true},
		{"gemini 1.5 flash", "gemini-1.5-flash-002", true},
		{"gemini 2", "gemini-2.0-flash-exp", true},
		{"wu alias", "wu-kitsune", true}, // permissive: WuApi proxy re-routes
		{"pixtral", "pixtral-large-2411", true},
		{"llama-3 text", "llama-3-70b", false}, // text-only
		{"mistral small", "mistral-small-latest", false},
		{"case insensitive", "GPT-4O-2024-11-20", true},
		{"whitespace", "  claude-3-opus  ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VisionCapableModel(tc.model); got != tc.want {
				t.Errorf("VisionCapableModel(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestDecideMultimodal(t *testing.T) {
	boolp := func(b bool) *bool { return &b }

	t.Run("vision model, no bundle → enabled", func(t *testing.T) {
		opts := DecideMultimodal("gpt-4o", nil)
		if !opts.Enabled {
			t.Error("expected enabled by default for vision model")
		}
	})
	t.Run("text model → disabled regardless", func(t *testing.T) {
		opts := DecideMultimodal("gpt-3.5-turbo", nil)
		if opts.Enabled {
			t.Error("text model must not enable multimodal")
		}
	})
	t.Run("vision model + image_inlining=false → disabled", func(t *testing.T) {
		opts := DecideMultimodal("gpt-4o", &presets.OpenAIBundleData{
			ImageInlining: boolp(false),
		})
		if opts.Enabled {
			t.Error("explicit opt-out must disable")
		}
	})
	t.Run("vision model + image_inlining=true → enabled", func(t *testing.T) {
		opts := DecideMultimodal("gpt-4o", &presets.OpenAIBundleData{
			ImageInlining: boolp(true),
		})
		if !opts.Enabled {
			t.Error("explicit opt-in must enable")
		}
	})
	t.Run("inline image quality passed through", func(t *testing.T) {
		opts := DecideMultimodal("gpt-4o", &presets.OpenAIBundleData{
			InlineImageQuality: "high",
		})
		if opts.ImageQuality != "high" {
			t.Errorf("quality: %q, want high", opts.ImageQuality)
		}
	})
}

func TestApplyMultimodal(t *testing.T) {
	opts := MultimodalOptions{Enabled: true, ImageQuality: "high"}

	t.Run("disabled → no-op", func(t *testing.T) {
		in := []map[string]any{
			{"role": "user", "content": "hi ![](https://example.com/images/attachments/aaaaaaaaaaaaaaaaaaaaaaaa.png)"},
		}
		out := ApplyMultimodal(in, MultimodalOptions{Enabled: false})
		if _, ok := out[0]["content"].(string); !ok {
			t.Error("content should remain a string when disabled")
		}
	})

	t.Run("user message with attachment → multipart", func(t *testing.T) {
		in := []map[string]any{
			{"role": "user", "content": "What is this? ![](https://nest.wusphere.ru/images/attachments/abcdef012345678901234567.png) Explain."},
		}
		out := ApplyMultimodal(in, opts)
		parts, ok := out[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("content type: %T, want []map[string]any", out[0]["content"])
		}
		if len(parts) != 2 {
			t.Fatalf("got %d parts, want 2 (text + image_url)", len(parts))
		}
		if parts[0]["type"] != "text" {
			t.Errorf("part 0 type: %v", parts[0]["type"])
		}
		txt, _ := parts[0]["text"].(string)
		if txt != "What is this?  Explain." {
			t.Errorf("stripped text: %q", txt)
		}
		if parts[1]["type"] != "image_url" {
			t.Errorf("part 1 type: %v", parts[1]["type"])
		}
		img, _ := parts[1]["image_url"].(map[string]any)
		if img["url"] != "https://nest.wusphere.ru/images/attachments/abcdef012345678901234567.png" {
			t.Errorf("url: %v", img["url"])
		}
		if img["detail"] != "high" {
			t.Errorf("detail: %v, want high", img["detail"])
		}
	})

	t.Run("message without attachment → unchanged", func(t *testing.T) {
		in := []map[string]any{
			{"role": "user", "content": "just text"},
		}
		out := ApplyMultimodal(in, opts)
		if s, _ := out[0]["content"].(string); s != "just text" {
			t.Errorf("content changed: %q", out[0]["content"])
		}
	})

	t.Run("assistant messages never transformed", func(t *testing.T) {
		in := []map[string]any{
			{"role": "assistant", "content": "![](https://nest.wusphere.ru/images/attachments/abcdef012345678901234567.png)"},
		}
		out := ApplyMultimodal(in, opts)
		if _, ok := out[0]["content"].(string); !ok {
			t.Error("assistant content must stay string")
		}
	})

	t.Run("system messages never transformed", func(t *testing.T) {
		in := []map[string]any{
			{"role": "system", "content": "you are helpful ![](https://nest.wusphere.ru/images/attachments/abcdef012345678901234567.png)"},
		}
		out := ApplyMultimodal(in, opts)
		if _, ok := out[0]["content"].(string); !ok {
			t.Error("system content must stay string")
		}
	})

	t.Run("empty-after-strip gets [image] filler", func(t *testing.T) {
		in := []map[string]any{
			{"role": "user", "content": "![](https://nest.wusphere.ru/images/attachments/abcdef012345678901234567.png)"},
		}
		out := ApplyMultimodal(in, opts)
		parts, _ := out[0]["content"].([]map[string]any)
		txt, _ := parts[0]["text"].(string)
		if txt != "[image]" {
			t.Errorf("filler text: %q, want [image]", txt)
		}
	})

	t.Run("multiple images preserved + deduped", func(t *testing.T) {
		url := "https://nest.wusphere.ru/images/attachments/abcdef012345678901234567.png"
		in := []map[string]any{
			{"role": "user", "content": "compare ![](" + url + ") with ![](" + url + ")"},
		}
		out := ApplyMultimodal(in, opts)
		parts, _ := out[0]["content"].([]map[string]any)
		imgCount := 0
		for _, p := range parts {
			if p["type"] == "image_url" {
				imgCount++
			}
		}
		if imgCount != 1 {
			t.Errorf("dedup failed, got %d images, want 1", imgCount)
		}
	})

	t.Run("non-attachment URLs ignored", func(t *testing.T) {
		in := []map[string]any{
			{"role": "user", "content": "check https://example.com/some-image.png"},
		}
		out := ApplyMultimodal(in, opts)
		if _, ok := out[0]["content"].(string); !ok {
			t.Error("non-attachment URLs must not trigger transformation")
		}
	})
}
