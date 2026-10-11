<script setup lang="ts">
// The sketch's entities with coordinates, drawn by MapLibre. Points close
// together are clustered, a click on a cluster zooms into it and a click
// on a point selects every entity under the cursor.
import {
  GeoJSONSource,
  LngLatBounds,
  Map as MapLibre,
  NavigationControl,
  Popup,
  setWorkerUrl,
  type MapLayerMouseEvent,
  type StyleSpecification,
} from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import type { FeatureCollection } from 'geojson'
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { consoleSettings } from '@/api/catalog'
import type { ConsoleSettings, GraphNode } from '@/api/types'
import { useCatalogStore } from '@/stores/catalog'
import { useThemeStore } from '@/stores/theme'

setWorkerUrl(workerUrl)

const props = defineProps<{
  nodes: GraphNode[]
  selected: Set<string>
}>()

const emit = defineEmits<{
  (e: 'select', id: string, additive: boolean): void
  (e: 'clear'): void
}>()

const SOURCE = 'entities'
const catalog = useCatalogStore()
const theme = useThemeStore()
const host = ref<HTMLDivElement | null>(null)
let map: MapLibre | null = null
let settings: ConsoleSettings | null = null
let fitted = false
const hover = new Popup({ closeButton: false, closeOnClick: false, offset: 10 })

function coord(v: unknown): number | null {
  const n = typeof v === 'string' ? Number(v) : v
  return typeof n === 'number' && Number.isFinite(n) ? n : null
}

const placed = computed(() =>
  props.nodes.flatMap((n) => {
    const lat = coord(n.properties.latitude)
    const lon = coord(n.properties.longitude)
    if (lat === null || lon === null || Math.abs(lat) > 90 || Math.abs(lon) > 180) return []
    return [{ node: n, lat, lon }]
  }),
)

defineExpose({ fit, count: computed(() => placed.value.length) })

function data(): FeatureCollection {
  return {
    type: 'FeatureCollection',
    features: placed.value.map((p) => ({
      type: 'Feature',
      geometry: { type: 'Point', coordinates: [p.lon, p.lat] },
      properties: {
        id: p.node.id,
        label: p.node.label,
        type: p.node.type,
        color: catalog.color(p.node.type),
      },
    })),
  }
}

function cssVar(name: string, fallback: string): string {
  return getComputedStyle(document.body).getPropertyValue(name).trim() || fallback
}

// styleFor is the operator's style, or a plain background when none is
// configured, so the points still show without a tile server.
function styleFor(dark: boolean): string | StyleSpecification {
  const url = dark ? settings?.map_style_dark : settings?.map_style_light
  if (url) return url
  return {
    version: 8,
    sources: {},
    layers: [
      {
        id: 'background',
        type: 'background',
        paint: { 'background-color': cssVar('--bg-tertiary', '#f5f5f5') },
      },
    ],
  }
}

// styleFont is a font stack one of the style's own layers draws with.
function styleFont(): string[] | null {
  const style = map?.getStyle()
  if (!style?.glyphs) return null
  for (const layer of style.layers) {
    const font = layer.type === 'symbol' ? layer.layout?.['text-font'] : undefined
    if (Array.isArray(font) && font.every((f) => typeof f === 'string')) return font as string[]
  }

  return null
}

function selectedStroke() {
  return [
    'case',
    ['in', ['get', 'id'], ['literal', [...props.selected]]],
    3,
    1,
  ] as unknown as number
}

// addLayers runs on every style load, a theme switch drops them.
function addLayers() {
  if (!map || map.getSource(SOURCE)) return
  const ring = cssVar('--text-primary', '#0a0a0a')
  const accent = cssVar('--text-secondary', '#404040')

  map.addSource(SOURCE, {
    type: 'geojson',
    data: data(),
    cluster: true,
    clusterRadius: 40,
    clusterMaxZoom: 14,
  })
  map.addLayer({
    id: 'clusters',
    type: 'circle',
    source: SOURCE,
    filter: ['has', 'point_count'],
    paint: {
      'circle-color': accent,
      'circle-opacity': 0.75,
      'circle-radius': ['step', ['get', 'point_count'], 12, 10, 16, 50, 22],
      'circle-stroke-width': 2,
      'circle-stroke-color': cssVar('--bg-primary', '#fff'),
    },
  })
  // A count needs a font the style serves, the plain background has none.
  const font = styleFont()
  if (font)
    map.addLayer({
      id: 'cluster-count',
      type: 'symbol',
      source: SOURCE,
      filter: ['has', 'point_count'],
      layout: {
        'text-field': ['get', 'point_count_abbreviated'],
        'text-font': font,
        'text-size': 11,
      },
      paint: { 'text-color': cssVar('--bg-primary', '#fff') },
    })
  map.addLayer({
    id: 'points',
    type: 'circle',
    source: SOURCE,
    filter: ['!', ['has', 'point_count']],
    paint: {
      'circle-color': ['get', 'color'],
      'circle-radius': 7,
      'circle-stroke-width': selectedStroke(),
      'circle-stroke-color': ring,
    },
  })
}

