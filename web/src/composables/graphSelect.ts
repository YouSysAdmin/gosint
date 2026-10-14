// Selection over the loaded graph: the relatives of a set of entities
// along the relationships, the piece they sit in, and the shortest
// path between two of them. Direction is ignored where it is not the
// point, transforms draw their edges both ways.
import type { GraphEdge } from '@/api/types'

export type Relatives = 'neighbours' | 'parents' | 'children' | 'connected'

// relativesOf is the ids one step from ids: both ends for neighbours,
// the sources of edges into them for parents, the targets of edges out
// of them for children, and everything reachable for connected. The
// starting ids are left out, except for connected.
export function relativesOf(ids: Set<string>, edges: GraphEdge[], how: Relatives): string[] {
  if (how === 'connected') return componentOf(ids, edges)
  const out = new Set<string>()
  for (const e of edges) {
    if (how !== 'children' && ids.has(e.target)) out.add(e.source)
    if (how !== 'parents' && ids.has(e.source)) out.add(e.target)
  }
  for (const id of ids) out.delete(id)
  return [...out]
}

// componentOf is every entity reachable from ids, ids included.
export function componentOf(ids: Set<string>, edges: GraphEdge[]): string[] {
  const adj = adjacency(edges)
  const seen = new Set(ids)
  const queue = [...ids]
  for (let h = 0; h < queue.length; h++) {
    const id = queue[h]
    for (const next of adj.get(id) ?? []) {
      if (!seen.has(next)) {
        seen.add(next)
        queue.push(next)
      }
    }
  }
  return [...seen]
}

// shortestPath is the ids from a to b along the fewest relationships,
// a and b included, or null when nothing joins them.
export function shortestPath(a: string, b: string, edges: GraphEdge[]): string[] | null {
  if (a === b) return [a]
  const adj = adjacency(edges)
  const prev = new Map<string, string>([[a, '']])
  const queue = [a]
  for (let h = 0; h < queue.length; h++) {
    const id = queue[h]
    for (const next of adj.get(id) ?? []) {
      if (prev.has(next)) continue
      prev.set(next, id)
      if (next === b) {
        const path = [b]
        for (let at = id; at; at = prev.get(at)!) path.push(at)
        return path.reverse()
      }
      queue.push(next)
    }
  }
  return null
}

function adjacency(edges: GraphEdge[]): Map<string, string[]> {
  const adj = new Map<string, string[]>()
  const add = (from: string, to: string) => {
    const list = adj.get(from)
    if (list) list.push(to)
    else adj.set(from, [to])
  }
  for (const e of edges) {
    add(e.source, e.target)
    add(e.target, e.source)
  }
  return adj
}
