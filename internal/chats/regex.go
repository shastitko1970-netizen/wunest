package chats

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/presets"
)

// Regex-script placement constants mirror SillyTavern's enum. Numeric
// values are stable across ST presets that users import.
const (
	regexPlacementMDDisplay    = 0 // deprecated in ST; still accepted on import
	regexPlacementUserInput    = 1 // user message before send
	regexPlacementAIOutput     = 2 // assistant output before save / display
	regexPlacementSlashCommand = 3 // unused
	regexPlacementWorldInfo    = 5 // unused
	regexPlacementReasoning    = 6 // thinking / reasoning blocks (future)
)

// ApplyRegexToUserInput runs all enabled scripts whose placement contains
// 1 against the outgoing user message.
func ApplyRegexToUserInput(bundle *presets.OpenAIBundleData, content string) string {
	return applyRegexWithPlacement(bundle, content, regexPlacementUserInput, regexApplyOptions{})
}

// ApplyRegexToAIOutput runs enabled scripts for AI output (placement 2).
// ST card/HTML transforms live here — must match SillyTavern replace
// semantics ($10+ capture groups, non-global patterns, trimStrings on
// captures, {{match}} alias).
func ApplyRegexToAIOutput(bundle *presets.OpenAIBundleData, content string) string {
	return applyRegexWithPlacement(bundle, content, regexPlacementAIOutput, regexApplyOptions{})
}

type regexApplyOptions struct {
	isMarkdown bool
	isPrompt   bool
}

func applyRegexWithPlacement(bundle *presets.OpenAIBundleData, content string, placement int, opts regexApplyOptions) string {
	if bundle == nil || len(bundle.Extensions.RegexScripts) == 0 || content == "" {
		return content
	}
	for _, script := range bundle.Extensions.RegexScripts {
		if script.Disabled {
			continue
		}
		if !placementMatches(script.Placement, placement) {
			continue
		}
		if !scriptMatchesContext(script, opts) {
			continue
		}
		find := script.FindRegex
		// substituteRegex: 0=raw, 1=macros in find (not expanded here yet).
		re, global, err := compileSTRegex(find)
		if err != nil {
			slog.Warn("regex: compile failed, skipping",
				"script", script.ScriptName,
				"err", err,
			)
			continue
		}
		content = runSTRegexReplace(content, re, global, script)
	}
	return content
}

// placementMatches returns true when a script should run for the requested
// ST placement. Placement 0 (legacy MD display) is treated as AI output.
func placementMatches(placements []int, want int) bool {
	for _, p := range placements {
		if p == want {
			return true
		}
		if want == regexPlacementAIOutput && p == regexPlacementMDDisplay {
			return true
		}
	}
	return false
}

// scriptMatchesContext mirrors SillyTavern getRegexedString filters.
func scriptMatchesContext(script presets.RegexScript, opts regexApplyOptions) bool {
	md := script.MarkdownOnly
	pr := script.PromptOnly
	if md && pr {
		return false
	}
	if md && opts.isMarkdown {
		return true
	}
	if pr && opts.isPrompt {
		return true
	}
	if !md && !pr && !opts.isMarkdown && !opts.isPrompt {
		return true
	}
	return false
}

type compiledSTRegex struct {
	re     *regexp.Regexp
	global bool
}

func compileSTRegex(pat string) (*regexp.Regexp, bool, error) {
	c, err := compileSTRegexFull(pat)
	if err != nil {
		return nil, false, err
	}
	return c.re, c.global, nil
}

func compileSTRegexFull(pat string) (*compiledSTRegex, error) {
	global := false
	if len(pat) >= 2 && pat[0] == '/' {
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
				case 'g':
					global = true
				case 'u', 'y':
					// no-op
				}
			}
			if modifier != "" {
				body = "(?" + modifier + ")" + body
			}
			re, err := regexp.Compile(body)
			if err != nil {
				return nil, err
			}
			return &compiledSTRegex{re: re, global: global}, nil
		}
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, err
	}
	// Plain patterns: ST treats as global when imported from /pat/g only;
	// unwrapped patterns replace all matches in our prior behavior.
	return &compiledSTRegex{re: re, global: true}, nil
}

var replaceGroupRef = regexp.MustCompile(`\$(\d+)|\$<([^>]+)>`)

// runSTRegexReplace applies one script using ST-style replacement callbacks.
func runSTRegexReplace(content string, re *regexp.Regexp, global bool, script presets.RegexScript) string {
	locs := re.FindAllStringSubmatchIndex(content, -1)
	if len(locs) == 0 {
		return content
	}
	if !global {
		locs = locs[:1]
	}
	var b strings.Builder
	b.Grow(len(content))
	last := 0
	for _, loc := range locs {
		b.WriteString(content[last:loc[0]])
		match := content[loc[0]:loc[1]]
		groups := submatchGroups(content, loc)
		b.WriteString(expandSTReplacement(script.ReplaceString, match, groups, script.TrimStrings))
		last = loc[1]
	}
	b.WriteString(content[last:])
	return b.String()
}

func submatchGroups(content string, loc []int) []string {
	out := make([]string, 0, len(loc)/2)
	for i := 0; i < len(loc); i += 2 {
		if loc[i] < 0 {
			out = append(out, "")
			continue
		}
		out = append(out, content[loc[i]:loc[i+1]])
	}
	return out
}

func expandSTReplacement(tmpl, match string, groups []string, trimStrings []string) string {
	if tmpl == "" {
		return ""
	}
	tmpl = strings.ReplaceAll(tmpl, "{{match}}", match)
	tmpl = strings.ReplaceAll(tmpl, "{{Match}}", match)
	out := replaceGroupRef.ReplaceAllStringFunc(tmpl, func(ref string) string {
		parts := replaceGroupRef.FindStringSubmatch(ref)
		if parts == nil {
			return ref
		}
		var val string
		if parts[1] != "" {
			n, _ := strconv.Atoi(parts[1])
			if n >= 0 && n < len(groups) {
				val = groups[n]
			}
		} else if parts[2] != "" {
			// Named groups: RE2 names not wired through []string; leave ref.
			return ref
		}
		return applyTrimStrings(val, trimStrings)
	})
	return out
}

func applyTrimStrings(s string, trimStrings []string) string {
	for _, t := range trimStrings {
		if t == "" {
			continue
		}
		s = strings.ReplaceAll(s, t, "")
	}
	return s
}

// translateSTReplace is kept for tests that assert legacy helper behavior.
func translateSTReplace(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "$&", "${0}")
	// Go interprets $10 as $1 + "0"; ST uses $10 as group 10.
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == '{' {
			// ${n} — copy through
			j := i + 2
			for j < len(s) && s[j] != '}' {
				j++
			}
			if j < len(s) {
				b.WriteString(s[i : j+1])
				i = j + 1
				continue
			}
		}
		j := i + 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j > i+1 {
			b.WriteString("${")
			b.WriteString(s[i+1 : j])
			b.WriteByte('}')
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
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
