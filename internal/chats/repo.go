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
			&c.Name, &meta, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt,
		); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		c.Metadata = meta
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetChat returns one chat by id, scoped to the user.
func (r *Repository) GetChat(ctx context.Context, userID, id uuid.UUID) (*Chat, error) {
	const q = `
		SELECT
		    c.id, c.user_id, c.character_id, COALESCE(ch.name, '') AS character_name,
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
		&c.Name, &meta, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get chat: %w", err)
	}
	c.Metadata = meta
	return &c, nil
}

func (r *Repository) CreateChat(ctx context.Context, in CreateChatInput) (*Chat, error) {
	if in.Name == "" {
		in.Name = "New chat"
	}
	if in.Metadata == nil {
		in.Metadata = []byte("{}")
	}
	const q = `
		INSERT INTO nest_chats (user_id, character_id, name, chat_metadata)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	var (
		id               uuid.UUID
		createdAt, updAt time.Time
	)
	if err := r.pg.QueryRow(ctx, q, in.UserID, in.CharacterID, in.Name, in.Metadata).
		Scan(&id, &createdAt, &updAt); err != nil {
		return nil, fmt.Errorf("insert chat: %w", err)
	}
	return &Chat{
		ID:            id,
		UserID:        in.UserID,
		CharacterID:   in.CharacterID,
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
		SELECT id, chat_id, role, content, swipes, swipe_id, extras, hidden, created_at
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
			&m.ID, &m.ChatID, &role, &m.Content, &swipes, &m.SwipeID, &extras, &m.Hidden, &m.CreatedAt,
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
func (r *Repository) AppendMessage(ctx context.Context, chatID uuid.UUID, role Role, content string, extras *MessageExtras) (*Message, error) {
	extrasJSON := []byte("{}")
	if extras != nil {
		b, err := json.Marshal(extras)
		if err != nil {
			return nil, fmt.Errorf("marshal extras: %w", err)
		}
		extrasJSON = b
	}

	const q = `
		INSERT INTO nest_messages (chat_id, role, content, extras)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`
	var (
		id        int64
		createdAt time.Time
	)
	if err := r.pg.QueryRow(ctx, q, chatID, string(role), content, extrasJSON).
		Scan(&id, &createdAt); err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}

	// Also bump the chat's updated_at so it floats to the top of ListChats.
	if _, err := r.pg.Exec(ctx, `UPDATE nest_chats SET updated_at = NOW() WHERE id = $1`, chatID); err != nil {
		// Non-fatal — log via caller.
		_ = err
	}

	return &Message{
		ID:        id,
		ChatID:    chatID,
		Role:      role,
		Content:   content,
		Extras:    extrasJSON,
		CreatedAt: createdAt,
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

// LastAssistantMessage returns the most recent assistant message in a chat,
// or ErrNotFound if the chat has none yet. Used by the regenerate endpoint.
func (r *Repository) LastAssistantMessage(ctx context.Context, chatID uuid.UUID) (*Message, error) {
	const q = `
		SELECT id, chat_id, role, content, swipes, swipe_id, extras, hidden, created_at
		  FROM nest_messages
		 WHERE chat_id = $1 AND role = 'assistant' AND hidden = FALSE
		 ORDER BY id DESC
		 LIMIT 1
	`
	var m Message
	var swipes, extras []byte
	var role string
	err := r.pg.QueryRow(ctx, q, chatID).Scan(
		&m.ID, &m.ChatID, &role, &m.Content, &swipes, &m.SwipeID, &extras, &m.Hidden, &m.CreatedAt,
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
