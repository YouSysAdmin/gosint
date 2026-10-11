// A compact tree for the hierarchy layout. A parent's LEAVES (children
// with no children of their own) are packed in a grid under it, the
// children that carry a subtree stand beside that grid, and the pieces
// of the graph are laid out in rows, so a hub with forty children stays
// a block instead of a strip.
//
// The tree is a spanning tree found breadth first from each piece's
// root, an entity nothing points to, the most connected first. A
// relationship left out of the tree stays drawn as it is.

export interface TreeNode {
  id: string
  degree: number
  // type and label order a grid of leaves, a type's entities together.
  type?: string
  label?: string
}

export interface TreeEdge {
  source: string
  target: string
}

export interface Spot {
  x: number
  y: number
}

// RANK is the step from a parent down to its children, LEAF_W and
// LEAF_H a leaf's cell in the grid, GAP the space between blocks.
const RANK = 120
const LEAF_W = 72
const LEAF_H = 46
const GAP = 48
const MAX_COLS = 6
// A row of pieces wraps past this width.
const ROW_WIDTH = 2400

interface Box {
  w: number
  h: number
  leaves: string[]
  cols: number
  inner: string[]
  childrenWidth: number
}

export function treeLayout(nodes: TreeNode[], edges: TreeEdge[]): Map<string, Spot> {
  const byId = new Map(nodes.map((n) => [n.id, n]))
  const out = new Map<string, string[]>()
  const inc = new Map<string, string[]>()
  for (const n of nodes) {
    out.set(n.id, [])
    inc.set(n.id, [])
  }
  for (const e of edges) {
    if (!out.has(e.source) || !out.has(e.target) || e.source === e.target) continue
    out.get(e.source)!.push(e.target)
    inc.get(e.target)!.push(e.source)
  }

  // Roots first: nothing points to them, then the most connected.
  const order = [...nodes].sort(
    (a, b) =>
      Number(inc.get(a.id)!.length > 0) - Number(inc.get(b.id)!.length > 0) || b.degree - a.degree,
  )

  const kids = new Map<string, string[]>()
  const seen = new Set<string>()
  const roots: string[] = []
  for (const r of order) {
    if (seen.has(r.id)) continue
    roots.push(r.id)
    seen.add(r.id)
    const queue = [r.id]
    while (queue.length) {
      const u = queue.shift()!
      const list: string[] = []
      // Along the arrows first, then against them, transforms draw both ways.
      for (const v of [...out.get(u)!, ...inc.get(u)!]) {
        if (seen.has(v)) continue
        seen.add(v)
        list.push(v)
        queue.push(v)
      }
      kids.set(u, list)
    }
  }

  const boxes = new Map<string, Box>()
  // measure works bottom up over the tree, iteratively so a long chain
  // cannot overflow the stack.
  const post: string[] = []
  for (const r of roots) {
    const stack = [r]
    while (stack.length) {
      const u = stack.pop()!
      post.push(u)
      stack.push(...(kids.get(u) ?? []))
    }
  }
  for (const u of post.reverse()) {
    const children = kids.get(u) ?? []
    const leaves = children
      .filter((c) => !(kids.get(c) ?? []).length)
      .sort((a, b) => {
        const na = byId.get(a)
        const nb = byId.get(b)
        return (
          (na?.type ?? '').localeCompare(nb?.type ?? '') ||
          (na?.label ?? '').localeCompare(nb?.label ?? '', undefined, { numeric: true })
        )
      })
    const inner = children.filter((c) => (kids.get(c) ?? []).length)
    const cols = leaves.length
      ? Math.min(leaves.length, MAX_COLS, Math.max(1, Math.ceil(Math.sqrt(leaves.length))))
      : 0
    const rows = cols ? Math.ceil(leaves.length / cols) : 0
    const blocks: { w: number; h: number }[] = []
    if (cols) blocks.push({ w: cols * LEAF_W, h: rows * LEAF_H })
    for (const c of inner) blocks.push(boxes.get(c)!)
    const childrenWidth = blocks.reduce((s, b) => s + b.w, 0) + GAP * Math.max(0, blocks.length - 1)
    const childrenHeight = blocks.reduce((m, b) => Math.max(m, b.h), 0)
    boxes.set(u, {
      w: Math.max(LEAF_W, childrenWidth),
      h: blocks.length ? RANK + childrenHeight : LEAF_H,
      leaves,
      cols,
      inner,
      childrenWidth,
    })
  }

  const spots = new Map<string, Spot>()
  const place = (root: string, left: number, top: number) => {
    const stack: [string, number, number][] = [[root, left, top]]
    while (stack.length) {
      const [u, l, t] = stack.pop()!
      const b = boxes.get(u)!
      let x = l + (b.w - b.childrenWidth) / 2
      const y = t + RANK
      // The grid of leaves in the middle, under its parent, the subtrees
      // split to its left and right.
      const half = Math.floor(b.inner.length / 2)
      for (const c of b.inner.slice(0, half)) {
        stack.push([c, x, y])
        x += boxes.get(c)!.w + GAP
      }

      let px = l + b.w / 2
      if (b.cols) {
        px = x + (b.cols * LEAF_W) / 2
        b.leaves.forEach((c, i) => {
          spots.set(c, {
            x: x + (i % b.cols) * LEAF_W + LEAF_W / 2,
            y: y + Math.floor(i / b.cols) * LEAF_H,
          })
        })
        x += b.cols * LEAF_W + GAP
      }

      for (const c of b.inner.slice(half)) {
        stack.push([c, x, y])
        x += boxes.get(c)!.w + GAP
      }

      spots.set(u, { x: px, y: t })
    }
  }

  // Pieces left to right, the biggest first, wrapping into rows.
  roots.sort((a, b) => boxes.get(b)!.w * boxes.get(b)!.h - boxes.get(a)!.w * boxes.get(a)!.h)
  let x = 0
  let y = 0
  let rowHeight = 0
  for (const r of roots) {
    const b = boxes.get(r)!
    if (x > 0 && x + b.w > ROW_WIDTH) {
      x = 0
      y += rowHeight + RANK
      rowHeight = 0
    }
    place(r, x, y)
    x += b.w + GAP * 2
    rowHeight = Math.max(rowHeight, b.h)
  }

  return spots
}
