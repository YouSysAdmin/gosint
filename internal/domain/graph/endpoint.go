// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/blob"
	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/filemeta"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/nmapreport"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/attachment"
	"github.com/yousysadmin/gosint/internal/domain/entitytype"
	attachmodel "github.com/yousysadmin/gosint/internal/models/attachment"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// Handler serves the graph of one sketch. Every route is behind
// requireSketch, reads need read and writes need update.
type Handler struct {
	Runtime *env.Runtime
}

func (h Handler) sketchID(c fiber.Ctx) string { return access.Sketch(c).ID }

// errUnknownType is a node type the registry does not have.
var errUnknownType = errors.New("unknown entity type")

// typed normalizes props for the entity type and returns the node they
// make: its key from the primary field, its label from the type (the
// given label fills in when the type computes none), its properties.
func typed(typ, label string, props map[string]any) (graphmodel.Node, error) {
	reg := types.Builtin()
	if _, ok := reg.Get(typ); !ok {
		return graphmodel.Node{}, errUnknownType
	}

	p := maps.Clone(props)
	if p == nil {
		p = map[string]any{}
	}

	if label != "" {
		p["label"] = label
	}

	out, err := reg.Build(typ, p)
	if err != nil {
		return graphmodel.Node{}, err
	}

	return graphmodel.Node{Type: typ, Key: reg.Key(typ, p), Label: out, Properties: p}, nil
}

// TypeLabeler is the graph store's Labeler over the type registry.
func TypeLabeler(typ string, props map[string]any) string {
	label, err := types.Builtin().Build(typ, props)
	if err != nil {
		return ""
	}

	return label
}

func refuseType(c fiber.Ctx, err error) error {
	if errors.Is(err, errUnknownType) {
		return response.BadRequestFields(c, "unknown entity type",
			[]validation.FieldError{{Field: "type", Rule: "type", Message: "type is not a known entity type"}})
	}

	return entitytype.Refuse(c, err)
}

// Graph answers every node and edge of the sketch.
func (h Handler) Graph(c fiber.Ctx) error {
	nodes, edges, err := h.Runtime.Store.Graph.Graph(c, h.sketchID(c))
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, graphResponse{Nodes: nodes, Edges: edges})
}

// AddNode adds a node, or merges into the node of the same type and key.
func (h Handler) AddNode(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[nodeInput](c)
	if !ok {
		return resp
	}

	ent, err := typed(in.Type, in.Label, in.Properties)
	if err != nil {
		return refuseType(c, err)
	}

	// The add may merge into an entity of the same key, so what was
	// there before is the one with that key.
	var before graphmodel.Snapshot
	if cur, err := h.Runtime.Store.Graph.NodeByKey(c, h.sketchID(c), in.Type, ent.Key); err != nil {
		return response.Internal(c, err)
	} else if cur != nil {
		if before, err = h.snapNodes(c, h.sketchID(c), []string{cur.ID}); err != nil {
			return response.Internal(c, err)
		}
	}

	out, err := h.Runtime.Store.Graph.UpsertNodes(c, h.sketchID(c), []graphmodel.Node{{
		Type:       in.Type,
		Key:        ent.Key,
		Label:      ent.Label,
		Properties: ent.Properties,
		Metadata:   graphmodel.WithOrigin(in.Metadata, byHand(c, "hand", "")),
		Color:      in.Color, Icon: in.Icon, Image: in.Image, Flag: in.Flag, Shape: in.Shape, Size: in.Size,
		X: in.X,
		Y: in.Y,
	}})
	if err != nil {
		return response.Internal(c, err)
	}

	after, err := h.snapNodes(c, h.sketchID(c), []string{out[0].ID})
	if err != nil {
		return response.Internal(c, err)
	}

	h.record(c, h.sketchID(c), "add "+out[0].Label, before, after)

	return response.Created(c, nodeResponse{Node: out[0]})
}

