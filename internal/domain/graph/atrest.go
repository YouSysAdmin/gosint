// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/yousysadmin/gosint/internal/database"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// Index purposes: the node key and type and the relationship label
// matched by equality, the node label searched by its pieces. Changing
// one orphans every stored index.
const (
	keyPurpose   = "gosint/node-key"
	typePurpose  = graphmodel.TypePurpose
	labelPurpose = "gosint/edge-label"
	gramPurpose  = "gosint/grams"
)

// gramChars is how many hex characters of a gram's keyed hash are kept.
// Short on purpose: two pieces sharing one only cost a check in Go, and
// the stored index tells less.
const gramChars = 16

// maxGramRunes bounds how much of a label is indexed.
const maxGramRunes = 512

func nodeAAD(column, id string) string { return database.AAD("nodes", column, id) }

func edgeAAD(id string) string { return database.AAD("edges", "properties", id) }

// nodeColumns are a node's columns sealNode writes, in nodeCols order:
// the type and the key as keyed hashes with their sealed copies, the
// entity's own columns sealed, the drawing style (colour, icon, flag,
// shape) as it is.
const nodeColumns = `node_type, type_sealed, node_key, key_sealed, node_label, properties, metadata,
       color, icon, image, flag, shape`

const nodeColumnCount = 12

// nodeSets is nodeColumns as an UPDATE's assignments.
const nodeSets = `node_type = ?, type_sealed = ?, node_key = ?, key_sealed = ?, node_label = ?,
       properties = ?, metadata = ?, color = ?, icon = ?, image = ?, flag = ?, shape = ?`

// nodeCols is a node's sealed columns as stored.
type nodeCols struct {
	typ, typeSealed, key, keySealed, label, props, meta string
	color, icon, image, flag, shape                     string
}

func (c *nodeCols) dest() []any {
	return []any{&c.typ, &c.typeSealed, &c.key, &c.keySealed, &c.label, &c.props, &c.meta,
		&c.color, &c.icon, &c.image, &c.flag, &c.shape}
}

func (c nodeCols) args() []any {
	return []any{c.typ, c.typeSealed, c.key, c.keySealed, c.label, c.props, c.meta,
		c.color, c.icon, c.image, c.flag, c.shape}
}

// keyIndex is the stored identity of an entity: a keyed hash of its type
// and key, so the unique constraint holds without the key in clear.
func (s *Store) keyIndex(typ, key string) (string, error) {
	return s.codec.Index(keyPurpose, typ+"\x00"+key)
}

// typeIndex is what node_type holds for typ.
func (s *Store) typeIndex(typ string) (string, error) {
	return s.codec.Index(typePurpose, typ)
}

// sealNode is n's columns sealed under its id.
func (s *Store) sealNode(n graphmodel.Node) (nodeCols, error) {
	var (
		c          nodeCols
		errs       []error
		err1, err2 error
	)

	seal := func(column, plain string) string {
		out, err := s.codec.SealText(nodeAAD(column, n.ID), plain)
		errs = append(errs, err)

		return out
	}

	c.typ, err1 = s.typeIndex(n.Type)
	c.key, err2 = s.keyIndex(n.Type, keyOf(n))
	errs = append(errs, err1, err2)
	c.typeSealed = seal("type_sealed", n.Type)
	c.keySealed = seal("key_sealed", keyOf(n))
	c.label = seal("node_label", n.Label)
	c.props = seal("properties", database.MustJSON(orEmpty(n.Properties)))
	c.meta = seal("metadata", database.MustJSON(orEmpty(n.Metadata)))
	c.image = seal("image", n.Image)
	c.color, c.icon, c.flag, c.shape = n.Color, n.Icon, n.Flag, n.Shape

	return c, errors.Join(errs...)
}

// openNode fills n from its stored columns.
func (s *Store) openNode(n *graphmodel.Node, c nodeCols) error {
	var errs []error
	open := func(column, stored string) string {
		out, err := s.codec.OpenText(nodeAAD(column, n.ID), stored)
		errs = append(errs, err)

		return out
	}

	n.Type = open("type_sealed", c.typeSealed)
	n.Key = open("key_sealed", c.keySealed)

	n.Label = open("node_label", c.label)
	n.Image = open("image", c.image)
	n.Color, n.Icon, n.Flag, n.Shape = c.color, c.icon, c.flag, c.shape
	errs = append(errs,
		database.UnmarshalJSON(open("properties", c.props), &n.Properties),
		database.UnmarshalJSON(open("metadata", c.meta), &n.Metadata))
	return errors.Join(errs...)
}

