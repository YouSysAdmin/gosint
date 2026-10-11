// Gosint, Copyright (c) 2026 YouSysAdmin

package chat

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	chatmodel "github.com/yousysadmin/gosint/internal/models/chat"
)

// Store persists chats and their messages, the titles, the turns and
// what they were about sealed at rest.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the chat store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func titleAAD(id string) string { return database.AAD("chats", "title", id) }

func messageAAD(column, id string) string { return database.AAD("chat_messages", column, id) }

const chatSelect = `SELECT id, sketch_id, owner_id, title, created_at, updated_at FROM chats`

func (s *Store) scanChat(row interface{ Scan(...any) error }) (*chatmodel.Chat, error) {
	var c chatmodel.Chat
	if err := row.Scan(&c.ID, &c.SketchID, &c.OwnerID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}

	title, err := s.codec.OpenText(titleAAD(c.ID), c.Title)
	if err != nil {
		return nil, err
	}

	c.Title = title

	return &c, nil
}

// Create inserts c.
func (s *Store) Create(ctx context.Context, c *chatmodel.Chat) error {
	title, err := s.codec.SealText(titleAAD(c.ID), c.Title)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO chats (id, sketch_id, owner_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.SketchID, c.OwnerID, title, c.CreatedAt, c.UpdatedAt)

	return err
}

// Get returns one of the owner's chats of the sketch, nil when absent.
func (s *Store) Get(ctx context.Context, sketchID, ownerID, id string) (*chatmodel.Chat, error) {
	c, err := s.scanChat(s.QueryRow(ctx, chatSelect+` WHERE id = ? AND sketch_id = ? AND owner_id = ?`, id, sketchID, ownerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return c, err
}

// List lists the owner's chats of the sketch, latest first.
func (s *Store) List(ctx context.Context, sketchID, ownerID string) ([]chatmodel.Chat, error) {
	rows, err := s.Query(ctx, chatSelect+` WHERE sketch_id = ? AND owner_id = ? ORDER BY updated_at DESC, id DESC`, sketchID, ownerID)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []chatmodel.Chat
	for rows.Next() {
		c, err := s.scanChat(rows)
		if err != nil {
			return nil, err
		}

		c.Entities = []string{}
		out = append(out, *c)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, s.fillEntities(ctx, out)
}

// fillEntities sets each chat's Entities from the contexts of its turns,
// in one query over the sketch's chats.
func (s *Store) fillEntities(ctx context.Context, chats []chatmodel.Chat) error {
	if len(chats) == 0 {
		return nil
	}

	args := make([]any, 0, len(chats))
	index := make(map[string]int, len(chats))
	for i, c := range chats {
		args = append(args, c.ID)
		index[c.ID] = i
	}

	rows, err := s.Query(ctx, `SELECT id, chat_id, context FROM chat_messages WHERE role = 'user' AND chat_id IN (`+
		strings.TrimSuffix(strings.Repeat("?, ", len(chats)), ", ")+`)`, args...)
	if err != nil {
		return err
	}

	defer rows.Close() //nolint:errcheck // read-only

	seen := map[string]map[string]bool{}
	for rows.Next() {
		var id, chatID, raw string
		if err := rows.Scan(&id, &chatID, &raw); err != nil {
			return err
		}

		var ids []string
		if err := database.OpenJSON(s.codec, messageAAD("context", id), raw, &ids); err != nil {
			return err
		}

		i := index[chatID]
		if seen[chatID] == nil {
			seen[chatID] = map[string]bool{}
		}

		for _, node := range ids {
			if !seen[chatID][node] {
				seen[chatID][node] = true
				chats[i].Entities = append(chats[i].Entities, node)
			}
		}
	}

	return rows.Err()
}

// Delete removes one of the owner's chats with its messages. False when
// there was none.
func (s *Store) Delete(ctx context.Context, sketchID, ownerID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM chats WHERE id = ? AND sketch_id = ? AND owner_id = ?`, id, sketchID, ownerID)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// Touch records activity and, when given, a title.
func (s *Store) Touch(ctx context.Context, id, title string, at time.Time) error {
	sealed := ""
	if title != "" {
		var err error
		if sealed, err = s.codec.SealText(titleAAD(id), title); err != nil {
			return err
		}
	}

	_, err := s.Exec(ctx, `UPDATE chats SET title = CASE WHEN ? = '' THEN title ELSE ? END, updated_at = ? WHERE id = ?`,
		sealed, sealed, at, id)

	return err
}

// AddMessage appends m to its chat.
func (s *Store) AddMessage(ctx context.Context, m *chatmodel.Message) error {
	if m.Context == nil {
		m.Context = []string{}
	}

	content, err1 := s.codec.SealText(messageAAD("content", m.ID), m.Content)
	about, err2 := database.SealJSON(s.codec, messageAAD("context", m.ID), m.Context)
	if err := errors.Join(err1, err2); err != nil {
		return err
	}

	_, err := s.Exec(ctx, `
		INSERT INTO chat_messages (id, chat_id, role, content, context, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, m.ChatID, m.Role, content, about, m.CreatedAt)

	return err
}

// Messages returns the chat's last limit turns, oldest first.
func (s *Store) Messages(ctx context.Context, chatID string, limit int) ([]chatmodel.Message, error) {
	rows, err := s.Query(ctx, `
		SELECT id, chat_id, role, content, context, created_at FROM chat_messages
		WHERE chat_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []chatmodel.Message
	for rows.Next() {
		var (
			m      chatmodel.Message
			rawCtx string
		)

		if err := rows.Scan(&m.ID, &m.ChatID, &m.Role, &m.Content, &rawCtx, &m.CreatedAt); err != nil {
			return nil, err
		}

		content, err := s.codec.OpenText(messageAAD("content", m.ID), m.Content)
		if err != nil {
			return nil, err
		}

		m.Content = content
		if err := database.OpenJSON(s.codec, messageAAD("context", m.ID), rawCtx, &m.Context); err != nil {
			return nil, err
		}

		out = append(out, m)
	}

	slices.Reverse(out)

	return out, rows.Err()
}
