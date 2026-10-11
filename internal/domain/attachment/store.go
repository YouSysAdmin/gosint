// Gosint, Copyright (c) 2026 YouSysAdmin

package attachment

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/blob/sealed"
	"github.com/yousysadmin/gosint/internal/database"
	amodel "github.com/yousysadmin/gosint/internal/models/attachment"
)

// Store persists the file records. The name, the metadata and the
// sha256 are sealed at rest, hash holds a keyed hash of the sha256, the
// name the sealed bytes are stored under.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

const attachmentSelect = `
SELECT id, sketch_id, node_id, name, content_type, size, hash, digest_sealed, uploader_id, created_at, metadata FROM attachments`

func aad(column, id string) string { return database.AAD("attachments", column, id) }

// Key is the keyed hash a record holds for a blob's sha256.
func Key(codec database.Codec, digest string) (string, error) {
	return codec.Index(sealed.NamePurpose, digest)
}

// Columns is a record's sealed columns: name, type, key, sealed sha256
// and metadata, empty for metadata not read yet.
func Columns(codec database.Codec, a *amodel.Attachment) (name, ctype, key, digest, meta string, err error) {
	var errs [5]error
	name, errs[0] = codec.SealText(aad("name", a.ID), a.Name)
	ctype, errs[4] = codec.SealText(aad("content_type", a.ID), a.ContentType)
	key, errs[1] = Key(codec, a.Hash)
	digest, errs[2] = codec.SealText(aad("digest_sealed", a.ID), a.Hash)
	if a.Metadata != nil {
		meta, errs[3] = database.SealJSON(codec, aad("metadata", a.ID), a.Metadata)
	}

	return name, ctype, key, digest, meta, errors.Join(errs[:]...)
}

func (s *Store) scanAttachment(row interface{ Scan(...any) error }) (*amodel.Attachment, error) {
	var (
		a                 amodel.Attachment
		key, digest, meta string
	)
	if err := row.Scan(&a.ID, &a.SketchID, &a.NodeID, &a.Name, &a.ContentType, &a.Size, &key, &digest, &a.UploaderID, &a.CreatedAt, &meta); err != nil {
		return nil, err
	}

	name, err1 := s.codec.OpenText(aad("name", a.ID), a.Name)
	ctype, err2 := s.codec.OpenText(aad("content_type", a.ID), a.ContentType)
	if err := errors.Join(err1, err2); err != nil {
		return nil, err
	}

	var err error
	a.Name, a.ContentType = name, ctype
	if a.Hash, err = s.codec.OpenText(aad("digest_sealed", a.ID), digest); err != nil {
		return nil, err
	}

	if meta != "" {
		a.Metadata = map[string]any{}
		if err := database.OpenJSON(s.codec, aad("metadata", a.ID), meta, &a.Metadata); err != nil {
			return nil, err
		}
	}

	return &a, nil
}

func (s *Store) collect(rows *sql.Rows, err error) ([]amodel.Attachment, error) {
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []amodel.Attachment
	for rows.Next() {
		a, err := s.scanAttachment(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *a)
	}

	return out, rows.Err()
}

// Create inserts a.
func (s *Store) Create(ctx context.Context, a *amodel.Attachment) error {
	return s.insert(ctx, a, "")
}