// UpdateNode changes one node.
func (h Handler) UpdateNode(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[patchNodeInput](c)
	if !ok {
		return resp
	}

	cur, err := h.Runtime.Store.Graph.Node(c, h.sketchID(c), c.Params("node_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if cur == nil {
		return response.NotFound(c, "node not found")
	}

	// The merged properties are what the type has to accept. A new label
	// renames the entity, so it replaces the main field unless the patch
	// sets that field itself, and a changed main field moves the label.
	merged := mergeMaps(cur.Properties, in.Properties)
	wantLabel := ""
	if in.Label != nil {
		wantLabel = *in.Label
		if t, ok := types.Builtin().Get(cur.Type); ok {
			for _, f := range t.Fields {
				if _, set := in.Properties[f.Name]; f.Primary && !set {
					delete(merged, f.Name)
				}
			}
		}
	}

	ent, err := typed(cur.Type, wantLabel, merged)
	if err != nil {
		return refuseType(c, err)
	}

	before, err := h.snapNodes(c, h.sketchID(c), []string{cur.ID})
	if err != nil {
		return response.Internal(c, err)
	}

	n, err := h.Runtime.Store.Graph.UpdateNode(c, h.sketchID(c), cur.ID, graphmodel.NodePatch{
		Key: &ent.Key, Label: &ent.Label, Properties: ent.Properties, Metadata: in.Metadata,
		Color: in.Color, Icon: in.Icon, Image: in.Image, Flag: in.Flag, Shape: in.Shape, Size: in.Size,
	})
	if err != nil {
		return response.Internal(c, err)
	}

	if n == nil {
		return response.NotFound(c, "node not found")
	}

	after, err := h.snapNodes(c, h.sketchID(c), []string{n.ID})
	if err != nil {
		return response.Internal(c, err)
	}

	h.record(c, h.sketchID(c), "edit "+cur.Label, before, after)

	return response.Success(c, nodeResponse{Node: *n})
}

// SetPositions stores where the console drew nodes.
func (h Handler) SetPositions(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[positionsInput](c)
	if !ok {
		return resp
	}

	positions := make([]graphmodel.Position, 0, len(in.Positions))
	for _, p := range in.Positions {
		positions = append(positions, graphmodel.Position{NodeID: p.NodeID, X: p.X, Y: p.Y})
	}

	n, err := h.Runtime.Store.Graph.SetPositions(c, h.sketchID(c), positions)
	if err != nil {
		return response.Internal(c, err)
	}

	if n > 0 {
		h.changed(c, h.sketchID(c), "moved")
	}

	return response.Success(c, countResponse{Count: n})
}

// DeleteNodes removes nodes and their edges, and with_dependents what
// was found through them.
func (h Handler) DeleteNodes(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[deleteNodesInput](c)
	if !ok {
		return resp
	}

	ids := in.IDs
	if in.WithDependents {
		more, err := h.Runtime.Store.Graph.Dependents(c, h.sketchID(c), in.IDs)
		if err != nil {
			return response.Internal(c, err)
		}

		ids = slices.Concat(in.IDs, more)
	}

	// The file records go with the nodes, their bytes after.
	hashes, err := h.Runtime.Store.Attachment.HashesOfNodes(c, h.sketchID(c), ids)
	if err != nil {
		return response.Internal(c, err)
	}

	before, err := h.snapNodes(c, h.sketchID(c), ids)
	if err != nil {
		return response.Internal(c, err)
	}

	n, err := h.Runtime.Store.Graph.DeleteNodes(c, h.sketchID(c), ids)
	if err != nil {
		return response.Internal(c, err)
	}

	attachment.Release(c, h.Runtime, hashes)
	if n > 0 {
		h.record(c, h.sketchID(c), "delete "+counted(n, "entity", "entities"), before, graphmodel.Snapshot{})
	}

	return response.Success(c, countResponse{Count: n})
}

// Dependents lists the nodes a delete with_dependents would also take.
func (h Handler) Dependents(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[idsInput](c)
	if !ok {
		return resp
	}

	ids, err := h.Runtime.Store.Graph.Dependents(c, h.sketchID(c), in.IDs)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, idsResponse{IDs: ids})
}

