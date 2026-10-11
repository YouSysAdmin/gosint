// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"cmp"
	"maps"

	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// mergeMaps returns dst with src laid over it. Nested objects merge key
// by key, anything else in src replaces what dst held, including an
// explicit null. Neither argument is modified.
//
// This is how a transform finding an entity again adds to it rather
// than erasing what an earlier one found.
// keepFirstOrigin is in without its origin when cur has one already:
// where an entity came from is set once, a later find keeps the first.
func keepFirstOrigin(cur, in map[string]any) map[string]any {
	if _, has := cur[graphmodel.OriginKey]; !has || in[graphmodel.OriginKey] == nil {
		return in
	}

	out := make(map[string]any, len(in))
	for k, v := range in {
		if k != graphmodel.OriginKey {
			out[k] = v
		}
	}

	return out
}

func mergeMaps(dst, src map[string]any) map[string]any {
	out := maps.Clone(dst)
	if out == nil {
		out = map[string]any{}
	}

	for k, v := range src {
		sub, isMap := v.(map[string]any)
		cur, curIsMap := out[k].(map[string]any)
		if isMap && curIsMap {
			out[k] = mergeMaps(cur, sub)

			continue
		}

		out[k] = v
	}

	return out
}

// mergeStyle keeps every style field in has unless want sets it.
func mergeStyle(has, want graphmodel.Style) graphmodel.Style {
	return graphmodel.Style{
		Color: cmp.Or(want.Color, has.Color),
		Icon:  cmp.Or(want.Icon, has.Icon),
		Image: cmp.Or(want.Image, has.Image),
		Flag:  cmp.Or(want.Flag, has.Flag),
		Shape: cmp.Or(want.Shape, has.Shape),
		Size:  cmp.Or(want.Size, has.Size),
	}
}

// applyNodePatch returns n with patch applied.
func applyNodePatch(n graphmodel.Node, p graphmodel.NodePatch) graphmodel.Node {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}

	set(&n.Key, p.Key)
	set(&n.Label, p.Label)
	set(&n.Color, p.Color)
	set(&n.Icon, p.Icon)
	set(&n.Image, p.Image)
	set(&n.Flag, p.Flag)
	set(&n.Shape, p.Shape)

	if p.Size != nil {
		n.Size = p.Size
	}

	n.Properties = mergeMaps(n.Properties, p.Properties)
	n.Metadata = mergeMaps(n.Metadata, p.Metadata)

	return n
}

// orEmpty is m, or an empty map so the column holds {} rather than null.
func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}

	return m
}