function refresh() {
  const src = map?.getSource<GeoJSONSource>(SOURCE)
  src?.setData(data())
  if (!fitted && placed.value.length) fit()
}

function fit() {
  if (!map || !placed.value.length) return
  const b = new LngLatBounds()
  for (const p of placed.value) b.extend([p.lon, p.lat])
  map.fitBounds(b, { padding: 60, maxZoom: 12, duration: fitted ? 400 : 0 })
  fitted = true
}

async function onCluster(e: MapLayerMouseEvent) {
  const f = e.features?.[0]
  const src = map?.getSource<GeoJSONSource>(SOURCE)
  if (!f || !src || f.geometry.type !== 'Point') return
  const zoom = await src.getClusterExpansionZoom(f.properties.cluster_id)
  map?.easeTo({ center: f.geometry.coordinates as [number, number], zoom })
}

function onPoint(e: MapLayerMouseEvent) {
  const ids = [...new Set((e.features ?? []).map((f) => String(f.properties.id)))]
  const additive = e.originalEvent.shiftKey || e.originalEvent.metaKey
  ids.forEach((id, i) => emit('select', id, additive || i > 0))
}

function onHover(e: MapLayerMouseEvent) {
  if (!map) return
  map.getCanvas().style.cursor = 'pointer'
  const fs = e.features ?? []
  const text = fs[0]?.properties.point_count
    ? `${fs[0].properties.point_count} entities`
    : fs.map((f) => `${f.properties.type}: ${f.properties.label}`).join('\n')
  hover.setLngLat(e.lngLat).setText(text).addTo(map)
}

function onLeave() {
  if (map) map.getCanvas().style.cursor = ''
  hover.remove()
}

// gone is set on unmount: the map must not be built after the view
// switched away while the settings were on their way.
let gone = false

onMounted(async () => {
  if (!host.value) return
  try {
    settings = await consoleSettings()
  } catch {
    settings = null
  }
  if (gone || !host.value) return

  map = new MapLibre({
    container: host.value,
    style: styleFor(theme.isDark),
    center: [0, 20],
    zoom: 1.3,
  })
  map.addControl(new NavigationControl({ showCompass: false }), 'top-right')
  map.on('style.load', () => {
    addLayers()
    if (!fitted) fit()
  })
  map.on('click', 'clusters', onCluster)
  map.on('click', 'points', onPoint)
  map.on('click', (e) => {
    if (!map?.queryRenderedFeatures(e.point, { layers: ['points', 'clusters'] }).length)
      emit('clear')
  })
  for (const layer of ['points', 'clusters']) {
    map.on('mousemove', layer, onHover)
    map.on('mouseleave', layer, onLeave)
  }
})

watch(placed, refresh)

watch(
  () => props.selected,
  () => {
    if (map?.getLayer('points'))
      map.setPaintProperty('points', 'circle-stroke-width', selectedStroke())
  },
)

watch(
  () => theme.isDark,
  (dark) => map?.setStyle(styleFor(dark), { diff: false }),
)

onBeforeUnmount(() => {
  gone = true
  hover.remove()
  map?.remove()
  map = null
})
</script>

<template>
  <div ref="host" class="map" :class="{ dark: theme.isDark }"></div>
</template>

<style scoped>
.map {
  width: 100%;
  height: 100%;
  background: var(--bg-tertiary);
}

.map :deep(.maplibregl-popup-content) {
  padding: 6px 10px;
  font-size: 12px;
  white-space: pre-line;
  background: var(--bg-popover);
  color: var(--text-primary);
  box-shadow: var(--shadow-md);
}

.map :deep(.maplibregl-ctrl-attrib),
.map :deep(.maplibregl-ctrl-group) {
  background: var(--bg-popover);
  color: var(--text-secondary);
}

.map.dark :deep(.maplibregl-ctrl-icon) {
  filter: invert(1);
}

.map :deep(.maplibregl-ctrl-attrib a) {
  color: var(--text-secondary);
}

.map :deep(.maplibregl-popup-tip) {
  display: none;
}
</style>