// Clear removes every node and edge of the sketch.
func (h Handler) Clear(c fiber.Ctx) error {
	hashes, err := h.Runtime.Store.Attachment.HashesOfSketch(c, h.sketchID(c))
	if err != nil {
		return response.Internal(c, err)
	}

	before, err := h.snapGraph(c, h.sketchID(c))
	if err != nil {
		return response.Internal(c, err)
	}

	n, err := h.Runtime.Store.Graph.Clear(c, h.sketchID(c))
	if err != nil {
		return response.Internal(c, err)
	}

	attachment.Release(c, h.Runtime, hashes)
	if n > 0 {
		h.record(c, h.sketchID(c), "clear the sketch", before, graphmodel.Snapshot{})
	}

	return response.Success(c, countResponse{Count: n})
}

// MergeNodes folds several nodes into one.
func (h Handler) MergeNodes(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[mergeInput](c)
	if !ok {
		return resp
	}

	keeping := false
	for _, id := range in.NodeIDs {
		keeping = keeping || id == in.KeepID
	}

	if in.KeepID != "" && !keeping {
		return response.BadRequest(c, "keep_id must be one of node_ids")
	}

	if !keeping && (in.Node.Type == "" || in.Node.Label == "") {
		return response.BadRequest(c, "node type and label are required unless keep_id is set")
	}

	before, err := h.snapNodes(c, h.sketchID(c), in.NodeIDs)
	if err != nil {
		return response.Internal(c, err)
	}

	var into graphmodel.Node
	if keeping {
		into, err = keptNode(before.Nodes, in.KeepID, in.Node)
	} else {
		into, err = typed(in.Node.Type, in.Node.Label, in.Node.Properties)
	}

	if errors.Is(err, errNotInSketch) {
		return response.NotFound(c, "node not found")
	}

	if err != nil {
		return refuseType(c, err)
	}

	n, err := h.Runtime.Store.Graph.MergeNodes(c, h.sketchID(c), in.NodeIDs, in.KeepID, into)
	if database.UniqueViolation(err) {
		return response.Conflict(c, "a node with that type and value already exists")
	}

	if err != nil {
		return response.Internal(c, err)
	}

	if n == nil {
		return response.NotFound(c, "node not found")
	}

	after, err := h.snapNodes(c, h.sketchID(c), []string{n.ID})
	if err != nil {
		return response.Internal(c, err)
	}

	h.record(c, h.sketchID(c), "merge "+counted(len(in.NodeIDs), "entity", "entities"), before, after)

	return response.Success(c, nodeResponse{Node: *n})
}

// errNotInSketch is a kept node that is not a node of the sketch.
var errNotInSketch = errors.New("node not found")

// keptNode is what a merge keeping one node writes: the properties of
// the other nodes, the kept node's over them and the request's on top,
// built through the kept node's type like a hand edit, so its key and
// label follow the merged fields.
func keptNode(nodes []graphmodel.Node, keepID string, in mergeNodeInput) (graphmodel.Node, error) {
	byID := map[string]graphmodel.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}

	cur, ok := byID[keepID]
	if !ok {
		return graphmodel.Node{}, errNotInSketch
	}

	merged := map[string]any{}
	for _, id := range slices.Sorted(maps.Keys(byID)) {
		if id != keepID {
			merged = mergeMaps(merged, byID[id].Properties)
		}
	}

	merged = mergeMaps(mergeMaps(merged, cur.Properties), in.Properties)

	// A new label renames the kept entity, as in UpdateNode.
	if in.Label != "" {
		if t, ok := types.Builtin().Get(cur.Type); ok {
			for _, f := range t.Fields {
				if _, set := in.Properties[f.Name]; f.Primary && !set {
					delete(merged, f.Name)
				}
			}
		}
	}

	return typed(cur.Type, in.Label, merged)
}

