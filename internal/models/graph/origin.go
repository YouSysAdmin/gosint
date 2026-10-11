// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"maps"
	"time"
)

// OriginKey is the metadata key (a node) or property key (an edge)
// holding where it came from. It is written once, on insert: the store
// keeps the first origin and drops a later one.
const OriginKey = "origin"

// SeenKey is the node metadata key holding the last transform run that
// found the entity again, overwritten on every such write.
const SeenKey = "seen"

// Origin says where a node or an edge came from: the scan and
// transform that found it and the input it was found from, or the
// person who added it and how (by hand, an import, the assistant).
type Origin struct {
	Scan      string
	Transform string
	Step      string

	// From is the label of the input entity, FromID its id.
	From   string
	FromID string

	// User is the account's id, Name how it reads.
	User string
	Name string

	// Via is hand, import, chat, transform or flow. Source names an
	// import's kind (file, nuclei, nmap, text).
	Via    string
	Source string
	At     time.Time
}

// Map is the origin as stored, empty fields left out.
func (o Origin) Map() map[string]any {
	out := map[string]any{}
	for k, v := range map[string]string{
		"scan": o.Scan, "transform": o.Transform, "step": o.Step, "from": o.From, "from_id": o.FromID,
		"user": o.User, "name": o.Name, "via": o.Via, "source": o.Source,
	} {
		if v != "" {
			out[k] = v
		}
	}

	if !o.At.IsZero() {
		out["at"] = o.At.UTC().Format(time.RFC3339)
	}

	return out
}

// WithOrigin is meta with the origin added when it has none, so an
// entity that arrives with its own, from an export say, keeps it.
func WithOrigin(meta map[string]any, o Origin) map[string]any {
	if _, has := meta[OriginKey]; has {
		return meta
	}

	out := make(map[string]any, len(meta)+1)
	maps.Copy(out, meta)

	out[OriginKey] = o.Map()

	return out
}
