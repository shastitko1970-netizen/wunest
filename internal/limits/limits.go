// Package limits implements per-resource slot caps for WuNest's
// subscription tiers (M54).
//
// One package, one purpose: given a user's current NestLevel and an
// existing object count, decide whether one more object can be created.
// Each create handler (characters, personas, presets, worldinfo) calls
// Check before invoking its Repo.Create — when Check returns
// ErrLimitReached the handler responds with HTTP 402 + a structured body
// the SPA can render as the "upgrade?" prompt.
//
// The numeric caps come from internal/models (NestFreeLimit etc.) — they
// aren't duplicated here so the source-of-truth stays with the model
// definitions. We only own the *policy* (which resource maps to which
// limit field, what an "unlimited" sentinel looks like, what the error
// envelope carries).
package limits

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/shastitko1970-netizen/wunest/internal/models"
)

// Resource is the category being counted. The four constants below
// match the four create-able object types in WuNest. Adding a fifth
// resource means: new const here, new field in catalog payload, new
// CountByUserID() in its repo, new Check call in its handler.
type Resource string

const (
	ResourceCharacter Resource = "character"
	ResourceLorebook  Resource = "lorebook"
	ResourcePersona   Resource = "persona"
	ResourcePreset    Resource = "preset"
)

// LimitFor returns the per-category cap for the given level. -1 means
// unlimited. Currently every resource has the same caps (3/10/-1), so
// the resource arg is unused — but it stays in the signature as a seam
// in case a future pricing change wants e.g. "Plus has 10 characters
// but 5 lorebooks" without rewriting every call site.
func LimitFor(level *models.NestLevel, _ Resource) int {
	return models.NestLimitFor(level)
}

// ErrLimitReached is returned from Check when the user has hit their
// per-resource cap. Carries the resource, the caller's current count,
// and the cap so HTTP layers can render a structured response.
//
// Callers convert this to a 402 Payment Required with a JSON body the
// SPA parses to render the "upgrade?" dialog. We pick 402 over 403
// deliberately: 403 is "you can't do this", 402 is "this requires
// payment" — semantically aligned and used by Stripe and a few other
// SaaS APIs for the same situation.
type ErrLimitReached struct {
	Resource Resource
	Current  int
	Max      int
}

func (e *ErrLimitReached) Error() string {
	return fmt.Sprintf("limit reached: %s %d/%d", e.Resource, e.Current, e.Max)
}

// IsLimitReached unwraps the error chain looking for ErrLimitReached.
// Used by HTTP handlers via errors.As to convert into a 402.
func IsLimitReached(err error) (*ErrLimitReached, bool) {
	var e *ErrLimitReached
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// WriteError writes a 402 Payment Required response with the structured
// JSON envelope the SPA expects:
//
//	{
//	  "kind":     "limit_reached",
//	  "resource": "character",
//	  "current":  3,
//	  "max":      3
//	}
//
// Why a fixed envelope: the SPA's apiFetch wrapper converts 402 into a
// rejection that includes the full body, so error UIs can distinguish
// "you hit a slot cap" from "you hit a daily quota" without parsing
// the upstream string.
func WriteError(w http.ResponseWriter, err *ErrLimitReached) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"kind":     "limit_reached",
		"resource": string(err.Resource),
		"current":  err.Current,
		"max":      err.Max,
	})
}

// Check reports whether the user can create one more object of the
// given resource. Returns nil for "yes" or *ErrLimitReached for "no".
//
// `currentCount` is the result of the resource's CountByUserID — caller
// owns the count query so this package stays pure (no DB).
//
// Unlimited (Pro = -1) returns nil unconditionally.
func Check(level *models.NestLevel, resource Resource, currentCount int) error {
	max := LimitFor(level, resource)
	if max < 0 {
		return nil // unlimited
	}
	if currentCount < max {
		return nil
	}
	return &ErrLimitReached{
		Resource: resource,
		Current:  currentCount,
		Max:      max,
	}
}