// Neighbors answers a node and everything one edge away.
func (h Handler) Neighbors(c fiber.Ctx) error {
	nodes, edges, err := h.Runtime.Store.Graph.Neighbors(c, h.sketchID(c), c.Params("node_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if nodes == nil {
		return response.NotFound(c, "node not found")
	}

	return response.Success(c, graphResponse{Nodes: nodes, Edges: edges})
}

// AddEdge adds a relationship between two nodes of the sketch.
func (h Handler) AddEdge(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[edgeInput](c)
	if !ok {
		return resp
	}

	var before graphmodel.Snapshot
	if cur, err := h.Runtime.Store.Graph.EdgeByEnds(c, h.sketchID(c), in.Source, in.Target, in.Label); err != nil {
		return response.Internal(c, err)
	} else if cur != nil {
		before.Edges = []graphmodel.Edge{*cur}
	}

	e, err := h.Runtime.Store.Graph.UpsertEdge(c, h.sketchID(c), graphmodel.Edge{
		SourceID: in.Source, TargetID: in.Target, Label: in.Label,
		Properties: graphmodel.WithOrigin(in.Properties, byHand(c, "hand", "")),
	})
	if err != nil {
		return response.Internal(c, err)
	}

	if e == nil {
		return response.NotFound(c, "source or target node not found")
	}

	h.record(c, h.sketchID(c), "link "+e.Label, before, graphmodel.Snapshot{Edges: []graphmodel.Edge{*e}})

	return response.Created(c, edgeResponse{Edge: *e})
}

// UpdateEdge changes one relationship.
func (h Handler) UpdateEdge(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[patchEdgeInput](c)
	if !ok {
		return resp
	}

	before, err := h.snapEdges(c, h.sketchID(c), []string{c.Params("edge_id")})
	if err != nil {
		return response.Internal(c, err)
	}

	e, err := h.Runtime.Store.Graph.UpdateEdge(c, h.sketchID(c), c.Params("edge_id"), graphmodel.EdgePatch{
		Label: in.Label, Properties: in.Properties,
	})
	if database.UniqueViolation(err) {
		return response.Conflict(c, "these nodes already have a relationship with that label")
	}

	if err != nil {
		return response.Internal(c, err)
	}

	if e == nil {
		return response.NotFound(c, "edge not found")
	}

	h.record(c, h.sketchID(c), "edit a relationship", before, graphmodel.Snapshot{Edges: []graphmodel.Edge{*e}})

	return response.Success(c, edgeResponse{Edge: *e})
}

// DeleteEdges removes relationships.
func (h Handler) DeleteEdges(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[idsInput](c)
	if !ok {
		return resp
	}

	before, err := h.snapEdges(c, h.sketchID(c), in.IDs)
	if err != nil {
		return response.Internal(c, err)
	}

	n, err := h.Runtime.Store.Graph.DeleteEdges(c, h.sketchID(c), in.IDs)
	if err != nil {
		return response.Internal(c, err)
	}

	if n > 0 {
		h.record(c, h.sketchID(c), "delete "+counted(n, "relationship", "relationships"), before, graphmodel.Snapshot{})
	}

	return response.Success(c, countResponse{Count: n})
}

// byHand is the caller as the origin of what they add: via is hand,
// import or chat, source an import's kind.
func byHand(c fiber.Ctx, via, source string) graphmodel.Origin {
	u := access.User(c)

	return graphmodel.Origin{User: u.ID, Name: u.DisplayName(), Via: via, Source: source, At: time.Now().UTC()}
}

// maxImportErrors bounds the refusals an import reports.
const maxImportErrors = 100

// Import adds entities and relationships in bulk, as an export writes
// them. Each entity goes through its type, one its type refuses is
// skipped and reported, and so are the edges touching it.
func (h Handler) Import(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[importInput](c)
	if !ok {
		return resp
	}

	return h.runImport(c, in)
}

// maxNmapReport bounds a report brought in as a file.
const maxNmapReport = 8 << 20

// ImportNmap adds the hosts of an nmap XML report (nmap -oX), sent as
// the request body: each address as an ip, its open ports as ports
// with service and banner, its names as domains. A report made
// elsewhere lands in the sketch without the server scanning anything.
func (h Handler) ImportNmap(c fiber.Ctx) error {
	body := c.Body()
	if len(body) > maxNmapReport {
		return response.BadRequest(c, "the report is over 8 MB")
	}

	report, err := nmapreport.Parse(body)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	in := importInput{Source: "nmap"}
	for _, host := range report.Hosts {
		in.Nodes = append(in.Nodes, importNode{ID: host.Addr, Type: "ip", Properties: map[string]any{"address": host.Addr}})
		for _, p := range host.Open() {
			ref := fmt.Sprintf("%s:%d/%s", host.Addr, p.Number, p.Protocol)
			props := map[string]any{"number": p.Number, "protocol": p.Protocol, "state": p.State}
			if p.Service != "" {
				props["service"] = p.Service
			}

			if banner := p.Banner(); banner != "" {
				props["banner"] = banner
			}

			in.Nodes = append(in.Nodes, importNode{ID: ref, Type: "port", Properties: props})
			in.Edges = append(in.Edges, importEdge{Source: host.Addr, Target: ref, Label: "HAS_PORT"})
		}

		for _, n := range host.Hostnames {
			in.Nodes = append(in.Nodes, importNode{ID: "name:" + n.Name, Type: "domain", Properties: map[string]any{"domain": n.Name}})
			if n.Kind == "PTR" {
				in.Edges = append(in.Edges, importEdge{Source: host.Addr, Target: "name:" + n.Name, Label: "REVERSE_RESOLVES_TO"})
			} else {
				in.Edges = append(in.Edges, importEdge{Source: "name:" + n.Name, Target: host.Addr, Label: "RESOLVES_TO"})
			}
		}
	}

	if len(in.Nodes) == 0 {
		return response.BadRequest(c, "the report names no host with an IP address")
	}

	return h.runImport(c, in)
}

// importFlags are the flag colours an import may set.
var importFlags = []string{"red", "orange", "yellow", "green", "blue"}

// knownKeys is the (type, key) of every entity in the sketch, read only
// when the import carries a flag, which is what the answer decides.
func (h Handler) knownKeys(c fiber.Ctx, sketchID string, in importInput) (map[string]bool, error) {
	if !slices.ContainsFunc(in.Nodes, func(n importNode) bool { return n.Flag != "" }) {
		return nil, nil
	}

	nodes, _, err := h.Runtime.Store.Graph.Graph(c, sketchID)
	if err != nil {
		return nil, err
	}

	known := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		known[n.Type+"|"+n.Key] = true
	}

	return known, nil
}

