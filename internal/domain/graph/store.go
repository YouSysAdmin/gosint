// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/database"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// casAttempts bounds the retries of a property merge that lost a race.
const casAttempts = 5

// errLostRace is a node that changed between reading and writing it.
var errLostRace = errors.New("graph: node changed concurrently")

// Labeler computes the label of a node of typ from its properties, ""
// when it cannot. A merge recomputes the label from the merged
// properties, so what was learned later shows.
type Labeler func(typ string, props map[string]any) string

// Store persists the nodes and edges of sketches. A node's key, label,
// properties, metadata and picture and an edge's properties are sealed
// at rest by the codec, the key matched through a keyed hash and the
// label searched through node_grams.
type Store struct {
	database.Base
	relabel Labeler
	codec   database.Codec
}

// NewStore binds the graph store to db. relabel may be nil, and then a
// merged node keeps its label.
func NewStore(db database.Database, relabel Labeler, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), relabel: relabel, codec: codec}
}

// keyOf is the identity of n, its label when no key was computed.
func keyOf(n graphmodel.Node) string { return cmp.Or(n.Key, n.Label) }

// querier is what both Base and Tx offer.
type querier interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) *sql.Row
}

const nodeSelect = `
SELECT id, sketch_id, ` + nodeColumns + `, size, x, y, version, created_at, updated_at
FROM nodes`

const edgeSelect = `
SELECT id, sketch_id, source_id, target_id, label, label_sealed, properties, created_at, updated_at
FROM edges`

func (s *Store) scanNode(row interface{ Scan(...any) error }) (*graphmodel.Node, error) {
	var (
		n    graphmodel.Node
		cols nodeCols
		size sql.NullInt64
		x, y sql.NullFloat64
	)

	dest := append(append([]any{&n.ID, &n.SketchID}, cols.dest()...), &size, &x, &y, &n.Version, &n.CreatedAt, &n.UpdatedAt)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}

	if err := s.openNode(&n, cols); err != nil {
		return nil, err
	}

	if size.Valid {
		n.Size = new(int(size.Int64))
	}

	if x.Valid {
		n.X = new(x.Float64)
	}

	if y.Valid {
		n.Y = new(y.Float64)
	}

	return &n, nil
}

