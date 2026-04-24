package chats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// ListChats returns a user's chats, most-recently-active first.
// The `last_message_at` is derived from the latest nest_messages row.
func (r *Repository) ListChats(ctx context.Context, userID uuid.UUID) ([]Chat, error) {
	const q = `
		SELECT
		    c.id, c.user_id, c.character_id, COALESCE(ch.name, '') AS character_name,
		    c.character_ids,
		    c.name, c.chat_metadata, c.created_at, c.updated_at,
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
			&c.Name, &meta, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt,
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
		    c.name, c.chat_metadata, c.created_at, c.updated_at,
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
		&c.Name, &meta, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt,
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
