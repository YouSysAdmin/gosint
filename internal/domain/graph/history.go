// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/attachment"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// MaxChanges is how many of an account's edits of a sketch are kept.
const MaxChanges = 50

const changeSelect = `
SELECT id, sketch_id, user_id, kind, before_state, after_state, undone, created_at
FROM graph_changes`

// RecordChange stores ch as the newest edit of its account on the
// sketch: the redo stack goes, and edits beyond MaxChanges are dropped.
// The snapshots are sealed like the nodes they hold.
func (s *Store) RecordChange(ctx context.Context, ch *graphmodel.Change) error {
	if ch.ID == "" {
		ch.ID = ids.New()
	}

	if ch.CreatedAt.IsZero() {
		ch.CreatedAt = time.Now().UTC()
	}

	kind, err := s.codec.SealText(changeAAD("kind", ch.ID), ch.Kind)
	if err != nil {
		return err
	}

	before, err := database.SealJSON(s.codec, changeAAD("before_state", ch.ID), ch.Before)
	if err != nil {
		return err
	}

	after, err := database.SealJSON(s.codec, changeAAD("after_state", ch.ID), ch.After)
	if err != nil {
		return err
	}

	return s.Tx(ctx, func(tx *database.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM graph_changes WHERE sketch_id = ? AND user_id = ? AND undone`,
			ch.SketchID, ch.UserID); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO graph_changes (id, sketch_id, user_id, kind, before_state, after_state, undone, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			ch.ID, ch.SketchID, ch.UserID, kind, before, after, false, ch.CreatedAt); err != nil {
			return err
		}

		// The ids are UUIDv7, so id order is time order on both engines.
		_, err := tx.Exec(ctx, `
			DELETE FROM graph_changes WHERE sketch_id = ? AND user_id = ? AND id NOT IN (
			  SELECT id FROM graph_changes WHERE sketch_id = ? AND user_id = ? ORDER BY id DESC LIMIT ?)`,
			ch.SketchID, ch.UserID, ch.SketchID, ch.UserID, MaxChanges)

		return err
	})
}

func changeAAD(column, id string) string { return database.AAD("graph_changes", column, id) }

// NextUndo is the account's newest edit of the sketch still in effect,
// NextRedo the oldest one undone. Nil when there is none.
func (s *Store) NextUndo(ctx context.Context, sketchID, userID string) (*graphmodel.Change, error) {
	return s.oneChange(ctx, changeSelect+` WHERE sketch_id = ? AND user_id = ? AND NOT undone ORDER BY id DESC LIMIT 1`, sketchID, userID)
}

// NextRedo is the oldest undone edit, the one to apply again first.
func (s *Store) NextRedo(ctx context.Context, sketchID, userID string) (*graphmodel.Change, error) {
	return s.oneChange(ctx, changeSelect+` WHERE sketch_id = ? AND user_id = ? AND undone ORDER BY id ASC LIMIT 1`, sketchID, userID)
}

func (s *Store) oneChange(ctx context.Context, query string, args ...any) (*graphmodel.Change, error) {
	var (
		ch            graphmodel.Change
		before, after string
	)
	err := s.QueryRow(ctx, query, args...).Scan(&ch.ID, &ch.SketchID, &ch.UserID, &ch.Kind, &before, &after, &ch.Undone, &ch.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if ch.Kind, err = s.codec.OpenText(changeAAD("kind", ch.ID), ch.Kind); err != nil {
		return nil, err
	}

	if err := database.OpenJSON(s.codec, changeAAD("before_state", ch.ID), before, &ch.Before); err != nil {
		return nil, err
	}

	if err := database.OpenJSON(s.codec, changeAAD("after_state", ch.ID), after, &ch.After); err != nil {
		return nil, err
	}

	return &ch, nil
}

// SetUndone marks a change undone, or in effect again.
func (s *Store) SetUndone(ctx context.Context, id string, undone bool) error {
	_, err := s.Exec(ctx, `UPDATE graph_changes SET undone = ? WHERE id = ?`, undone, id)

	return err
}

// Revert writes the sketch from the state in from to the state in to,
// in one transaction: nodes and edges present in from and absent in to
// are deleted, every node and edge of to is written back as it was,
// by its id, and so are the records of the files on them, whose bytes
// the release sweep keeps for a while. A node whose type and key
// another node holds by now, or an edge whose end is gone, is left
// out. The blobs of the deleted nodes' files are let go, for the
// sweep.
func (s *Store) Revert(ctx context.Context, sketchID string, from, to graphmodel.Snapshot) error {
	now := time.Now().UTC()

	return s.Tx(ctx, func(tx *database.Tx) error {
		keep := map[string]bool{}
		for _, n := range to.Nodes {
			keep[n.ID] = true
		}

		var gone []string
		for _, n := range from.Nodes {
			if !keep[n.ID] {
				gone = append(gone, n.ID)
			}
		}

		released, err := releasesOf(ctx, tx, sketchID, gone)
		if err != nil {
			return err
		}

		for chunk := range slices.Chunk(gone, inChunk) {
			if _, err := tx.Exec(ctx, `DELETE FROM nodes WHERE sketch_id = ? AND id IN (`+placeholders(len(chunk))+`)`,
				idArgs(sketchID, chunk)...); err != nil {
				return err
			}
		}

		for _, key := range released {
			if err := attachment.MarkKey(ctx, tx, key, now); err != nil {
				return err
			}
		}

		for _, n := range to.Nodes {
			if err := s.restoreNode(ctx, tx, sketchID, n, now); err != nil {
				return err
			}
		}

		for _, f := range to.Files {
			name, ctype, key, digest, meta, err := attachment.Columns(s.codec, &f)
			if err != nil {
				return err
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO attachments (id, sketch_id, node_id, name, content_type, size, hash, digest_sealed, uploader_id, created_at, metadata)
				SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM nodes WHERE id = ? AND sketch_id = ?)
				ON CONFLICT (id) DO NOTHING`,
				f.ID, sketchID, f.NodeID, name, ctype, f.Size, key, digest, f.UploaderID, f.CreatedAt, meta, f.NodeID, sketchID); err != nil {
				return err
			}
		}

		keepEdge := map[string]bool{}
		for _, e := range to.Edges {
			keepEdge[graphmodel.EdgeKey(e)] = true
		}

		for _, e := range from.Edges {
			if keepEdge[graphmodel.EdgeKey(e)] {
				continue
			}

			id, err := s.edgeIDByEnds(ctx, tx, sketchID, e.SourceID, e.TargetID, e.Label)
			if err != nil {
				return err
			}

			if _, err := tx.Exec(ctx, `DELETE FROM edges WHERE sketch_id = ? AND id = ?`, sketchID, id); err != nil {
				return err
			}
		}

		for _, e := range to.Edges {
			if err := s.restoreEdge(ctx, tx, sketchID, e, now); err != nil {
				return err
			}
		}

		return touch(ctx, tx, sketchID, now)
	})
}