// labelIndex is what edges.label holds for a relationship label.
func (s *Store) labelIndex(label string) (string, error) {
	return s.codec.Index(labelPurpose, label)
}

// sealLabel is an edge's label columns: the keyed hash and the sealed copy.
func (s *Store) sealLabel(id, label string) (string, string, error) {
	idx, err1 := s.labelIndex(label)
	sealed, err2 := s.codec.SealText(database.AAD("edges", "label_sealed", id), label)

	return idx, sealed, errors.Join(err1, err2)
}

// openLabel is an edge's label.
func (s *Store) openLabel(id, sealed string) (string, error) {
	return s.codec.OpenText(database.AAD("edges", "label_sealed", id), sealed)
}

// edgeIDByEnds is the id of the edge with these ends and label, "" when
// none.
func (s *Store) edgeIDByEnds(ctx context.Context, q querier, sketchID, source, target, label string) (string, error) {
	idx, err := s.labelIndex(label)
	if err != nil {
		return "", err
	}

	var id string
	err = q.QueryRow(ctx, `SELECT id FROM edges WHERE sketch_id = ? AND source_id = ? AND target_id = ? AND label = ?`,
		sketchID, source, target, idx).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}

	return id, err
}

// byKey is the node of the sketch with that type and key, matched by
// their keyed hashes.
func (s *Store) byKey(ctx context.Context, q querier, sketchID, typ, key string) (*graphmodel.Node, error) {
	idx, err1 := s.keyIndex(typ, key)
	tidx, err2 := s.typeIndex(typ)
	if err := errors.Join(err1, err2); err != nil {
		return nil, err
	}

	return s.scanNode(q.QueryRow(ctx, nodeSelect+` WHERE sketch_id = ? AND node_type = ? AND node_key = ?`,
		sketchID, tidx, idx))
}

// grams is the stored index of every two and three character piece of
// text, lowercased, each once, under the key of one sketch: the same
// piece hashes apart in two sketches, so pieces cannot be learned from
// one sketch and read in another.
func (s *Store) grams(text, sketchID string, sizes ...int) ([]string, error) {
	runes := []rune(strings.ToLower(text))
	if len(runes) > maxGramRunes {
		runes = runes[:maxGramRunes]
	}

	if len(sizes) == 0 {
		sizes = []int{2, 3}
	}

	seen := map[string]bool{}

	var out []string
	for _, size := range sizes {
		for i := 0; i+size <= len(runes); i++ {
			g := string(runes[i : i+size])
			if seen[g] {
				continue
			}

			seen[g] = true
			h, err := s.codec.Index(gramPurpose+":"+sketchID, g)
			if err != nil {
				return nil, err
			}

			if len(h) > gramChars {
				h = h[:gramChars]
			}

			out = append(out, h)
		}
	}

	return out, nil
}

// gramRows bounds one insert of grams, under every engine's parameter
// limit.
const gramRows = 300

// writeGrams replaces the search index of n's label.
func (s *Store) writeGrams(ctx context.Context, q querier, n graphmodel.Node) error {
	if _, err := q.Exec(ctx, `DELETE FROM node_grams WHERE node_id = ?`, n.ID); err != nil {
		return err
	}

	grams, err := s.grams(n.Label, n.SketchID)
	if err != nil {
		return err
	}

	for i := 0; i < len(grams); i += gramRows {
		chunk := grams[i:min(i+gramRows, len(grams))]
		args := make([]any, 0, 3*len(chunk))
		for _, g := range chunk {
			args = append(args, n.ID, n.SketchID, g)
		}

		values := strings.TrimSuffix(strings.Repeat("(?, ?, ?), ", len(chunk)), ", ")
		if _, err := q.Exec(ctx, `INSERT INTO node_grams (node_id, sketch_id, gram) VALUES `+values+`
			ON CONFLICT (node_id, gram) DO NOTHING`, args...); err != nil {
			return err
		}
	}

	return nil
}
