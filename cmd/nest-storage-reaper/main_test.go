package main

import "testing"

func TestExtractBackgroundURLs_bgImageUrl(t *testing.T) {
	refs := map[string]bool{}
	raw := []byte(`{
		"appearance": {
			"bgImageUrl": "https://nest.example.com/images/backgrounds/abcdef0123456789012345678.png",
			"accent": "#ef4444"
		}
	}`)
	(&runner{}).extractBackgroundURLs(refs, raw)
	if !refs["abcdef0123456789012345678.png"] {
		t.Fatalf("expected bgImageUrl key in refs, got %v", refs)
	}
}

func TestExtractBackgroundURLs_regexOnly(t *testing.T) {
	refs := map[string]bool{}
	raw := []byte(`{"appearance":{"themePreset":"nest-default-dark","bgImageUrl":"https://x/images/backgrounds/aaaaaaaaaaaaaaaaaaaaaaaa.jpg"}}`)
	(&runner{}).extractBackgroundURLs(refs, raw)
	if !refs["aaaaaaaaaaaaaaaaaaaaaaaa.jpg"] {
		t.Fatalf("expected regex-harvested key, got %v", refs)
	}
}