// runImport writes an import into this sketch and answers with what it
// did.
func (h Handler) runImport(c fiber.Ctx, in importInput) error {
	res, err := h.doImport(c, h.sketchID(c), in)
	if err != nil {
		return response.Internal(c, err)
	}

	if res.out.NodesCreated > 0 || res.out.EdgesCreated > 0 {
		h.changed(c, h.sketchID(c), "import")
	}

	return response.Success(c, res.out)
}

// imported is what an import did: the answer, the entities written in
// the order of the input's nodes that went through, and the written id
// of each node and file by the id the input gave it.
type imported struct {
	out     importResponse
	written []graphmodel.Node
	ids     map[string]string
}

// doImport writes an import into the sketch.
func (h Handler) doImport(c fiber.Ctx, sketchID string, in importInput) (imported, error) {
	res := imported{ids: map[string]string{}}
	out := &res.out
	out.Errors = append([]string{}, in.Notes...)
	report := func(format string, args ...any) {
		if len(out.Errors) < maxImportErrors {
			out.Errors = append(out.Errors, fmt.Sprintf(format, args...))
		}
	}

	var (
		batch []graphmodel.Node
		refs  []string
	)

	// A flag an import carries colours a NEW entity: one that is there
	// already keeps the flag the investigator left on it, a re-import
	// must not undo a triage.
	known, err := h.knownKeys(c, sketchID, in)
	if err != nil {
		return res, err
	}

	origin := byHand(c, "import", cmp.Or(in.Source, "import"))
	for i, n := range in.Nodes {
		typ := strings.ToLower(n.Type)
		ent, err := typed(typ, n.Label, n.Properties)
		if err != nil {
			out.NodesSkipped++
			report("node %d (%s %q): %v", i+1, typ, n.Label, err)

			continue
		}

		ent.X, ent.Y = n.X, n.Y
		ent.Metadata = graphmodel.WithOrigin(n.Metadata, origin)
		ent.Color, ent.Icon, ent.Image, ent.Shape, ent.Size = n.Color, n.Icon, n.Image, n.Shape, n.Size
		if slices.Contains(importFlags, n.Flag) && !known[typ+"|"+ent.Key] {
			ent.Flag = n.Flag
		}

		batch = append(batch, ent)
		refs = append(refs, n.ID)
	}

	// Edges by index into the batch, so the store writes everything in
	// one transaction. One touching a skipped node is skipped too.
	byRef := map[string]int{}
	for i, ref := range refs {
		if ref != "" {
			byRef[ref] = i
		}
	}

	edges := make([]graphmodel.EdgeRef, 0, len(in.Edges))
	for _, e := range in.Edges {
		src, okS := byRef[e.Source]
		dst, okT := byRef[e.Target]
		if !okS || !okT {
			out.EdgesSkipped++

			continue
		}

		edges = append(edges, graphmodel.EdgeRef{Source: src, Target: dst, Label: e.Label, Properties: e.Properties})
	}

	written, created, err := h.Runtime.Store.Graph.Import(c, sketchID, batch, edges)
	if err != nil {
		return res, err
	}

	res.written = written
	for ref, i := range byRef {
		res.ids[ref] = written[i].ID
	}

	out.NodesCreated = len(written)
	out.EdgesCreated = created
	out.EdgesSkipped += len(edges) - created

	files := map[string]string{}
	for i, f := range in.Files {
		idx, ok := byRef[f.Node]
		if !ok {
			out.FilesSkipped++

			continue
		}

		id, err := h.importFile(c, sketchID, written[idx].ID, f)
		if err != nil {
			out.FilesSkipped++
			report("file %d (%s): %v", i+1, f.Name, err)

			continue
		}

		out.FilesCreated++
		if f.ID != "" {
			files[f.ID] = id
			res.ids[f.ID] = id
		}
	}

	h.relinkImages(c, sketchID, written, files)

	return res, nil
}