func (s *Store) scanEdge(row interface{ Scan(...any) error }) (*graphmodel.Edge, error) {
	var (
		e                    graphmodel.Edge
		label, sealed, props string
	)

	err := row.Scan(&e.ID, &e.SketchID, &e.SourceID, &e.TargetID, &label, &sealed, &props, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if e.Label, err = s.openLabel(e.ID, sealed); err != nil {
		return nil, err
	}

	if err := database.OpenJSON(s.codec, edgeAAD(e.ID), props, &e.Properties); err != nil {
		return nil, err
	}

	return &e, nil
}

func (s *Store) collectNodes(rows *sql.Rows, err error) ([]graphmodel.Node, error) {
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []graphmodel.Node
	for rows.Next() {
		n, err := s.scanNode(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *n)
	}

	return out, rows.Err()
}

func (s *Store) collectEdges(rows *sql.Rows, err error) ([]graphmodel.Edge, error) {
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []graphmodel.Edge
	for rows.Next() {
		e, err := s.scanEdge(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *e)
	}

	return out, rows.Err()
}

func (s *Store) getNode(ctx context.Context, q querier, sketchID, id string) (*graphmodel.Node, error) {
	n, err := s.scanNode(q.QueryRow(ctx, nodeSelect+` WHERE sketch_id = ? AND id = ?`, sketchID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return n, err
}

func (s *Store) getEdge(ctx context.Context, q querier, sketchID, id string) (*graphmodel.Edge, error) {
	e, err := s.scanEdge(q.QueryRow(ctx, edgeSelect+` WHERE sketch_id = ? AND id = ?`, sketchID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return e, err
}

// touch bumps the sketch's updated_at with the graph write it belongs to.
func touch(ctx context.Context, q querier, sketchID string, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE sketches SET updated_at = ? WHERE id = ?`, now, sketchID)

	return err
}

// Graph returns every node and edge of the sketch.
func (s *Store) Graph(ctx context.Context, sketchID string) ([]graphmodel.Node, []graphmodel.Edge, error) {
	nodes, err := s.collectNodes(s.Query(ctx, nodeSelect+` WHERE sketch_id = ? ORDER BY created_at, id`, sketchID))
	if err != nil {
		return nil, nil, err
	}

	edges, err := s.collectEdges(s.Query(ctx, edgeSelect+` WHERE sketch_id = ? ORDER BY created_at, id`, sketchID))
	if err != nil {
		return nil, nil, err
	}

	return nodes, edges, nil
}

// Node returns one node of the sketch.
func (s *Store) Node(ctx context.Context, sketchID, id string) (*graphmodel.Node, error) {
	return s.getNode(ctx, s, sketchID, id)
}

// inChunk bounds one IN list, under every engine's parameter limit.
const inChunk = 500

// placeholders is n comma-separated "?".
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// idArgs is sketchID followed by ids, for a query over an IN list.
func idArgs(sketchID string, ids []string) []any {
	args := make([]any, 0, len(ids)+1)
	args = append(args, sketchID)
	for _, id := range ids {
		args = append(args, id)
	}

	return args
}

// NodesIn returns the nodes of the sketch among ids, one query per
// chunk of ids. An id that is not a node is simply absent.
func (s *Store) NodesIn(ctx context.Context, sketchID string, ids []string) ([]graphmodel.Node, error) {
	var out []graphmodel.Node
	for chunk := range slices.Chunk(ids, inChunk) {
		nodes, err := s.collectNodes(s.Query(ctx, nodeSelect+` WHERE sketch_id = ? AND id IN (`+placeholders(len(chunk))+`) ORDER BY created_at, id`,
			idArgs(sketchID, chunk)...))
		if err != nil {
			return nil, err
		}

		out = append(out, nodes...)
	}

	return out, nil
}

// Import writes nodes and the edges among them in one transaction, the
// edges by index into nodes. An edge whose end was not written, that
// duplicates another or that was already there, is not counted.
func (s *Store) Import(ctx context.Context, sketchID string, nodes []graphmodel.Node, edges []graphmodel.EdgeRef) (written []graphmodel.Node, created int, err error) {
	now := time.Now().UTC()
	written = make([]graphmodel.Node, 0, len(nodes))
	err = s.Tx(ctx, func(tx *database.Tx) error {
		for _, in := range nodes {
			n, err := s.upsertNode(ctx, tx, sketchID, in, now)
			if err != nil {
				return err
			}

			written = append(written, *n)
		}

		for _, e := range edges {
			if e.Source < 0 || e.Source >= len(written) || e.Target < 0 || e.Target >= len(written) {
				continue
			}

			edge, isNew, err := s.upsertEdgeNew(ctx, tx, sketchID, graphmodel.Edge{
				SourceID: written[e.Source].ID, TargetID: written[e.Target].ID, Label: cmp.Or(e.Label, "RELATED_TO"),
				Properties: e.Properties,
			}, now)
			if err != nil {
				return err
			}

			if edge != nil && isNew {
				created++
			}
		}

		return touch(ctx, tx, sketchID, now)
	})
	if err != nil {
		return nil, 0, err
	}

	return written, created, nil
}

// UpsertNodes creates or merges each node, all in one transaction.
func (s *Store) UpsertNodes(ctx context.Context, sketchID string, nodes []graphmodel.Node) ([]graphmodel.Node, error) {
	now := time.Now().UTC()
	out := make([]graphmodel.Node, 0, len(nodes))
	err := s.Tx(ctx, func(tx *database.Tx) error {
		for _, in := range nodes {
			n, err := s.upsertNode(ctx, tx, sketchID, in, now)
			if err != nil {
				return err
			}

			out = append(out, *n)
		}

		return touch(ctx, tx, sketchID, now)
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// upsertNode inserts in, or merges it into the node with the same type
// and key.
//
// The merge happens in Go, since the two engines share no JSON merge
// operator, and is written back only if the row still carries the
// version it was read at.
func (s *Store) upsertNode(ctx context.Context, q querier, sketchID string, in graphmodel.Node, now time.Time) (*graphmodel.Node, error) {
	in.Key = keyOf(in)
	for range casAttempts {
		cur, err := s.byKey(ctx, q, sketchID, in.Type, in.Key)
		if errors.Is(err, sql.ErrNoRows) {
			n, err := s.insertNode(ctx, q, sketchID, in, now)
			if errors.Is(err, errLostRace) {
				continue
			}

			return n, err
		}

		if err != nil {
			return nil, err
		}

		next := *cur
		next.Properties = mergeMaps(cur.Properties, in.Properties)
		next.Metadata = mergeMaps(cur.Metadata, keepFirstOrigin(cur.Metadata, in.Metadata))
		next.Style = mergeStyle(cur.Style, in.Style)
		next.X = cmp.Or(in.X, cur.X)
		next.Y = cmp.Or(in.Y, cur.Y)
		next.Label = s.labelFor(next, cmp.Or(in.Label, cur.Label))

		err = s.writeNode(ctx, q, &next, cur.Label, now)
		if errors.Is(err, errLostRace) {
			continue
		}

		if err != nil {
			return nil, err
		}

		return &next, nil
	}

	return nil, fmt.Errorf("upsert node %s/%s: %w", in.Type, in.Label, errLostRace)
}

// labelFor recomputes n's label from its merged properties, falling
// back to fallback.
func (s *Store) labelFor(n graphmodel.Node, fallback string) string {
	if s.relabel == nil {
		return fallback
	}

	return cmp.Or(s.relabel(n.Type, maps.Clone(n.Properties)), fallback)
}

// insertNode inserts a new node. errLostRace means another writer
// created the same (type, key) first.
func (s *Store) insertNode(ctx context.Context, q querier, sketchID string, in graphmodel.Node, now time.Time) (*graphmodel.Node, error) {
	n := in
	n.ID = ids.New()
	n.SketchID = sketchID
	n.Version = 1
	n.CreatedAt = now
	n.UpdatedAt = now
	n.Properties = orEmpty(in.Properties)
	n.Metadata = orEmpty(in.Metadata)
	cols, err := s.sealNode(n)
	if err != nil {
		return nil, err
	}

	res, err := q.Exec(ctx, `
		INSERT INTO nodes (id, sketch_id, `+nodeColumns+`, size, x, y, version, created_at, updated_at)
		VALUES (?, ?, `+placeholders(nodeColumnCount)+`, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (sketch_id, node_type, node_key) DO NOTHING`,
		append(append([]any{n.ID, n.SketchID}, cols.args()...), n.Size, n.X, n.Y, n.Version, n.CreatedAt, n.UpdatedAt)...)
	if err != nil {
		return nil, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}

	if affected == 0 {
		return nil, errLostRace
	}

	if err := s.writeGrams(ctx, q, n); err != nil {
		return nil, err
	}

	return &n, nil
}

// writeNode stores n over the version it was read at and bumps it. The
// search index follows when the label is no longer oldLabel.
func (s *Store) writeNode(ctx context.Context, q querier, n *graphmodel.Node, oldLabel string, now time.Time) error {
	cols, err := s.sealNode(*n)
	if err != nil {
		return err
	}

	res, err := q.Exec(ctx, `
		UPDATE nodes SET `+nodeSets+`, size = ?, x = ?, y = ?,
		       version = version + 1, updated_at = ?
		WHERE id = ? AND sketch_id = ? AND version = ?`,
		append(cols.args(), n.Size, n.X, n.Y, now, n.ID, n.SketchID, n.Version)...)
	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return errLostRace
	}

	if n.Label != oldLabel {
		if err := s.writeGrams(ctx, q, *n); err != nil {
			return err
		}
	}

	n.Version++
	n.UpdatedAt = now

	return nil
}

// UpdateNode applies patch to one node. Nil when it is not a node of
// the sketch.
func (s *Store) UpdateNode(ctx context.Context, sketchID, id string, patch graphmodel.NodePatch) (*graphmodel.Node, error) {
	now := time.Now().UTC()

	var out *graphmodel.Node
	err := s.Tx(ctx, func(tx *database.Tx) error {
		for range casAttempts {
			cur, err := s.getNode(ctx, tx, sketchID, id)
			if err != nil || cur == nil {
				return err
			}

			next := applyNodePatch(*cur, patch)

			err = s.writeNode(ctx, tx, &next, cur.Label, now)
			if errors.Is(err, errLostRace) {
				continue
			}

			if err != nil {
				return err
			}

			out = &next

			return touch(ctx, tx, sketchID, now)
		}

		return errLostRace
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// SetPositions moves nodes, ignoring ids that are not on the sketch.
func (s *Store) SetPositions(ctx context.Context, sketchID string, positions []graphmodel.Position) (int, error) {
	now := time.Now().UTC()
	count := 0
	err := s.Tx(ctx, func(tx *database.Tx) error {
		for _, p := range positions {
			res, err := tx.Exec(ctx, `UPDATE nodes SET x = ?, y = ? WHERE id = ? AND sketch_id = ?`,
				p.X, p.Y, p.NodeID, sketchID)
			if err != nil {
				return err
			}

			n, err := res.RowsAffected()
			if err != nil {
				return err
			}

			count += int(n)
		}

		return touch(ctx, tx, sketchID, now)
	})

	return count, err
}

// DeleteNodes removes nodes and, by cascade, every edge touching them.
func (s *Store) DeleteNodes(ctx context.Context, sketchID string, ids []string) (int, error) {
	return s.deleteFrom(ctx, "nodes", sketchID, ids)
}

// Clear removes every node of the sketch, the edges go with them.
func (s *Store) Clear(ctx context.Context, sketchID string) (int, error) {
	now := time.Now().UTC()
	count := 0
	err := s.Tx(ctx, func(tx *database.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM edges WHERE sketch_id = ?`, sketchID); err != nil {
			return err
		}

		res, err := tx.Exec(ctx, `DELETE FROM nodes WHERE sketch_id = ?`, sketchID)
		if err != nil {
			return err
		}

		n, err := res.RowsAffected()
		if err != nil {
			return err
		}

		count = int(n)

		return touch(ctx, tx, sketchID, now)
	})

	return count, err
}

// Dependents lists the nodes found through ids: once ids are gone, the
// pieces of the graph left hanging only from them, with nothing in a
// piece older than the node of ids it hangs from. A piece reaching the
// rest of the graph, or holding an older entity, is not a dependent.
// Edge direction is ignored, since transforms draw both ways. Age is
// the id: UUIDv7 sorts by creation, with no two the same, where two
// created_at in one transaction could tie.
func (s *Store) Dependents(ctx context.Context, sketchID string, ids []string) ([]string, error) {
	exists := map[string]bool{}
	if err := s.scanPairs(ctx, `SELECT id, id FROM nodes WHERE sketch_id = ?`, sketchID,
		func(rows *sql.Rows) error {
			var id, again string
			if err := rows.Scan(&id, &again); err != nil {
				return err
			}

			exists[id] = true

			return nil
		}); err != nil {
		return nil, err
	}

	adjacent := map[string][]string{}
	if err := s.scanPairs(ctx, `SELECT source_id, target_id FROM edges WHERE sketch_id = ?`, sketchID,
		func(rows *sql.Rows) error {
			var a, b string
			if err := rows.Scan(&a, &b); err != nil {
				return err
			}

			adjacent[a] = append(adjacent[a], b)
			adjacent[b] = append(adjacent[b], a)

			return nil
		}); err != nil {
		return nil, err
	}

	gone := map[string]bool{}
	for _, id := range ids {
		if exists[id] {
			gone[id] = true
		}
	}

	seen := map[string]bool{}
	var out []string
	for _, root := range ids {
		if !gone[root] {
			continue
		}

		for _, start := range adjacent[root] {
			if gone[start] || seen[start] {
				continue
			}

			// One piece, and the oldest removed node it hangs from.
			piece, hangsFrom := []string{start}, root
			seen[start] = true
			for i := 0; i < len(piece); i++ {
				for _, next := range adjacent[piece[i]] {
					switch {
					case gone[next]:
						hangsFrom = min(hangsFrom, next)
					case !seen[next]:
						seen[next] = true
						piece = append(piece, next)
					}
				}
			}

			older := slices.ContainsFunc(piece, func(id string) bool { return id < hangsFrom })
			if !older {
				out = append(out, piece...)
			}
		}
	}

	slices.Sort(out)

	return out, nil
}

// scanPairs runs a query of one sketch and hands each row to scan.
func (s *Store) scanPairs(ctx context.Context, query, sketchID string, scan func(*sql.Rows) error) error {
	rows, err := s.Query(ctx, query, sketchID)
	if err != nil {
		return err
	}

	defer rows.Close() //nolint:errcheck // read-only

	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}

	return rows.Err()
}

// DeleteEdges removes edges.
func (s *Store) DeleteEdges(ctx context.Context, sketchID string, ids []string) (int, error) {
	return s.deleteFrom(ctx, "edges", sketchID, ids)
}

// deleteFrom removes ids from table, a chunk per statement. table is one
// of two literals, never input.
func (s *Store) deleteFrom(ctx context.Context, table, sketchID string, ids []string) (int, error) {
	now := time.Now().UTC()
	count := 0
	err := s.Tx(ctx, func(tx *database.Tx) error {
		for chunk := range slices.Chunk(ids, inChunk) {
			res, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE sketch_id = ? AND id IN (`+placeholders(len(chunk))+`)`,
				idArgs(sketchID, chunk)...)
			if err != nil {
				return err
			}

			n, err := res.RowsAffected()
			if err != nil {
				return err
			}

			count += int(n)
		}

		return touch(ctx, tx, sketchID, now)
	})

	return count, err
}

// UpsertEdge creates the edge or merges properties into the existing
// one. Nil when an end is not a node of the sketch.
func (s *Store) UpsertEdge(ctx context.Context, sketchID string, e graphmodel.Edge) (*graphmodel.Edge, error) {
	now := time.Now().UTC()

	var out *graphmodel.Edge
	err := s.Tx(ctx, func(tx *database.Tx) error {
		var err error
		out, err = s.upsertEdge(ctx, tx, sketchID, e, now)
		if err != nil || out == nil {
			return err
		}

		return touch(ctx, tx, sketchID, now)
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (s *Store) upsertEdge(ctx context.Context, q querier, sketchID string, in graphmodel.Edge, now time.Time) (*graphmodel.Edge, error) {
	e, _, err := s.upsertEdgeNew(ctx, q, sketchID, in, now)

	return e, err
}

// upsertEdgeNew is upsertEdge also saying whether the relationship is
// new rather than one that was there.
func (s *Store) upsertEdgeNew(ctx context.Context, q querier, sketchID string, in graphmodel.Edge, now time.Time) (*graphmodel.Edge, bool, error) {
	for _, end := range []string{in.SourceID, in.TargetID} {
		n, err := s.getNode(ctx, q, sketchID, end)
		if err != nil || n == nil {
			return nil, false, err
		}
	}

	// A relationship there already is updated.
	id, err := s.edgeIDByEnds(ctx, q, sketchID, in.SourceID, in.TargetID, in.Label)
	if err != nil {
		return nil, false, err
	}

	created := false
	if id == "" {
		// DO UPDATE rather than DO NOTHING, because only an updated row
		// comes back from RETURNING. The SET is a no-op on the key column.
		// The row is new when the id coming back is the one offered. The
		// columns are sealed under the offered id, kept only when that id
		// is the row's.
		fresh := ids.New()
		label, sealed, err := s.sealLabel(fresh, in.Label)
		if err != nil {
			return nil, false, err
		}

		props, err := database.SealJSON(s.codec, edgeAAD(fresh), orEmpty(in.Properties))
		if err != nil {
			return nil, false, err
		}

		err = q.QueryRow(ctx, `
			INSERT INTO edges (id, sketch_id, source_id, target_id, label, label_sealed, properties, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (sketch_id, source_id, target_id, label) DO UPDATE SET label = excluded.label
			RETURNING id`,
			fresh, sketchID, in.SourceID, in.TargetID, label, sealed, props, now, now).
			Scan(&id)
		if err != nil {
			return nil, false, err
		}

		created = id == fresh
	}

	cur, err := s.getEdge(ctx, q, sketchID, id)
	if err != nil || cur == nil || len(in.Properties) == 0 {
		return cur, created, err
	}

	cur, err = s.writeEdge(ctx, q, cur, nil, keepFirstOrigin(cur.Properties, in.Properties), now)

	return cur, created, err
}

// writeEdge stores a label change and merged properties on cur.
func (s *Store) writeEdge(ctx context.Context, q querier, cur *graphmodel.Edge, label *string, props map[string]any, now time.Time) (*graphmodel.Edge, error) {
	next := *cur
	next.Properties = mergeMaps(cur.Properties, props)
	next.UpdatedAt = now
	if label != nil {
		next.Label = *label
	}

	sealed, err := database.SealJSON(s.codec, edgeAAD(next.ID), orEmpty(next.Properties))
	if err != nil {
		return nil, err
	}

	idx, labelSealed, err := s.sealLabel(next.ID, next.Label)
	if err != nil {
		return nil, err
	}

	res, err := q.Exec(ctx, `UPDATE edges SET label = ?, label_sealed = ?, properties = ?, updated_at = ? WHERE id = ? AND sketch_id = ?`,
		idx, labelSealed, sealed, now, next.ID, next.SketchID)
	if err != nil {
		return nil, err
	}

	// Gone between the read and the write is not found, not success.
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return nil, err
	}

	return &next, nil
}

// UpdateEdge applies patch to one edge. Nil when it is not an edge of
// the sketch. A relabel onto an edge that already exists is a unique
// violation.
func (s *Store) UpdateEdge(ctx context.Context, sketchID, id string, patch graphmodel.EdgePatch) (*graphmodel.Edge, error) {
	now := time.Now().UTC()

	var out *graphmodel.Edge
	err := s.Tx(ctx, func(tx *database.Tx) error {
		cur, err := s.getEdge(ctx, tx, sketchID, id)
		if err != nil || cur == nil {
			return err
		}

		out, err = s.writeEdge(ctx, tx, cur, patch.Label, patch.Properties, now)
		if err != nil {
			return err
		}

		return touch(ctx, tx, sketchID, now)
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// MergeNodes folds ids into one node and moves their outside edges onto
// it, keeping each edge's label and properties.
func (s *Store) MergeNodes(ctx context.Context, sketchID string, mergeIDs []string, keepID string, into graphmodel.Node) (*graphmodel.Node, error) {
	now := time.Now().UTC()

	var out *graphmodel.Node
	err := s.Tx(ctx, func(tx *database.Tx) error {
		merged := map[string]bool{}
		for _, id := range mergeIDs {
			n, err := s.getNode(ctx, tx, sketchID, id)
			if err != nil || n == nil {
				return err
			}

			merged[id] = true
		}

		target, err := s.mergeTarget(ctx, tx, sketchID, keepID, merged, into, now)
		if err != nil {
			return err
		}

		// Edges to the outside move onto the target. Edges among the
		// merged nodes are dropped with them.
		var moved []graphmodel.Edge
		for id := range merged {
			if id == target.ID {
				continue
			}

			edges, err := s.collectEdges(tx.Query(ctx, edgeSelect+`
				WHERE sketch_id = ? AND (source_id = ? OR target_id = ?)`, sketchID, id, id))
			if err != nil {
				return err
			}

			for _, e := range edges {
				switch {
				case merged[e.SourceID] && merged[e.TargetID]:
					continue
				case merged[e.SourceID]:
					e.SourceID = target.ID
				default:
					e.TargetID = target.ID
				}

				moved = append(moved, e)
			}
		}

		for id := range merged {
			if id == target.ID {
				continue
			}

			if _, err := tx.Exec(ctx, `DELETE FROM nodes WHERE id = ? AND sketch_id = ?`, id, sketchID); err != nil {
				return err
			}
		}

		for _, e := range moved {
			if _, err := s.upsertEdge(ctx, tx, sketchID, e, now); err != nil {
				return err
			}
		}

		out = target

		return touch(ctx, tx, sketchID, now)
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// mergeTarget is the node a merge lands on: keepID rewritten with into,
// or into upserted. A kept node keeps its type. Properties of the
// merged nodes are folded in, the target's own winning, and into's
// winning over all.
func (s *Store) mergeTarget(ctx context.Context, tx *database.Tx, sketchID, keepID string, merged map[string]bool,
	into graphmodel.Node, now time.Time) (*graphmodel.Node, error) {
	props := map[string]any{}
	for _, id := range slices.Sorted(maps.Keys(merged)) {
		if id == keepID {
			continue
		}

		// Checked when the merge began, but on postgres a concurrent
		// delete may land between the two reads.
		n, err := s.getNode(ctx, tx, sketchID, id)
		if err != nil || n == nil {
			return nil, err
		}

		props = mergeMaps(props, n.Properties)
	}

	if !merged[keepID] {
		into.Properties = mergeMaps(props, into.Properties)

		return s.upsertNode(ctx, tx, sketchID, into, now)
	}

	cur, err := s.getNode(ctx, tx, sketchID, keepID)
	if err != nil || cur == nil {
		return nil, err
	}

	// into was built through the type over the merged fields, so its key
	// and label are the kept node's from here on.
	next := *cur
	next.Key = cmp.Or(into.Key, cur.Key)
	next.Label = cmp.Or(into.Label, cur.Label)
	next.Properties = mergeMaps(mergeMaps(props, cur.Properties), into.Properties)
	if err := s.writeNode(ctx, tx, &next, cur.Label, now); err != nil {
		return nil, err
	}

	return &next, nil
}

// Neighbors returns the node, the nodes one edge away and those edges.
func (s *Store) Neighbors(ctx context.Context, sketchID, id string) ([]graphmodel.Node, []graphmodel.Edge, error) {
	center, err := s.getNode(ctx, s, sketchID, id)
	if err != nil || center == nil {
		return nil, nil, err
	}

	edges, err := s.collectEdges(s.Query(ctx, edgeSelect+`
		WHERE sketch_id = ? AND (source_id = ? OR target_id = ?)
		ORDER BY created_at, id`, sketchID, id, id))
	if err != nil {
		return nil, nil, err
	}

	seen := map[string]bool{center.ID: true}
	var others []string
	for _, e := range edges {
		other := e.TargetID
		if other == id {
			other = e.SourceID
		}

		if !seen[other] {
			seen[other] = true
			others = append(others, other)
		}
	}

	// One read for all the neighbours, a hub has hundreds.
	around, err := s.NodesIn(ctx, sketchID, others)
	if err != nil {
		return nil, nil, err
	}

	return append([]graphmodel.Node{*center}, around...), edges, nil
}