func (s *Store) insert(ctx context.Context, a *amodel.Attachment, onConflict string) error {
	name, ctype, key, digest, meta, err := Columns(s.codec, a)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO attachments (id, sketch_id, node_id, name, content_type, size, hash, digest_sealed, uploader_id, created_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`+onConflict,
		a.ID, a.SketchID, a.NodeID, name, ctype, a.Size, key, digest, a.UploaderID, a.CreatedAt, meta)

	return err
}

// Get returns one file of the sketch, nil when absent.
func (s *Store) Get(ctx context.Context, sketchID, id string) (*amodel.Attachment, error) {
	a, err := s.scanAttachment(s.QueryRow(ctx, attachmentSelect+` WHERE id = ? AND sketch_id = ?`, id, sketchID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return a, err
}

// ListForNode is a node's files, oldest first.
func (s *Store) ListForNode(ctx context.Context, sketchID, nodeID string) ([]amodel.Attachment, error) {
	return s.collect(s.Query(ctx, attachmentSelect+` WHERE sketch_id = ? AND node_id = ? ORDER BY created_at, id`, sketchID, nodeID))
}

// ListForSketch is every file of the sketch, by node then age.
func (s *Store) ListForSketch(ctx context.Context, sketchID string) ([]amodel.Attachment, error) {
	return s.collect(s.Query(ctx, attachmentSelect+` WHERE sketch_id = ? ORDER BY node_id, created_at, id`, sketchID))
}

// Delete removes one record and reports whether it was there.
func (s *Store) Delete(ctx context.Context, sketchID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM attachments WHERE id = ? AND sketch_id = ?`, id, sketchID)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// HashesOfNodes lists the distinct sha256 the files of nodes name, read
// before the nodes go so the blobs can be released after.
func (s *Store) HashesOfNodes(ctx context.Context, sketchID string, nodeIDs []string) ([]string, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	args := []any{sketchID}
	for _, id := range nodeIDs {
		args = append(args, id)
	}

	return s.digests(ctx, `SELECT id, hash, digest_sealed FROM attachments WHERE sketch_id = ? AND node_id IN (`+
		strings.TrimSuffix(strings.Repeat("?, ", len(nodeIDs)), ", ")+`)`, args...)
}

// HashesOfSketch lists the distinct sha256 of every file of the sketch.
func (s *Store) HashesOfSketch(ctx context.Context, sketchID string) ([]string, error) {
	return s.digests(ctx, `SELECT id, hash, digest_sealed FROM attachments WHERE sketch_id = ?`, sketchID)
}

func (s *Store) digests(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []string
	for rows.Next() {
		var id, key, digest string
		if err := rows.Scan(&id, &key, &digest); err != nil {
			return nil, err
		}

		d, err := s.codec.OpenText(aad("digest_sealed", id), digest)
		if err != nil {
			return nil, err
		}

		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}

	return out, rows.Err()
}

// CountByHash is how many records across every sketch name the blob of
// the sha256.
func (s *Store) CountByHash(ctx context.Context, digest string) (int, error) {
	key, err := Key(s.codec, digest)
	if err != nil {
		return 0, err
	}

	return s.CountByKey(ctx, key)
}

// CountByKey is how many records hold the keyed hash.
func (s *Store) CountByKey(ctx context.Context, key string) (int, error) {
	var n int
	err := s.QueryRow(ctx, `SELECT COUNT(*) FROM attachments WHERE hash = ?`, key).Scan(&n)

	return n, err
}

// ListForNodes is the files of the nodes among ids, by node then age.
func (s *Store) ListForNodes(ctx context.Context, sketchID string, nodeIDs []string) ([]amodel.Attachment, error) {
	var out []amodel.Attachment
	for chunk := range slices.Chunk(nodeIDs, 500) {
		args := []any{sketchID}
		for _, id := range chunk {
			args = append(args, id)
		}

		part, err := s.collect(s.Query(ctx, attachmentSelect+` WHERE sketch_id = ? AND node_id IN (`+
			strings.TrimSuffix(strings.Repeat("?, ", len(chunk)), ", ")+`) ORDER BY node_id, created_at, id`, args...))
		if err != nil {
			return nil, err
		}

		out = append(out, part...)
	}

	return out, nil
}

// Restore writes a's record back under its id, when no record has
// that id. For an undo, after its node returned.
func (s *Store) Restore(ctx context.Context, a amodel.Attachment) error {
	return s.insert(ctx, &a, ` ON CONFLICT (id) DO NOTHING`)
}

// MarkReleased notes that no record names the blobs of these sha256 as
// of now, for the sweep to delete once files.retain_deleted has passed.
func (s *Store) MarkReleased(ctx context.Context, digests []string, now time.Time) error {
	for _, digest := range digests {
		key, err := Key(s.codec, digest)
		if err != nil {
			return err
		}

		if err := MarkKey(ctx, s, key, now); err != nil {
			return err
		}
	}

	return nil
}

// Execer is what both Base and Tx offer for a write.
type Execer interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// MarkKey writes one release mark, by the keyed hash the records hold.
func MarkKey(ctx context.Context, q Execer, key string, now time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO blob_releases (hash, released_at) VALUES (?, ?)
		ON CONFLICT (hash) DO UPDATE SET released_at = excluded.released_at`, key, now.UTC())

	return err
}

// ReleasedBefore lists the keyed hashes of the blobs let go before the
// moment.
func (s *Store) ReleasedBefore(ctx context.Context, before time.Time) ([]string, error) {
	rows, err := s.Query(ctx, `SELECT hash FROM blob_releases WHERE released_at < ? ORDER BY released_at`, before.UTC())
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}

		out = append(out, key)
	}

	return out, rows.Err()
}

// Unmark forgets a release, the blob deleted or named again.
func (s *Store) Unmark(ctx context.Context, key string) error {
	_, err := s.Exec(ctx, `DELETE FROM blob_releases WHERE hash = ?`, key)

	return err
}
