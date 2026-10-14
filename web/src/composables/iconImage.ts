// Entity type icons as images a canvas can draw: the Lucide icon's
// shapes written out as an SVG data URL, loaded once per icon and colour.
// The ink is white on every fill: no type colour of the palette is light
// enough to lose it, and a dark ink picked by luminance made the icons
// on the yellow and the pale types read as black at a glance.
import { Circle } from 'lucide-vue-next'
import type { VNode } from 'vue'
import { iconFor } from '@/composables/entityIcons'

type IconNode = [string, Record<string, string>][]

const images = new Map<string, HTMLImageElement | null>()

// svgOf reads the shapes out of the icon component, whose vnode carries
// them as the iconNode prop.
function svgOf(name: string, color: string): string {
  const icon = iconFor(name)
  if (icon === Circle) return ''
  const render = icon as unknown as (p: object, ctx: object) => VNode
  const shapes = (render({}, { slots: {}, attrs: {} }).props?.iconNode ?? []) as IconNode
  if (!shapes.length) return ''
  const body = shapes
    .map(([tag, attrs]) => {
      const a = Object.entries(attrs)
        .filter(([k]) => k !== 'key')
        .map(([k, v]) => `${k}="${String(v).replace(/"/g, '&quot;')}"`)
        .join(' ')
      return `<${tag} ${a}/>`
    })
    .join('')

  return (
    `<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" ` +
    `fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" ` +
    `stroke-linejoin="round">${body}</svg>`
  )
}

// iconImage is the icon ready to draw, null while it loads or when the
// type has no icon of its own. loaded is called once it can be drawn.
export function iconImage(
  name: string,
  color: string,
  loaded: () => void,
): HTMLImageElement | null {
  const key = name + '|' + color
  const known = images.get(key)
  if (known !== undefined) return known && known.complete ? known : null
  const svg = name ? svgOf(name, color) : ''
  if (!svg) {
    images.set(key, null)
    return null
  }

  const img = new Image()
  img.onload = loaded
  img.onerror = () => images.set(key, null)
  img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg)
  images.set(key, img)

  return null
}

// ICON_INK is the colour every icon is drawn in.
export const ICON_INK = '#ffffff'