// releasesOf is the distinct blobs the files of nodeIDs name, as the
// release marks name them.
func releasesOf(ctx context.Context, q querier, sketchID string, nodeIDs []string) ([]string, error) {
	seen := map[string]bool{}

	var out []string
	for chunk := range slices.Chunk(nodeIDs, inChunk) {
		err := func() error {
			rows, err := q.Query(ctx, `SELECT hash FROM attachments WHERE sketch_id = ? AND node_id IN (`+placeholders(len(chunk))+`)`,
				idArgs(sketchID, chunk)...)
			if err != nil {
				return err
			}

			defer rows.Close() //nolint:errcheck // read-only

			for rows.Next() {
				var key string
				if err := rows.Scan(&key); err != nil {
					return err
				}

				if !seen[key] {
					seen[key] = true
					out = append(out, key)
				}
			}

			return rows.Err()
		}()
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// PruneChanges drops the journal entries older than the moment, on
// every sketch: an edit older than files.retain_deleted could bring
// back nodes whose files are gone.
func (s *Store) PruneChanges(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.Exec(ctx, `DELETE FROM graph_changes WHERE created_at < ?`, before.UTC())
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// restoreNode writes n back under its id, inserting it when it is gone.
// A node whose type and key another node took since is left as it is,
// so one entity added by a later edit never blocks the whole revert.
func (s *Store) restoreNode(ctx context.Context, q querier, sketchID string, n graphmodel.Node, now time.Time) error {
	n.SketchID = sketchID
	cols, err := s.sealNode(n)
	if err != nil {
		return err
	}

	var taken int
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM nodes WHERE sketch_id = ? AND node_type = ? AND node_key = ? AND id <> ?`,
		sketchID, cols.typ, cols.key, n.ID).Scan(&taken); err != nil {
		return err
	}

	if taken > 0 {
		return nil
	}

	res, err := q.Exec(ctx, `
		UPDATE nodes SET `+nodeSets+`, size = ?, x = ?, y = ?,
		       version = version + 1, updated_at = ?
		WHERE id = ? AND sketch_id = ?`,
		append(cols.args(), n.Size, n.X, n.Y, now, n.ID, sketchID)...)
	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		created := n.CreatedAt
		if created.IsZero() {
			created = now
		}

		res, err = q.Exec(ctx, `
			INSERT INTO nodes (id, sketch_id, `+nodeColumns+`, size, x, y, version, created_at, updated_at)
			VALUES (?, ?, `+placeholders(nodeColumnCount)+`, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (sketch_id, node_type, node_key) DO NOTHING`,
			append(append([]any{n.ID, sketchID}, cols.args()...), n.Size, n.X, n.Y, 1, created, now)...)
		if err != nil {
			return err
		}

		if affected, err = res.RowsAffected(); err != nil || affected == 0 {
			return err
		}
	}

	return s.writeGrams(ctx, q, n)
}

// restoreEdge writes e back, under its own id when the relationship is
// gone, over the one with the same ends and label otherwise. An edge
// whose end is not a node any more is left out, and one whose id now
// names another relationship gets a fresh id.
func (s *Store) restoreEdge(ctx context.Context, q querier, sketchID string, e graphmodel.Edge, now time.Time) error {
	for _, end := range []string{e.SourceID, e.TargetID} {
		var n int
		if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM nodes WHERE sketch_id = ? AND id = ?`, sketchID, end).Scan(&n); err != nil {
			return err
		}

		if n == 0 {
			return nil
		}
	}

	// The columns are sealed under the row's id, so the relationship
	// that is there keeps its own.
	there, err := s.edgeIDByEnds(ctx, q, sketchID, e.SourceID, e.TargetID, e.Label)
	if err != nil {
		return err
	}

	if there != "" {
		cur, err := s.getEdge(ctx, q, sketchID, there)
		if err != nil || cur == nil {
			return err
		}

		cur.Properties = nil
		_, err = s.writeEdge(ctx, q, cur, nil, orEmpty(e.Properties), now)

		return err
	}

	id := e.ID
	if id != "" {
		other, err := s.getEdge(ctx, q, sketchID, id)
		if err != nil {
			return err
		}

		if other != nil && (other.SourceID != e.SourceID || other.TargetID != e.TargetID || other.Label != e.Label) {
			id = ""
		}
	}

	if id == "" {
		id = ids.New()
	}

	created := e.CreatedAt
	if created.IsZero() {
		created = now
	}

	props, err := database.SealJSON(s.codec, edgeAAD(id), orEmpty(e.Properties))
	if err != nil {
		return err
	}

	label, sealed, err := s.sealLabel(id, e.Label)
	if err != nil {
		return err
	}

	_, err = q.Exec(ctx, `
		INSERT INTO edges (id, sketch_id, source_id, target_id, label, label_sealed, properties, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (sketch_id, source_id, target_id, label) DO NOTHING`,
		id, sketchID, e.SourceID, e.TargetID, label, sealed, props, created, now)

	return err
}

// NodeByKey is the node of the sketch with that type and key, nil when
// there is none.
func (s *Store) NodeByKey(ctx context.Context, sketchID, typ, key string) (*graphmodel.Node, error) {
	n, err := s.byKey(ctx, s, sketchID, typ, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return n, err
}

// EdgesIn is the edges of the sketch among ids.
func (s *Store) EdgesIn(ctx context.Context, sketchID string, ids []string) ([]graphmodel.Edge, error) {
	var out []graphmodel.Edge
	for chunk := range slices.Chunk(ids, inChunk) {
		edges, err := s.collectEdges(s.Query(ctx, edgeSelect+` WHERE sketch_id = ? AND id IN (`+placeholders(len(chunk))+`) ORDER BY id`,
			idArgs(sketchID, chunk)...))
		if err != nil {
			return nil, err
		}

		out = append(out, edges...)
	}

	return out, nil
}

// EdgesTouching is every edge with an end among nodeIDs, each once.
func (s *Store) EdgesTouching(ctx context.Context, sketchID string, nodeIDs []string) ([]graphmodel.Edge, error) {
	seen := map[string]bool{}

	var out []graphmodel.Edge
	for chunk := range slices.Chunk(nodeIDs, inChunk) {
		marks := placeholders(len(chunk))
		args := append(idArgs(sketchID, chunk), toAny(chunk)...)
		edges, err := s.collectEdges(s.Query(ctx, edgeSelect+` WHERE sketch_id = ? AND (source_id IN (`+marks+`) OR target_id IN (`+marks+`)) ORDER BY id`, args...))
		if err != nil {
			return nil, err
		}

		for _, e := range edges {
			if !seen[e.ID] {
				seen[e.ID] = true
				out = append(out, e)
			}
		}
	}

	return out, nil
}

// EdgeByEnds is the edge with these ends and label, nil when none.
func (s *Store) EdgeByEnds(ctx context.Context, sketchID, source, target, label string) (*graphmodel.Edge, error) {
	id, err := s.edgeIDByEnds(ctx, s, sketchID, source, target, label)
	if err != nil || id == "" {
		return nil, err
	}

	return s.getEdge(ctx, s, sketchID, id)
}

func toAny(ids []string) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, id)
	}

	return out
}
