<script setup lang="ts">
// The selected entity and its neighbours, drawn as rings around it, so
// its surroundings show without hunting for them on the big canvas. A
// click on a neighbour moves to it.
//
// Each neighbour sits in the DIRECTION it has on the canvas, the closer
// ones on the inner ring, so left on the graph is left here. Dots that
// share a direction are pushed apart along their ring. A neighbour with
// no position yet takes a free spot.
import { computed, ref, watch } from 'vue'

export interface Neighbour {
  id: string
  label: string
  type: string
  color: string
  x?: number | null
  y?: number | null
}

const props = defineProps<{ center: Neighbour; neighbours: Neighbour[] }>()
const emit = defineEmits<{ (e: 'focus', id: string): void }>()

const SIZE = 220
const MID = SIZE / 2
// Ring k holds PER_RING * k dots, so the rings fill evenly outward.
const PER_RING = 14
const MAX_SHOWN = 300

const hovered = ref<Neighbour | null>(null)
watch(
  () => props.center.id,
  () => (hovered.value = null),
)

// A few neighbours make one small ring, a crowd fills the box.
const rings = computed(() => {
  let n = 0
  let k = 0
  while (n < Math.min(props.neighbours.length, MAX_SHOWN)) n += PER_RING * ++k
  return k
})
const width = computed(() => `${Math.min(260, 120 + rings.value * 45)}px`)

const TAU = 2 * Math.PI

// spread keeps sorted angles at least gap apart, moving each pair that
// is too close away from each other, a few passes round the ring.
function spread(angles: number[], gap: number): number[] {
  const a = [...angles]
  const n = a.length
  if (n < 2) return a
  const g = Math.min(gap, TAU / n)
  for (let pass = 0; pass < 40; pass++) {
    let moved = false
    for (let i = 0; i < n; i++) {
      const j = (i + 1) % n
      let d = a[j] - a[i]
      if (j === 0) d += TAU
      if (d < g - 1e-6) {
        const push = (g - d) / 2
        a[i] -= push
        a[j] += push
        moved = true
      }
    }
    if (!moved) break
  }
  return a
}

const placed = computed(() => {
  const c = props.center
  const known = c.x != null && c.y != null
  const list = props.neighbours.slice(0, MAX_SHOWN).map((n, i) => {
    const has = known && n.x != null && n.y != null
    const dx = has ? n.x! - c.x! : 0
    const dy = has ? n.y! - c.y! : 0
    return {
      n,
      has,
      dist: has ? Math.hypot(dx, dy) : Infinity,
      // Without a position, spread evenly by list order.
      angle: has ? Math.atan2(dy, dx) : (i / Math.max(1, props.neighbours.length)) * TAU,
    }
  })
  list.sort((a, b) => a.dist - b.dist)

  const rings: (typeof list)[] = []
  let i = 0
  for (let k = 1; i < list.length; k++) {
    rings.push(list.slice(i, i + PER_RING * k))
    i += PER_RING * k
  }
  const step = (MID - 18) / Math.max(1, rings.length)
  const dot = Math.max(2.5, Math.min(6, step / 3))
  return rings.flatMap((ring, k) => {
    const r = step * (k + 1)
    ring.sort((a, b) => a.angle - b.angle)
    const gap = (2 * dot + 3) / r
    // A ring too full to keep every direction is laid out evenly in the
    // order of the directions, pushing apart would not settle.
    const angles =
      ring.length * gap >= TAU
        ? ring.map((_, j) => ring[0].angle + (j / ring.length) * TAU)
        : spread(
            ring.map((p) => p.angle),
            gap,
          )
    return ring.map((p, j) => ({
      n: p.n,
      x: MID + r * Math.cos(angles[j]),
      y: MID + r * Math.sin(angles[j]),
      dot,
    }))
  })
})
</script>

<template>
  <div class="map">
    <svg
      :style="{ maxWidth: width }"
      :viewBox="`0 0 ${SIZE} ${SIZE}`"
      role="img"
      :aria-label="`${neighbours.length} neighbours`"
    >
      <line
        v-for="p in placed"
        :key="'l' + p.n.id"
        :x1="MID"
        :y1="MID"
        :x2="p.x"
        :y2="p.y"
        class="wire"
        :class="{ lit: hovered?.id === p.n.id }"
      />
      <circle
        v-for="p in placed"
        :key="p.n.id"
        :cx="p.x"
        :cy="p.y"
        :r="hovered?.id === p.n.id ? p.dot + 1.5 : p.dot"
        :fill="p.n.color"
        stroke="transparent"
        stroke-width="10"
        class="dot"
        @mouseenter="hovered = p.n"
        @mouseleave="hovered = null"
        @click="emit('focus', p.n.id)"
      >
        <title>{{ p.n.label }}</title>
      </circle>
      <circle :cx="MID" :cy="MID" r="9" :fill="center.color" class="center" />
    </svg>
    <div class="caption text-xs">
      <template v-if="hovered">{{ hovered.label }}</template>
      <template v-else>
        {{ neighbours.length }} {{ neighbours.length === 1 ? 'neighbour' : 'neighbours' }}
        <span v-if="neighbours.length > MAX_SHOWN" class="text-muted">
          (the first {{ MAX_SHOWN }} drawn)
        </span>
      </template>
    </div>
  </div>
</template>

<style scoped>
.map {
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  padding: 8px;
}

svg {
  display: block;
  width: 100%;
  margin: 0 auto;
}

.wire {
  stroke: var(--text-muted);
  stroke-opacity: 0.25;
  stroke-width: 0.6;
}

.wire.lit {
  stroke: var(--graph-accent);
  stroke-opacity: 1;
  stroke-width: 1.2;
}

.dot {
  cursor: pointer;
  stroke: var(--bg-secondary);
  stroke-width: 1;
}

.center {
  stroke: var(--graph-accent);
  stroke-width: 2;
}

.caption {
  margin-top: 4px;
  min-height: 1.4em;
  text-align: center;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