// importFile stores one file of an import on the node it names and
// answers the record's id.
func (h Handler) importFile(c fiber.Ctx, sketchID, nodeID string, f importFile) (string, error) {
	data, err := base64.StdEncoding.DecodeString(f.Data)
	if err != nil {
		return "", errors.New("data is not base64")
	}

	if err := blob.ValidName(f.Name); err != nil {
		return "", errors.New("the name is not allowed")
	}

	hash, size, err := h.Runtime.Blobs.Put(c, bytes.NewReader(data), h.Runtime.FileLimits.Get().MaxSize)
	if err != nil {
		return "", err
	}

	// The document's content_type is not trusted: a type the serve
	// path would render inline must come from the name and the bytes.
	ct := attachment.Sniff(f.Name, data[:min(len(data), 512)])

	a := &attachmodel.Attachment{ID: ids.New(), SketchID: sketchID, NodeID: nodeID, Name: f.Name, ContentType: ct,
		Size: size, Hash: hash, UploaderID: access.User(c).ID, CreatedAt: time.Now().UTC()}
	a.Metadata = filemeta.Extract(f.Name, ct, data)
	if err := h.Runtime.Store.Attachment.Create(c, a); err != nil {
		return "", err
	}

	if err := attachment.KeepBytes(c, h.Runtime, hash, data); err != nil {
		return "", err
	}

	return a.ID, nil
}
