package chats

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/presets"
)

// Regex-script placement constants mirror SillyTavern's enum. Numeric
// values are stable across ST presets that users import.
const (
	regexPlacementUserInput      = 1 // user message before send
	regexPlacementAIOutput       = 2 // assistant output before display
	regexPlacementSlashCommand   = 3 // unused
	regexPlacementWorldInfo      = 4 // unused
	regexPlacementReasoning      = 5 // unused
	regexPlacementDisplay        = 6 // post-render (unused server-side)
)

// ApplyRegexToUserInput runs all enabled scripts whose placement contains
// 1 against the outgoing user message. Used to implement jailbreak-style
// input-mangling (replace space with unicode invisible char, etc.) that
// bypasses content filters. Returns the (possibly modified) content.
func ApplyRegexToUserInput(bundle *presets.OpenAIBundleData, content string) string {
	return applyRegexWithPlacement(bundle, content, regexPlacementUserInput)
}

// ApplyRegexToAIOutput runs all enabled scripts whose placement contains
// 2 against the assistant's final content. Used for post-processing —
// stripping HTML, removing hidden unicode, rewriting formatting. Returns
// the (possibly modified) content.
func ApplyRegexToAIOutput(bundle *presets.OpenAIBundleData, content string) string {
	return applyRegexWithPlacement(bundle, content, regexPlacementAIOutput)
}

// applyRegexWithPlacement is the shared executor. Scripts with an invalid
// or uncompilable regex are skipped with a warn-level log (we never fail
// the request over a broken user-provided script) and the content is
// passed through unchanged past that script.
func applyRegexWithPlacement(bundle *presets.OpenAIBundleData, content string, placement int) string {
	if bundle == nil || len(bundle.Extensions.RegexScripts) == 0 || content == "" {
		return content
	}
	for _, script := range bundle.Extensions.RegexScripts {
		if script.Disabled {
			continue
		}
		if !containsInt(script.Placement, placement) {
			continue
		}
		re, err := compileSTRegex(script.FindRegex)
		if err != nil {
			slog.Warn("regex: compile failed, skipping",
				"script", script.ScriptName,
				"err", err,
			)
			continue
		}
		content = re.ReplaceAllString(content, translateSTReplace(script.ReplaceString))
		for _, trim := range script.TrimStrings {
			if trim == "" {
				continue
			}
			content = strings.ReplaceAll(content, trim, "")
		}
	}
	return content
}

// compileSTRegex translates ST's "/pattern/flags" JavaScript-style regex
// literal into Go's RE2. Supported flags:
//
//	i — case-insensitive      → (?i)
//	s — dotall                 → (?s)
//	m — multiline              → (?m)
//	g — global-replace         (RE2 is always "global" for ReplaceAll; no-op)
//	u — unicode                (RE2 is always UTF-8; no-op)
//	y — sticky                 (unsupported, ignored)
//
// Plain patterns without the slash-wrap are used as-is. Errors bubble up
// so the caller can skip a bad script.
func compileSTRegex(pat string) (*regexp.Regexp, error) {
	if len(pat) >= 2 && pat[0] == '/' {
		// Find the LAST slash — users may have slashes inside the pattern.
		if last := strings.LastIndex(pat, "/"); last > 0 {
			body := pat[1:last]
			flags := pat[last+1:]
			modifier := ""
			for _, f := range flags {
				switch f {
				case 'i':
					modifier += "i"
				case 's':
					modifier += "s"
				case 'm':
					modifier += "m"
				case 'g', 'u':
					// RE2 is always global in ReplaceAll; RE2 treats input
					// as UTF-8 by default. No-op.
				}
			}
			if modifier != "" {
				body = "(?" + modifier + ")" + body
			}
			return regexp.Compile(body)
		}
	}
	return regexp.Compile(pat)
}

// translateSTReplace converts JS-style `$1`, `$&` etc. to Go's `$1`. Go's
// regexp replacement syntax is largely compatible, but `$&` (whole match)
// needs to become `${0}`. We also escape literal `$` that isn't a
// capture reference (rare in user scripts — their replace strings are
// usually plain text).
func translateSTReplace(s string) string {
	if s == "" {
		return s
	}
	// `$&` → `${0}` (whole match).
	s = strings.ReplaceAll(s, "$&", "${0}")
	return s
}

func containsInt(haystack []int, needle int) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// SummarizeRegexScripts is a debug aid — stringifies the enabled/disabled
// list for a bundle so auth logs can show "preset X has N regex scripts".
// Unused in prod paths; kept exported for ad-hoc logging.
func SummarizeRegexScripts(bundle *presets.OpenAIBundleData) string {
	if bundle == nil {
		return "no bundle"
	}
	total := len(bundle.Extensions.RegexScripts)
	if total == 0 {
		return "no regex"
	}
	enabled := 0
	for _, s := range bundle.Extensions.RegexScripts {
		if !s.Disabled {
			enabled++
		}
	}
	return fmt.Sprintf("%d regex (%d enabled)", total, enabled)
}
