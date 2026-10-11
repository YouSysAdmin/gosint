// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/store"
)

// Searching entities across every sketch the caller may read: by a
// piece of the label (the palette), or by type and key exactly (the
// panel's "also in", where else this very entity appears). Labels are
// sealed, so a piece is found through node_grams and checked in Go.

// maxHits bounds a search.
const maxHits = 50

// mentionHits bounds the analysis editor's mention list.
const mentionHits = 20

// candidateFactor is how many rows per hit a search reads before the
// check in Go, which drops the pieces matching only by their hashes.
const candidateFactor = 8

// maxQueryGrams bounds the pieces of a query sent to SQL, the check in
// Go sees the whole query.
const maxQueryGrams = 24

// sketchChunk is how many sketches one query covers: the pieces are
// hashed per sketch, so each sketch adds its own to the query.
const sketchChunk = 50

// maxScan bounds the rows a search reads and opens, whatever it asks.
const maxScan = 2000

// Search finds entities in the sketches of the investigations userID
// belongs to.
func (s *Store) Search(ctx context.Context, userID string, q store.SearchQuery) ([]store.Hit, error) {
	needle := strings.ToLower(q.Q)
	limit := q.Limit
	if limit <= 0 || limit > maxHits {
		limit = maxHits
	}

	sketches, err := s.visibleSketches(ctx, userID, q.Investigation, q.ExcludeSketch)
	if err != nil || len(sketches) == 0 {
		return []store.Hit{}, err
	}

	// Under two characters there is no piece to look up, the labels of
	// the scope are read, up to maxScan, and checked in Go.
	readAll := utf8.RuneCountInString(needle) < 2
	size := 3
	if utf8.RuneCountInString(needle) == 2 {
		size = 2
	}

	budget := maxScan
	if !readAll || q.Type != "" {
		budget = min(maxScan, limit*candidateFactor)
	}

	out := []store.Hit{}
	for chunk := range slices.Chunk(sketches, sketchChunk) {
		if budget <= 0 {
			break
		}

		where, args := []string{"n.sketch_id IN (" + placeholders(len(chunk)) + ")"}, toAny(chunk)
		if !readAll {
			grams, count, err := s.queryGrams(needle, size, chunk)
			if err != nil {
				return nil, err
			}

			where = append(where, `n.id IN (SELECT node_id FROM node_grams WHERE sketch_id IN (`+placeholders(len(chunk))+`)
				AND gram IN (`+placeholders(len(grams))+`) GROUP BY node_id HAVING COUNT(*) = ?)`)
			args = append(append(append(args, toAny(chunk)...), toAny(grams)...), count)
		}

		if q.Type != "" {
			idx, err1 := s.keyIndex(q.Type, q.Key)
			tidx, err2 := s.typeIndex(q.Type)
			if err := errors.Join(err1, err2); err != nil {
				return nil, err
			}

			where = append(where, "n.node_type = ?", "n.node_key = ?")
			args = append(args, tidx, idx)
		}

		hits, read, err := s.hits(ctx, where, args, budget, needle)
		if err != nil {
			return nil, err
		}

		budget -= read
		out = append(out, hits...)
	}

	slices.SortFunc(out, func(a, b store.Hit) int {
		return cmp.Or(cmp.Compare(a.Label, b.Label), cmp.Compare(a.SketchTitle, b.SketchTitle), cmp.Compare(a.ID, b.ID))
	})

	return out[:min(len(out), limit)], nil
}

// visibleSketches is the sketches of the investigations userID belongs
// to, narrowed to one investigation and without one sketch when asked.
func (s *Store) visibleSketches(ctx context.Context, userID, investigationID, exclude string) ([]string, error) {
	query := `SELECT s.id FROM sketches s JOIN investigation_members m ON m.investigation_id = s.investigation_id
		WHERE m.user_id = ?`
	args := []any{userID}
	if investigationID != "" {
		query += ` AND s.investigation_id = ?`
		args = append(args, investigationID)
	}

	if exclude != "" {
		query += ` AND s.id <> ?`
		args = append(args, exclude)
	}

	rows, err := s.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}

		out = append(out, id)
	}

	return out, rows.Err()
}

