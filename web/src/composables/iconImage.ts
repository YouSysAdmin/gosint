// Entity type icons as images a canvas can draw: the Lucide icon's
// shapes written out as an SVG data URL, loaded once per icon and colour.
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

const inks = new Map<string, string>()
let probe: CanvasRenderingContext2D | null = null

// inkOn is the icon colour that reads on a fill: white on a dark one,
// near black on a light one.
export function inkOn(fill: string): string {
  const known = inks.get(fill)
  if (known) return known
  probe ??= document.createElement('canvas').getContext('2d')
  let ink = '#ffffff'
  if (probe) {
    probe.clearRect(0, 0, 1, 1)
    probe.fillStyle = fill
    probe.fillRect(0, 0, 1, 1)
    const [r, g, b] = probe.getImageData(0, 0, 1, 1).data
    const lum = (0.2126 * r! + 0.7152 * g! + 0.0722 * b!) / 255
    if (lum > 0.65) ink = '#1f2328'
  }
  inks.set(fill, ink)

  return ink
}
