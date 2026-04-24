package chats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shastitko1970-netizen/wunest/internal/db"
)

// ErrNotFound is returned when a chat/message row is absent.
var ErrNotFound = errors.New("chat not found")

type Repository struct {
	pg *db.Postgres
}

func NewRepository(pg *db.Postgres) *Repository {
	return &Repository{pg: pg}
}

// --- chats ---

// SearchHit is one row from the full-text search across a user's chats.
type SearchHit struct {
	ChatID        uuid.UUID `json:"chat_id"`
	ChatName      string    `json:"chat_name"`
	CharacterID   *uuid.UUID `json:"character_id,omitempty"`
	CharacterName string    `json:"character_name,omitempty"`
	MessageID     int64     `json:"message_id"`
	Role          Role      `json:"role"`
	Snippet       string    `json:"snippet"`        // ts_headline-wrapped with <b>…</b> markers
	CreatedAt     time.Time `json:"created_at"`
}

// SearchMessages runs a full-text search across all messages the user
// owns. The search is scoped by user_id (join chat → user) and uses
// `simple` tsvector config (see migration 008 for language rationale).
//
// Ranking: ts_rank_cd against the query's tsquery. Tie-break on
// created_at DESC so recent hits float.
//
// optionalCharacterID restricts to a single character's chats when
// non-nil — used by the Library search-in-character-card flow.
//
// Limit caps result size; 50 is a reasonable default that keeps the
// response JSON under 1 MiB even with verbose snippets.
func (r *Repository) SearchMessages(
	ctx context.Context,
	userID uuid.UUID,
	query string,
	optionalCharacterID *uuid.UUID,
	limit int,
) ([]SearchHit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// plainto_tsquery turns a user string ("alice the blacksmith") into
	// an AND tsquery — simpler than websearch_to_tsquery (which handles
	// quotes + OR) but sufficient for chat search. If the query is all
	// short-token noise, the matcher finds nothing, which is fine.
	const qs = `
		SELECT
		    c.id, c.name,
		    c.character_id, COALESCE(ch.name, '') AS character_name,
		    m.id, m.role,
		    ts_headline('simple', m.content, plainto_tsquery('simple', $2),
		        'StartSel=<<<, StopSel=>>>, MaxWords=20, MinWords=5, MaxFragments=2') AS snippet,
		    m.created_at
		  FROM nest_messages m
		  JOIN nest_chats c ON c.id = m.chat_id
		  LEFT JOIN nest_characters ch ON ch.id = c.character_id
		 WHERE c.user_id = $1
		   AND m.content_tsv @@ plainto_tsquery('simple', $2)
		   AND m.hidden = FALSE
		   AND ($3::uuid IS NULL OR c.character_id = $3)
		 ORDER BY ts_rank_cd(m.content_tsv, plainto_tsquery('simple', $2)) DESC,
		          m.created_at DESC
		 LIMIT $4
	`
	rows, err := r.pg.Query(ctx, qs, userID, q, optionalCharacterID, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	out := make([]SearchHit, 0)
	for rows.Next() {
		var h SearchHit
		var role string
		if err := rows.Scan(
			&h.ChatID, &h.ChatName, &h.CharacterID, &h.CharacterName,
			&h.MessageID, &role, &h.Snippet, &h.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan search hit: %w", err)
		}
		h.Role = Role(role)
		out = append(out, h)
	}
	return out, rows.Err()
}

// ListChats returns a user's chats, most-recently-active first.
// The `last_message_at` is derived from the latest nest_messages row.
func (r *Repository) ListChats(ctx context.Context, userID uuid.UUID) ([]Chat, error) {
	const q = `
		SELECT
		    c.id, c.user_id, c.character_id, COALESCE(ch.name, '') AS character_name,
		    c.character_ids,
		    c.name, c.tags, c.chat_metadata, c.created_at, c.updated_at,
		    COALESCE((SELECT MAX(m.created_at) FROM nest_messages m WHERE m.chat_id = c.id), c.updated_at) AS last_message_at
		  FROM nest_chats c
		  LEFT JOIN nest_characters ch ON ch.id = c.character_id
		 WHERE c.user_id = $1
		 ORDER BY last_message_at DESC
	`
	rows, err := r.pg.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()

	out := make([]Chat, 0)
	for rows.Next() {
		var c Chat
		var meta []byte
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.CharacterID, &c.CharacterName,
			&c.CharacterIDs,
			&c.Name, &c.Tags, &meta, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt,
		); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		c.Metadata = meta
		c.IsGroupChat = len(c.CharacterIDs) > 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetChat returns one chat by id, scoped to the user.
func (r *Repository) GetChat(ctx context.Context, userID, id uuid.UUID) (*Chat, error) {
	const q = `
		SELECT
		    c.id, c.user_id, c.character_id, COALESCE(ch.name, '') AS character_name,
		    c.character_ids,
		    c.name, c.tags, c.chat_metadata, c.created_at, c.updated_at,
		    COALESCE((SELECT MAX(m.created_at) FROM nest_messages m WHERE m.chat_id = c.id), c.updated_at)
		  FROM nest_chats c
		  LEFT JOIN nest_characters ch ON ch.id = c.character_id
		 WHERE c.user_id = $1 AND c.id = $2
	`
	var c Chat
	var meta []byte
	err := r.pg.QueryRow(ctx, q, userID, id).Scan(
		&c.ID, &c.UserID, &c.CharacterID, &c.CharacterName,
		&c.CharacterIDs,
		&c.Name, &c.Tags, &meta, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get chat: %w", err)
	}
	c.Metadata = meta
	c.IsGroupChat = len(c.CharacterIDs) > 1
	return &c, nil
}

func (r *Repository) CreateChat(ctx context.Context, in CreateChatInput) (*Chat, error) {
	if in.Name == "" {
		in.Name = "New chat"
	}
	if in.Metadata == nil {
		in.Metadata = []byte("{}")
	}
	// Reconcile CharacterID vs CharacterIDs. Callers may pass either (or
	// both for clarity); we treat CharacterIDs as the source of truth and
	// derive CharacterID from position 0 when needed.
	if len(in.CharacterIDs) == 0 && in.CharacterID != nil {
		in.CharacterIDs = []uuid.UUID{*in.CharacterID}
	}
	if in.CharacterID == nil && len(in.CharacterIDs) > 0 {
		first := in.CharacterIDs[0]
		in.CharacterID = &first
	}
	// nest_chats.character_ids is NOT NULL; a nil Go slice marshals as
	// NULL via pgx. Force to empty-array so the INSERT doesn't panic.
	if in.CharacterIDs == nil {
		in.CharacterIDs = []uuid.UUID{}
	}
	id := uuid.New() // app-side UUID; see characters/repo.go comment
	const q = `
		INSERT INTO nest_chats (id, user_id, character_id, character_ids, name, chat_metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at
	`
	var createdAt, updAt time.Time
	if err := r.pg.QueryRow(ctx, q,
		id, in.UserID, in.CharacterID, in.CharacterIDs, in.Name, in.Metadata,
	).Scan(&createdAt, &updAt); err != nil {
		return nil, fmt.Errorf("insert chat: %w", err)
	}
	return &Chat{
		ID:            id,
		UserID:        in.UserID,
		CharacterID:   in.CharacterID,
		CharacterIDs:  in.CharacterIDs,
		IsGroupChat:   len(in.CharacterIDs) > 1,
		Name:          in.Name,
		Metadata:      in.Metadata,
		CreatedAt:     createdAt,
		UpdatedAt:     updAt,
		LastMessageAt: updAt,
	}, nil
}

// SetTags overwrites the tag list for a chat. Empty slice clears tags.
// Tags are stored verbatim — caller handles dedup/normalisation (the
// frontend does case-folded dedup before submit).
func (r *Repository) SetTags(ctx context.Context, userID, id uuid.UUID, tags []string) error {
	if tags == nil {
		tags = []string{}
	}
	// Trim whitespace + drop empties + dedupe (case-insensitive) server-
	// side as a belt-and-suspenders guard. Order is preserved from input.
	seen := make(map[string]struct{}, len(tags))
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		clean = append(clean, t)
	}
	const q = `UPDATE nest_chats SET tags = $3, updated_at = NOW() WHERE user_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, userID, id, clean)
	if err != nil {
		return fmt.Errorf("set tags: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DistinctTags returns all tags the user has ever used, alphabetically
// sorted. Used for autocomplete + filter UI. Cheap query thanks to the
// GIN index.
func (r *Repository) DistinctTags(ctx context.Context, userID uuid.UUID) ([]string, error) {
	const q = `
		SELECT DISTINCT unnest(tags) AS t
		  FROM nest_chats
		 WHERE user_id = $1
		 ORDER BY t ASC
	`
	rows, err := r.pg.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("distinct tags: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0, 20)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) RenameChat(ctx context.Context, userID, id uuid.UUID, name string) error {
	const q = `UPDATE nest_chats SET name = $3, updated_at = NOW() WHERE user_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, userID, id, name)
	if err != nil {
		return fmt.Errorf("rename chat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSampler merges sampler settings into a chat's chat_metadata JSONB
// without touching any other metadata fields. Uses jsonb_set so sibling
// keys (world info state, author's note, etc.) survive untouched.
func (r *Repository) SetSampler(ctx context.Context, userID, id uuid.UUID, sampler ChatSamplerMetadata) error {
	samplerJSON, err := json.Marshal(sampler)
	if err != nil {
		return fmt.Errorf("marshal sampler: %w", err)
	}
	const q = `
		UPDATE nest_chats
		   SET chat_metadata = jsonb_set(COALESCE(chat_metadata, '{}'::jsonb), '{sampler}', $3::jsonb, true),
		       updated_at = NOW()
		 WHERE user_id = $1 AND id = $2
	`
	tag, err := r.pg.Exec(ctx, q, userID, id, string(samplerJSON))
	if err != nil {
		return fmt.Errorf("set sampler: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// readSampler extracts chat_metadata.sampler if present. Returns a zero
// value (no fields set) when missing/invalid — callers treat that as
// "use server defaults".
func readSampler(raw json.RawMessage) ChatSamplerMetadata {
	if len(raw) == 0 {
		return ChatSamplerMetadata{}
	}
	var envelope struct {
		Sampler ChatSamplerMetadata `json:"sampler"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ChatSamplerMetadata{}
	}
	return envelope.Sampler
}

// SetPersona sets or clears the per-chat persona override. Pass uuid.Nil to
// clear. Sibling metadata (sampler etc.) is preserved.
func (r *Repository) SetPersona(ctx context.Context, userID, id uuid.UUID, personaID uuid.UUID) error {
	// jsonb_set would only create a string key; for a proper UUID-or-null
	// we build a tiny JSON value client-side and either set or remove the key.
	if personaID == uuid.Nil {
		const clear = `
			UPDATE nest_chats
			   SET chat_metadata = chat_metadata - 'persona_id',
			       updated_at = NOW()
			 WHERE user_id = $1 AND id = $2
		`
		tag, err := r.pg.Exec(ctx, clear, userID, id)
		if err != nil {
			return fmt.Errorf("clear persona: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	}

	// to_jsonb($3) wraps the text id as a JSON string value.
	const q = `
		UPDATE nest_chats
		   SET chat_metadata = jsonb_set(COALESCE(chat_metadata, '{}'::jsonb), '{persona_id}', to_jsonb($3::text), true),
		       updated_at = NOW()
		 WHERE user_id = $1 AND id = $2
	`
	tag, err := r.pg.Exec(ctx, q, userID, id, personaID.String())
	if err != nil {
		return fmt.Errorf("set persona: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetAuthorsNote writes or clears chat_metadata.authors_note. Pass nil to
// remove the key entirely. Sibling metadata (sampler, persona_id) stays put.
func (r *Repository) SetAuthorsNote(ctx context.Context, userID, id uuid.UUID, note *AuthorsNote) error {
	if note == nil {
		const clear = `
			UPDATE nest_chats
			   SET chat_metadata = chat_metadata - 'authors_note',
			       updated_at = NOW()
			 WHERE user_id = $1 AND id = $2
		`
		tag, err := r.pg.Exec(ctx, clear, userID, id)
		if err != nil {
			return fmt.Errorf("clear authors_note: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	}
	noteJSON, err := json.Marshal(note)
	if err != nil {
		return fmt.Errorf("marshal authors_note: %w", err)
	}
	const q = `
		UPDATE nest_chats
		   SET chat_metadata = jsonb_set(COALESCE(chat_metadata, '{}'::jsonb), '{authors_note}', $3::jsonb, true),
		       updated_at = NOW()
		 WHERE user_id = $1 AND id = $2
	`
	tag, err := r.pg.Exec(ctx, q, userID, id, noteJSON)
	if err != nil {
		return fmt.Errorf("set authors_note: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// readAuthorsNote extracts chat_metadata.authors_note or nil. The pointer
// shape lets callers distinguish "unset" from "explicitly empty".
func readAuthorsNote(raw json.RawMessage) *AuthorsNote {
	if len(raw) == 0 {
		return nil
	}
	var envelope struct {
		AuthorsNote *AuthorsNote `json:"authors_note"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil
	}
	return envelope.AuthorsNote
}

// readVariables extracts chat_metadata.variables as a flat string map.
// Returns an empty map when missing — so the macro engine can mutate
// via {{setvar}} without a nil check.
func readVariables(raw json.RawMessage) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var envelope struct {
		Variables map[string]string `json:"variables"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return out
	}
	for k, v := range envelope.Variables {
		out[k] = v
	}
	return out
}

// SaveVariables upserts chat_metadata.variables with the given map.
// Used after generation to persist {{setvar}} side-effects. Other
// metadata keys (sampler, persona, authors_note, …) are preserved.
func (r *Repository) SaveVariables(ctx context.Context, chatID uuid.UUID, vars map[string]string) error {
	if vars == nil {
		vars = map[string]string{}
	}
	payload, err := json.Marshal(vars)
	if err != nil {
		return fmt.Errorf("marshal variables: %w", err)
	}
	const q = `
		UPDATE nest_chats
		   SET chat_metadata = jsonb_set(
		           COALESCE(chat_metadata, '{}'::jsonb),
		           '{variables}',
		           $2::jsonb,
		           true),
		       updated_at = NOW()
		 WHERE id = $1
	`
	if _, err := r.pg.Exec(ctx, q, chatID, string(payload)); err != nil {
		return fmt.Errorf("save variables: %w", err)
	}
	return nil
}

// UsageTotal is the persisted "how much provider money this chat has eaten"
// counter. Monotonically increasing: incremented atomically after every
// successful provider call (initial send, regenerate, swipe, continue).
// Swipes/regens don't reduce it when prior assistant extras get overwritten,
// so the SPA's spend chip stays truthful even across retries and branching.
type UsageTotal struct {
	TokensIn  int64 `json:"tokens_in"`
	TokensOut int64 `json:"tokens_out"`
	APICalls  int64 `json:"api_calls"`
}

// IncrementChatUsage atomically adds the reported usage of one API call to
// chat_metadata.usage_total. COALESCE handles first-call-on-this-chat
// (field doesn't exist yet) and legacy chats from before this counter was
// introduced. Returns the updated UsageTotal so the caller can emit it on
// the SSE `done` event for the SPA to reflect immediately (no refetch).
func (r *Repository) IncrementChatUsage(ctx context.Context, chatID uuid.UUID, tokensIn, tokensOut int64) (*UsageTotal, error) {
	// Guard against providers that reported nothing — still count the call,
	// but zeroed tokens so we don't poison the average.
	if tokensIn < 0 {
		tokensIn = 0
	}
	if tokensOut < 0 {
		tokensOut = 0
	}
	const q = `
		UPDATE nest_chats
		   SET chat_metadata = COALESCE(chat_metadata, '{}'::jsonb) || jsonb_build_object(
		         'usage_total',
		         jsonb_build_object(
		           'tokens_in',  COALESCE((chat_metadata->'usage_total'->>'tokens_in')::bigint,  0) + $2,
		           'tokens_out', COALESCE((chat_metadata->'usage_total'->>'tokens_out')::bigint, 0) + $3,
		           'api_calls',  COALESCE((chat_metadata->'usage_total'->>'api_calls')::bigint,  0) + 1
		         )
		       ),
		       updated_at = NOW()
		 WHERE id = $1
		 RETURNING
		   COALESCE((chat_metadata->'usage_total'->>'tokens_in')::bigint,  0),
		   COALESCE((chat_metadata->'usage_total'->>'tokens_out')::bigint, 0),
		   COALESCE((chat_metadata->'usage_total'->>'api_calls')::bigint,  0)
	`
	var out UsageTotal
	if err := r.pg.QueryRow(ctx, q, chatID, tokensIn, tokensOut).Scan(&out.TokensIn, &out.TokensOut, &out.APICalls); err != nil {
		return nil, fmt.Errorf("increment chat usage: %w", err)
	}
	return &out, nil
}

// SetBYOK writes or clears chat_metadata.byok_id. uuid.Nil clears the key;
// otherwise the chat's stream path will use the corresponding BYOK key
// instead of the user's WuApi key. Ownership check stays with the handler.
func (r *Repository) SetBYOK(ctx context.Context, userID, id, byokID uuid.UUID) error {
	if byokID == uuid.Nil {
		const clear = `
			UPDATE nest_chats
			   SET chat_metadata = chat_metadata - 'byok_id',
			       updated_at = NOW()
			 WHERE user_id = $1 AND id = $2
		`
		tag, err := r.pg.Exec(ctx, clear, userID, id)
		if err != nil {
			return fmt.Errorf("clear byok_id: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	}
	const q = `
		UPDATE nest_chats
		   SET chat_metadata = jsonb_set(COALESCE(chat_metadata, '{}'::jsonb), '{byok_id}', to_jsonb($3::text), true),
		       updated_at = NOW()
		 WHERE user_id = $1 AND id = $2
	`
	tag, err := r.pg.Exec(ctx, q, userID, id, byokID.String())
	if err != nil {
		return fmt.Errorf("set byok_id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// readBYOKID extracts chat_metadata.byok_id, or uuid.Nil when missing.
func readBYOKID(raw json.RawMessage) uuid.UUID {
	if len(raw) == 0 {
		return uuid.Nil
	}
	var env struct {
		BYOKID string `json:"byok_id"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return uuid.Nil
	}
	if env.BYOKID == "" {
		return uuid.Nil
	}
	id, err := uuid.Parse(env.BYOKID)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// readPersonaID extracts chat_metadata.persona_id if present. Returns uuid.Nil
// when missing/invalid so callers can treat "no override" uniformly.
func readPersonaID(raw json.RawMessage) uuid.UUID {
	if len(raw) == 0 {
		return uuid.Nil
	}
	var envelope struct {
		PersonaID string `json:"persona_id"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return uuid.Nil
	}
	if envelope.PersonaID == "" {
		return uuid.Nil
	}
	id, err := uuid.Parse(envelope.PersonaID)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func (r *Repository) DeleteChat(ctx context.Context, userID, id uuid.UUID) error {
	const q = `DELETE FROM nest_chats WHERE user_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, userID, id)
	if err != nil {
		return fmt.Errorf("delete chat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- messages ---

// ListMessages returns all messages in a chat, oldest first.
// Hidden messages are NOT returned unless includeHidden is true.
func (r *Repository) ListMessages(ctx context.Context, chatID uuid.UUID, includeHidden bool) ([]Message, error) {
	q := `
		SELECT id, chat_id, role, content, swipes, swipe_id, extras, hidden, character_id, swipe_character_ids, created_at
		  FROM nest_messages
		 WHERE chat_id = $1
	`
	if !includeHidden {
		q += ` AND hidden = FALSE`
	}
	q += ` ORDER BY id ASC`

	rows, err := r.pg.Query(ctx, q, chatID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	out := make([]Message, 0)
	for rows.Next() {
		var m Message
		var swipes, extras []byte
		var role string
		if err := rows.Scan(
			&m.ID, &m.ChatID, &role, &m.Content, &swipes, &m.SwipeID, &extras, &m.Hidden, &m.CharacterID, &m.SwipeCharacterIDs, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.Role = Role(role)
		m.Swipes = swipes
		m.Extras = extras
		out = append(out, m)
	}
	return out, rows.Err()
}

// AppendMessage inserts a new message. Returns the persisted row with its id.
// Delegates to AppendMessageForCharacter with a nil speaker — keeps legacy
// callers working unchanged.
func (r *Repository) AppendMessage(ctx context.Context, chatID uuid.UUID, role Role, content string, extras *MessageExtras) (*Message, error) {
	return r.AppendMessageForCharacter(ctx, chatID, role, content, nil, extras)
}

// AppendMessageForCharacter inserts a new message and records the speaking
// character (for group-chat assistant turns). `characterID` should be nil
// for user/system messages and for single-character assistant messages
// where chat.character_id already says who spoke.
func (r *Repository) AppendMessageForCharacter(
	ctx context.Context,
	chatID uuid.UUID,
	role Role,
	content string,
	characterID *uuid.UUID,
	extras *MessageExtras,
) (*Message, error) {
	extrasJSON := []byte("{}")
	if extras != nil {
		b, err := json.Marshal(extras)
		if err != nil {
			return nil, fmt.Errorf("marshal extras: %w", err)
		}
		extrasJSON = b
	}

	const q = `
		INSERT INTO nest_messages (chat_id, role, content, character_id, extras)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`
	var (
		id        int64
		createdAt time.Time
	)
	if err := r.pg.QueryRow(ctx, q, chatID, string(role), content, characterID, extrasJSON).
		Scan(&id, &createdAt); err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}

	// Also bump the chat's updated_at so it floats to the top of ListChats.
	if _, err := r.pg.Exec(ctx, `UPDATE nest_chats SET updated_at = NOW() WHERE id = $1`, chatID); err != nil {
		// Non-fatal — log via caller.
		_ = err
	}

	return &Message{
		ID:          id,
		ChatID:      chatID,
		Role:        role,
		Content:     content,
		Extras:      extrasJSON,
		CharacterID: characterID,
		CreatedAt:   createdAt,
	}, nil
}

// AppendMessageWithSwipes inserts a message with a pre-populated swipes[]
// array and swipe_id pointing at the visible entry. Used when seeding a
// new chat's greeting from a character that ships alternate_greetings —
// matches SillyTavern's behavior of letting users swipe through all the
// greetings the character author wrote.
//
// Invariants: swipes[swipeID] must equal content; pass an empty swipes
// slice to fall back to the normal AppendMessage path.
//
// Thin wrapper: delegates to AppendMessageWithSwipesAttributed with
// nil swipeCharacterIDs (single-character attribution).
func (r *Repository) AppendMessageWithSwipes(
	ctx context.Context,
	chatID uuid.UUID,
	role Role,
	content string,
	swipes []string,
	swipeID int,
	extras *MessageExtras,
) (*Message, error) {
	return r.AppendMessageWithSwipesAttributed(ctx, chatID, role, content, swipes, nil, swipeID, nil, extras)
}

// AppendMessageWithSwipesAttributed is the group-chat-aware variant of
// AppendMessageWithSwipes: swipe i is owned by swipeCharacterIDs[i].
// Used when seeding a group chat's opening swipes pool where each
// swipe is a different character's first_mes.
//
// Arguments:
//   swipes               — texts, parallel to swipeCharacterIDs
//   swipeCharacterIDs    — attribution per swipe; nil means
//                          "all swipes attributed to characterID"
//   swipeID              — visible-swipe pointer (0..len-1)
//   characterID          — message-level attribution; also the owner
//                          of swipes[swipeID]'s initial content
//
// Callers already guard against len(swipes) <= 1 by falling through
// to AppendMessageForCharacter / AppendMessage — keeps the "no swipes
// needed" hot path cheap.
func (r *Repository) AppendMessageWithSwipesAttributed(
	ctx context.Context,
	chatID uuid.UUID,
	role Role,
	content string,
	swipes []string,
	swipeCharacterIDs []uuid.UUID,
	swipeID int,
	characterID *uuid.UUID,
	extras *MessageExtras,
) (*Message, error) {
	if len(swipes) <= 1 {
		// One-or-fewer variants — no point storing a swipes array.
		return r.AppendMessageForCharacter(ctx, chatID, role, content, characterID, extras)
	}
	if swipeID < 0 || swipeID >= len(swipes) {
		swipeID = 0
	}
	extrasJSON := []byte("{}")
	if extras != nil {
		b, err := json.Marshal(extras)
		if err != nil {
			return nil, fmt.Errorf("marshal extras: %w", err)
		}
		extrasJSON = b
	}
	swipesJSON, err := json.Marshal(swipes)
	if err != nil {
		return nil, fmt.Errorf("marshal swipes: %w", err)
	}

	// Validate swipeCharacterIDs length — nil passes through; wrong
	// length triggers a silent reset to nil so we never persist a
	// misaligned array (bug that'd be hell to debug later).
	if swipeCharacterIDs != nil && len(swipeCharacterIDs) != len(swipes) {
		swipeCharacterIDs = nil
	}

	const q = `
		INSERT INTO nest_messages
		    (chat_id, role, content, swipes, swipe_id, extras, character_id, swipe_character_ids)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`
	var (
		id        int64
		createdAt time.Time
	)
	if err := r.pg.QueryRow(ctx, q,
		chatID, string(role), content, swipesJSON, swipeID, extrasJSON, characterID, swipeCharacterIDs,
	).Scan(&id, &createdAt); err != nil {
		return nil, fmt.Errorf("insert message with swipes: %w", err)
	}

	// Bump chat updated_at so the row floats in ListChats, same as the
	// regular AppendMessage path.
	if _, err := r.pg.Exec(ctx, `UPDATE nest_chats SET updated_at = NOW() WHERE id = $1`, chatID); err != nil {
		_ = err
	}

	return &Message{
		ID:                id,
		ChatID:            chatID,
		Role:              role,
		Content:           content,
		Swipes:            swipesJSON,
		SwipeID:           swipeID,
		Extras:            extrasJSON,
		CharacterID:       characterID,
		SwipeCharacterIDs: swipeCharacterIDs,
		CreatedAt:         createdAt,
	}, nil
}

// UpdateMessageContent replaces the content and extras of an existing message.
// Used when the assistant's stream finishes — we initially insert an empty
// placeholder and patch it once the final text is known.
func (r *Repository) UpdateMessageContent(ctx context.Context, id int64, content string, extras *MessageExtras) error {
	extrasJSON := []byte("{}")
	if extras != nil {
		b, err := json.Marshal(extras)
		if err != nil {
			return fmt.Errorf("marshal extras: %w", err)
		}
		extrasJSON = b
	}
	const q = `UPDATE nest_messages SET content = $2, extras = $3 WHERE id = $1`
	if _, err := r.pg.Exec(ctx, q, id, content, extrasJSON); err != nil {
		return fmt.Errorf("update message: %w", err)
	}
	return nil
}

// DeleteMessage removes a message from a chat.
func (r *Repository) DeleteMessage(ctx context.Context, chatID uuid.UUID, messageID int64) error {
	const q = `DELETE FROM nest_messages WHERE chat_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, chatID, messageID)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMessagesAfter removes the target message AND every message with a
// higher id in the same chat. Returns the count of rows removed so the UI
// can give feedback ("123 messages deleted") — useful confirmation after
// a prune since the user doesn't always know how many were in the tail.
//
// Includes the target itself because the usual trigger is "this reply is
// bad + everything it spawned" — keeping the target would leave the bad
// message + a user turn pointing nowhere.
func (r *Repository) DeleteMessagesAfter(ctx context.Context, chatID uuid.UUID, messageID int64) (int64, error) {
	const q = `DELETE FROM nest_messages WHERE chat_id = $1 AND id >= $2`
	tag, err := r.pg.Exec(ctx, q, chatID, messageID)
	if err != nil {
		return 0, fmt.Errorf("delete messages after: %w", err)
	}
	return tag.RowsAffected(), nil
}

// EditMessageContent updates just the content field of a message, leaving
// role, extras, swipes, timestamps untouched. Use for user-driven edits
// from the UI; `UpdateMessageContent` is for stream-finalisation.
func (r *Repository) EditMessageContent(ctx context.Context, chatID uuid.UUID, messageID int64, content string) error {
	const q = `UPDATE nest_messages SET content = $3 WHERE chat_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, chatID, messageID, content)
	if err != nil {
		return fmt.Errorf("edit message: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Summaries (M38.4 memory) ─────────────────────────────────────

// ListSummaries returns every summary row for the chat, position-asc.
// All three roles (auto, manual, pinned) interleave in the result; UI
// sorts by role + position.
func (r *Repository) ListSummaries(ctx context.Context, chatID uuid.UUID) ([]Summary, error) {
	const q = `
		SELECT id, chat_id, content, role, covered_through_message_id,
		       token_count, model, position, created_at, updated_at
		  FROM nest_chat_summaries
		 WHERE chat_id = $1
		 ORDER BY role DESC, position ASC, created_at ASC
	`
	rows, err := r.pg.Query(ctx, q, chatID)
	if err != nil {
		return nil, fmt.Errorf("list summaries: %w", err)
	}
	defer rows.Close()
	out := make([]Summary, 0, 4)
	for rows.Next() {
		var s Summary
		if err := rows.Scan(
			&s.ID, &s.ChatID, &s.Content, &s.Role, &s.CoveredThroughMessageID,
			&s.TokenCount, &s.Model, &s.Position, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetAutoSummary returns the chat's single auto-summary row (nil + no
// error when missing). Used by the memory-injection path to decide
// whether to prepend a summary block.
func (r *Repository) GetAutoSummary(ctx context.Context, chatID uuid.UUID) (*Summary, error) {
	const q = `
		SELECT id, chat_id, content, role, covered_through_message_id,
		       token_count, model, position, created_at, updated_at
		  FROM nest_chat_summaries
		 WHERE chat_id = $1 AND role = 'auto'
		 LIMIT 1
	`
	var s Summary
	err := r.pg.QueryRow(ctx, q, chatID).Scan(
		&s.ID, &s.ChatID, &s.Content, &s.Role, &s.CoveredThroughMessageID,
		&s.TokenCount, &s.Model, &s.Position, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get auto summary: %w", err)
	}
	return &s, nil
}

// UpsertAutoSummary replaces-or-inserts the single auto-summary row.
// Returns the persisted Summary so the caller can emit it to the UI.
func (r *Repository) UpsertAutoSummary(
	ctx context.Context,
	chatID uuid.UUID,
	content string,
	coveredThroughMessageID int64,
	tokenCount int,
	model string,
) (*Summary, error) {
	// Delete existing auto row then insert fresh — cleaner than a
	// conditional UPSERT given we want a fresh updated_at either way.
	if _, err := r.pg.Exec(ctx,
		`DELETE FROM nest_chat_summaries WHERE chat_id = $1 AND role = 'auto'`,
		chatID,
	); err != nil {
		return nil, fmt.Errorf("drop old auto summary: %w", err)
	}
	id := uuid.New()
	const q = `
		INSERT INTO nest_chat_summaries
		    (id, chat_id, content, role, covered_through_message_id, token_count, model, position)
		VALUES ($1, $2, $3, 'auto', $4, $5, $6, 0)
		RETURNING created_at, updated_at
	`
	s := &Summary{
		ID:                      id,
		ChatID:                  chatID,
		Content:                 content,
		Role:                    "auto",
		CoveredThroughMessageID: &coveredThroughMessageID,
		TokenCount:              tokenCount,
		Model:                   model,
	}
	if err := r.pg.QueryRow(ctx, q,
		id, chatID, content, coveredThroughMessageID, tokenCount, model,
	).Scan(&s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert auto summary: %w", err)
	}
	return s, nil
}

// CreateManualSummary inserts a user-authored summary row (role=manual
// or role=pinned). Position defaults to the next available slot.
func (r *Repository) CreateManualSummary(
	ctx context.Context,
	chatID uuid.UUID,
	content string,
	pinned bool,
) (*Summary, error) {
	role := "manual"
	if pinned {
		role = "pinned"
	}
	id := uuid.New()
	s := &Summary{
		ID:      id,
		ChatID:  chatID,
		Content: content,
		Role:    role,
	}
	const q = `
		INSERT INTO nest_chat_summaries (id, chat_id, content, role, position)
		VALUES ($1, $2, $3, $4,
		    COALESCE((SELECT MAX(position)+1 FROM nest_chat_summaries WHERE chat_id = $2 AND role = $4), 0))
		RETURNING position, created_at, updated_at
	`
	if err := r.pg.QueryRow(ctx, q, id, chatID, content, role).
		Scan(&s.Position, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert manual summary: %w", err)
	}
	return s, nil
}

// UpdateSummary edits an existing summary's content + role. Used for
// user edits in the memory drawer (including promoting auto→manual
// to prevent regen from overwriting user tweaks).
func (r *Repository) UpdateSummary(ctx context.Context, id uuid.UUID, content string, role string) error {
	if role == "" {
		role = "manual"
	}
	const q = `
		UPDATE nest_chat_summaries
		   SET content = $2, role = $3, updated_at = NOW()
		 WHERE id = $1
	`
	tag, err := r.pg.Exec(ctx, q, id, content, role)
	if err != nil {
		return fmt.Errorf("update summary: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSummary removes a summary by id (owner verified at handler via
// JOIN to chat ownership).
func (r *Repository) DeleteSummary(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pg.Exec(ctx, `DELETE FROM nest_chat_summaries WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete summary: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ChatStats is an aggregate snapshot of a chat's activity. All counts
// include hidden messages (which still exist in DB); tokens are based
// on what the model actually saw at the time (extras.tokens_*).
type ChatStats struct {
	ChatID               uuid.UUID `json:"chat_id"`
	MessagesTotal        int       `json:"messages_total"`
	MessagesUser         int       `json:"messages_user"`
	MessagesAssistant    int       `json:"messages_assistant"`
	MessagesSystem       int       `json:"messages_system"`
	MessagesHidden       int       `json:"messages_hidden"`
	TokensInTotal        int       `json:"tokens_in_total"`
	TokensOutTotal       int       `json:"tokens_out_total"`
	SwipesTotal          int       `json:"swipes_total"`          // cumulative regenerations
	FirstMessageAt       *time.Time `json:"first_message_at,omitempty"`
	LastMessageAt        *time.Time `json:"last_message_at,omitempty"`
	UniqueModelsUsed     int        `json:"unique_models_used"`    // DISTINCT extras.model
}

// GetChatStats runs one query per metric in parallel-ish (actually
// serial but cheap — all hit the same indexed (chat_id, id) path).
// Returns zero values for an empty chat rather than ErrNotFound — a
// chat with no messages is valid.
func (r *Repository) GetChatStats(ctx context.Context, chatID uuid.UUID) (*ChatStats, error) {
	s := &ChatStats{ChatID: chatID}
	// One combined query pulls counts + aggregates in a single round-trip.
	// JSONB extras.tokens_in / .tokens_out are optional; COALESCE to 0.
	const q = `
		SELECT
		    COUNT(*) AS total,
		    COUNT(*) FILTER (WHERE role = 'user') AS n_user,
		    COUNT(*) FILTER (WHERE role = 'assistant') AS n_asst,
		    COUNT(*) FILTER (WHERE role = 'system') AS n_sys,
		    COUNT(*) FILTER (WHERE hidden = TRUE) AS n_hidden,
		    COALESCE(SUM((extras->>'tokens_in')::int), 0) AS toks_in,
		    COALESCE(SUM((extras->>'tokens_out')::int), 0) AS toks_out,
		    COALESCE(SUM(jsonb_array_length(COALESCE(swipes, '[]'::jsonb))), 0) AS swipes_total,
		    MIN(created_at) AS first_at,
		    MAX(created_at) AS last_at,
		    COUNT(DISTINCT extras->>'model') FILTER (WHERE extras->>'model' IS NOT NULL AND extras->>'model' != '' AND extras->>'model' != 'greeting') AS models_used
		  FROM nest_messages
		 WHERE chat_id = $1
	`
	var firstAt, lastAt *time.Time
	err := r.pg.QueryRow(ctx, q, chatID).Scan(
		&s.MessagesTotal,
		&s.MessagesUser,
		&s.MessagesAssistant,
		&s.MessagesSystem,
		&s.MessagesHidden,
		&s.TokensInTotal,
		&s.TokensOutTotal,
		&s.SwipesTotal,
		&firstAt,
		&lastAt,
		&s.UniqueModelsUsed,
	)
	if err != nil {
		return nil, fmt.Errorf("chat stats: %w", err)
	}
	s.FirstMessageAt = firstAt
	s.LastMessageAt = lastAt
	return s, nil
}

// SetMessageHidden toggles the silent-message flag. Hidden messages
// still feed into the prompt (preserves context) but the UI greys
// them out. Use case: metadata notes, OOC commentary, scene direction
// the user doesn't want cluttering the visible chat.
func (r *Repository) SetMessageHidden(ctx context.Context, chatID uuid.UUID, messageID int64, hidden bool) error {
	const q = `UPDATE nest_messages SET hidden = $3 WHERE chat_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, chatID, messageID, hidden)
	if err != nil {
		return fmt.Errorf("set hidden: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// maxSwipesPerMessage caps how many alternative variants we retain per row.
// Past this, BeginSwipe slides a window forward: oldest entries fall off,
// swipe_id rebases, and `nest_messages.swipes` never grows unboundedly.
// 20 is plenty for roleplay "let me try that again" loops without turning
// a single row into a megabyte of JSON.
const maxSwipesPerMessage = 20

// RestoreSwipes overwrites the swipes[] and swipe_id columns for a message
// without touching content. Used by the chat import path to faithfully
// reinstate alternate variants from a .jsonl export. The raw JSON is
// forwarded verbatim — callers are expected to have validated shape.
func (r *Repository) RestoreSwipes(ctx context.Context, chatID uuid.UUID, messageID int64, swipesJSON json.RawMessage, swipeID int) error {
	const q = `
		UPDATE nest_messages
		   SET swipes = $3::jsonb, swipe_id = $4
		 WHERE chat_id = $1 AND id = $2
	`
	tag, err := r.pg.Exec(ctx, q, chatID, messageID, string(swipesJSON), swipeID)
	if err != nil {
		return fmt.Errorf("restore swipes: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetMessage fetches a single message by chat + id, for handlers that need
// to inspect an existing row (swipe, edit, delete confirmation).
func (r *Repository) GetMessage(ctx context.Context, chatID uuid.UUID, messageID int64) (*Message, error) {
	const q = `
		SELECT id, chat_id, role, content, swipes, swipe_id, extras, hidden, character_id, swipe_character_ids, created_at
		  FROM nest_messages
		 WHERE chat_id = $1 AND id = $2
	`
	var m Message
	var swipes, extras []byte
	var role string
	err := r.pg.QueryRow(ctx, q, chatID, messageID).Scan(
		&m.ID, &m.ChatID, &role, &m.Content, &swipes, &m.SwipeID, &extras, &m.Hidden, &m.CharacterID, &m.SwipeCharacterIDs, &m.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	m.Role = Role(role)
	m.Swipes = swipes
	m.Extras = extras
	return &m, nil
}

// BeginSwipe prepares a message for a new streamed variant:
//   - captures the current `content` into `swipes[]` if it isn't already
//     there (so we don't lose the pre-swipe version);
//   - appends an empty "" slot to `swipes[]`;
//   - bumps `swipe_id` to the new slot's index;
//   - clears `content` so the streaming loop starts from empty.
//
// The returned int is the new swipe_id. Callers then stream tokens into
// the message via the usual UpdateMessageContent path. FinalizeSwipe
// must be called on done to mirror the final content into swipes[].
func (r *Repository) BeginSwipe(ctx context.Context, chatID uuid.UUID, messageID int64) (int, error) {
	tx, err := r.pg.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var (
		content    string
		swipesRaw  []byte
		swipeID    int
	)
	if err := tx.QueryRow(ctx,
		`SELECT content, swipes, swipe_id FROM nest_messages WHERE chat_id = $1 AND id = $2 FOR UPDATE`,
		chatID, messageID,
	).Scan(&content, &swipesRaw, &swipeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("lock message: %w", err)
	}

	var swipes []string
	if len(swipesRaw) > 0 {
		_ = json.Unmarshal(swipesRaw, &swipes)
	}

	// Seed swipes[] with current content if empty; otherwise ensure the
	// existing swipe_id slot reflects the latest content (covers the case
	// where the current turn was regenerated with replace semantics).
	if len(swipes) == 0 {
		swipes = []string{content}
		swipeID = 0
	} else if swipeID >= 0 && swipeID < len(swipes) {
		swipes[swipeID] = content
	}

	// Window the stored history so `nest_messages` rows don't bloat on
	// heavy regen. Keep the most recent (maxSwipesPerMessage-1) variants
	// so the append below lands at the cap. Oldest entries drop first;
	// swipeID rebased to whichever surviving position still holds the
	// "current" content, or to the last kept slot if the current was
	// aged out (shouldn't happen since we always keep the last N, but
	// belt-and-suspenders).
	if keep := maxSwipesPerMessage - 1; keep > 0 && len(swipes) > keep {
		dropped := len(swipes) - keep
		swipes = swipes[dropped:]
		switch {
		case swipeID >= dropped:
			swipeID -= dropped
		default:
			swipeID = 0
		}
	}

	// Append a fresh empty slot; that's where the new stream lands.
	swipes = append(swipes, "")
	newSwipeID := len(swipes) - 1

	newRaw, err := json.Marshal(swipes)
	if err != nil {
		return 0, fmt.Errorf("marshal swipes: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE nest_messages SET content = '', swipes = $3, swipe_id = $4 WHERE chat_id = $1 AND id = $2`,
		chatID, messageID, newRaw, newSwipeID,
	); err != nil {
		return 0, fmt.Errorf("begin swipe: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return newSwipeID, nil
}

// FinalizeSwipe writes `content` into both `nest_messages.content` AND into
// `swipes[swipe_id]` so the array mirrors the visible state. Called from the
// stream loop's finalization path.
func (r *Repository) FinalizeSwipe(ctx context.Context, chatID uuid.UUID, messageID int64, content string) error {
	// Read current swipes + swipe_id, patch the slot, write back.
	// Atomic via a single UPDATE using jsonb_set against the current swipe_id.
	const q = `
		UPDATE nest_messages
		   SET content = $3,
		       swipes  = jsonb_set(COALESCE(swipes, '[]'::jsonb), ARRAY[swipe_id::text], to_jsonb($3::text), true)
		 WHERE chat_id = $1 AND id = $2
	`
	tag, err := r.pg.Exec(ctx, q, chatID, messageID, content)
	if err != nil {
		return fmt.Errorf("finalize swipe: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SelectSwipe navigates between stored variants: sets swipe_id = targetID,
// copies swipes[targetID] into `content`, returns the resulting message.
// Invalid targetID returns ErrNotFound-style behaviour (no-op + ErrNotFound).
func (r *Repository) SelectSwipe(ctx context.Context, chatID uuid.UUID, messageID int64, targetID int) (*Message, error) {
	tx, err := r.pg.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var swipesRaw []byte
	if err := tx.QueryRow(ctx,
		`SELECT swipes FROM nest_messages WHERE chat_id = $1 AND id = $2 FOR UPDATE`,
		chatID, messageID,
	).Scan(&swipesRaw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock message: %w", err)
	}
	var swipes []string
	if len(swipesRaw) > 0 {
		_ = json.Unmarshal(swipesRaw, &swipes)
	}
	if targetID < 0 || targetID >= len(swipes) {
		return nil, ErrNotFound
	}
	newContent := swipes[targetID]
	if _, err := tx.Exec(ctx,
		`UPDATE nest_messages SET content = $3, swipe_id = $4 WHERE chat_id = $1 AND id = $2`,
		chatID, messageID, newContent, targetID,
	); err != nil {
		return nil, fmt.Errorf("select swipe: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetMessage(ctx, chatID, messageID)
}

// LastAssistantMessage returns the most recent assistant message in a chat,
// or ErrNotFound if the chat has none yet. Used by the regenerate endpoint.
func (r *Repository) LastAssistantMessage(ctx context.Context, chatID uuid.UUID) (*Message, error) {
	const q = `
		SELECT id, chat_id, role, content, swipes, swipe_id, extras, hidden, character_id, swipe_character_ids, created_at
		  FROM nest_messages
		 WHERE chat_id = $1 AND role = 'assistant' AND hidden = FALSE
		 ORDER BY id DESC
		 LIMIT 1
	`
	var m Message
	var swipes, extras []byte
	var role string
	err := r.pg.QueryRow(ctx, q, chatID).Scan(
		&m.ID, &m.ChatID, &role, &m.Content, &swipes, &m.SwipeID, &extras, &m.Hidden, &m.CharacterID, &m.SwipeCharacterIDs, &m.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("last assistant: %w", err)
	}
	m.Role = Role(role)
	m.Swipes = swipes
	m.Extras = extras
	return &m, nil
}