// queryGrams is the pieces of needle hashed under each sketch of chunk,
// and how many pieces one node must match.
func (s *Store) queryGrams(needle string, size int, chunk []string) ([]string, int, error) {
	var all []string
	count := 0
	for _, sketchID := range chunk {
		grams, err := s.grams(needle, sketchID, size)
		if err != nil {
			return nil, 0, err
		}

		if len(grams) > maxQueryGrams {
			grams = grams[:maxQueryGrams]
		}

		count = len(grams)
		all = append(all, grams...)
	}

	return all, count, nil
}

// hits reads up to budget rows matching where, opens them and keeps the
// ones holding needle. It answers how many rows it read.
func (s *Store) hits(ctx context.Context, where []string, args []any, budget int, needle string) ([]store.Hit, int, error) {
	rows, err := s.Query(ctx, `
		SELECT n.id, n.type_sealed, n.node_label, s.id, s.title, i.id, i.name
		FROM nodes n
		JOIN sketches s ON s.id = n.sketch_id
		JOIN investigations i ON i.id = s.investigation_id
		WHERE `+strings.Join(where, " AND ")+` LIMIT ?`, append(args, budget)...)
	if err != nil {
		return nil, 0, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var (
		out  []store.Hit
		read int
	)

	for rows.Next() {
		read++

		var (
			h          store.Hit
			typeSealed string
		)

		if err := rows.Scan(&h.ID, &typeSealed, &h.Label, &h.SketchID, &h.SketchTitle, &h.InvestigationID, &h.InvestigationName); err != nil {
			return nil, 0, err
		}

		if err := s.openHit(&h, typeSealed); err != nil {
			return nil, 0, err
		}

		if needle == "" || strings.Contains(strings.ToLower(h.Label), needle) {
			out = append(out, h)
		}
	}

	return out, read, rows.Err()
}

// openHit opens the sealed names of a hit.
func (s *Store) openHit(h *store.Hit, typeSealed string) error {
	typ, err := s.codec.OpenText(nodeAAD("type_sealed", h.ID), typeSealed)
	errs := []error{err}
	h.Type = typ

	open := func(aad string, v *string) {
		plain, err := s.codec.OpenText(aad, *v)
		errs = append(errs, err)
		*v = plain
	}

	open(nodeAAD("node_label", h.ID), &h.Label)
	open(database.AAD("sketches", "title", h.SketchID), &h.SketchTitle)
	open(database.AAD("investigations", "name", h.InvestigationID), &h.InvestigationName)

	return errors.Join(errs...)
}

// searchResponse answers a search.
type searchResponse struct {
	Hits []store.Hit `json:"hits"`
}

// Search answers GET /search: ?q= for a piece of the label, or ?type=
// and ?key= for the entity itself, ?exclude= a sketch to leave out.
func (h Handler) Search(c fiber.Ctx) error {
	q := store.SearchQuery{
		Q:             strings.TrimSpace(c.Query("q")),
		Type:          strings.ToLower(strings.TrimSpace(c.Query("type"))),
		Key:           strings.TrimSpace(c.Query("key")),
		ExcludeSketch: c.Query("exclude"),
	}
	if len(q.Q) > 200 || len(q.Key) > 2000 {
		return response.BadRequest(c, "the query is too long")
	}

	if q.Q == "" && (q.Type == "" || q.Key == "") {
		return response.BadRequest(c, "give q, or type and key")
	}

	hits, err := h.Runtime.Store.Graph.Search(c, access.User(c).ID, q)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, searchResponse{Hits: hits})
}

// Entities answers GET /investigations/:id/entities?q=: the entities of
// this investigation's sketches whose label holds q, for the analysis
// editor's mentions. An empty q lists the first ones by label.
func (h Handler) Entities(c fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) > 200 {
		return response.BadRequest(c, "the query is too long")
	}

	hits, err := h.Runtime.Store.Graph.Search(c, access.User(c).ID, store.SearchQuery{
		Q:             q,
		Investigation: access.Investigation(c).Investigation.ID,
		Limit:         mentionHits,
	})
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, searchResponse{Hits: hits})
}
