<script setup lang="ts">
// The one way the console renders an email body, ported from mailyard.
//
// The frame stays sandbox="" with no allow-same-origin, so the markup
// inside it, written by whoever sent the message, cannot reach the
// session, run a script or submit a form. Remote images are held back
// until the reader asks: fetching one tells whoever hosts it that the
// message was opened, when and roughly from where, a read receipt
// nobody agreed to. No view writes srcdoc anywhere else, a test holds
// that.
import { computed, ref, watch } from 'vue'

const props = defineProps<{
  html: string
  title?: string

  /** Frame height when not filling the parent. */
  minHeight?: string

  /** Fill the parent instead of sizing to content. */
  fill?: boolean

  /**
   * Embedded images as data URIs, keyed by Content-ID. A cid: URL
   * names a part of the message and resolves nowhere on its own, so
   * the caller fetches those parts and the preview swaps them in.
   */
  inlineImages?: Record<string, string>
}>()

// cid: references swapped for the embedded parts they name. The result
// is a data: URI, which the remote pass leaves alone: those bytes are
// already in the message and loading them tells nobody anything.
function withInlineImages(html: string): string {
  const images = props.inlineImages
  if (!images || Object.keys(images).length === 0) return html

  const lookup = (ref: string): string | undefined => {
    let id = ref.trim().replace(/^cid:/i, '')
    try {
      id = decodeURIComponent(id)
    } catch {
      // A stray percent sign, keep the id as written.
    }

    return images[id]
  }

  return html
    .replace(
      /((?:\s|(?<=<[a-z][^\s/>]*)\/)src\s*=\s*)(?:"(cid:[^"]*)"|'(cid:[^']*)'|(cid:[^\s>"']+))/gi,
      (whole, pre, dq, sq, bare) => {
        const data = lookup(dq ?? sq ?? bare ?? '')

        return data ? `${pre}"${data}"` : whole
      },
    )
    .replace(/url\(\s*(['"]?)\s*(cid:[^)'"]+)\1\s*\)/gi, (whole, _q, ref) => {
      const data = lookup(ref)

      return data ? `url(${data})` : whole
    })
}

// Remote images, held back until the reader asks for them. Every remote
// reference becomes a data- attribute, which no browser fetches, and
// nothing needs undoing: showing images is just not running this pass.
// data: and cid: are left alone, they fetch nothing.
function withoutRemoteImages(html: string): string {
  const remote = (value: string) => !/^\s*(data:|cid:)/i.test(value)

  // An attribute value in any of its three spellings, and the name may
  // follow a slash straight after the tag name (<img/src=...>).
  const attr = (name: string) =>
    new RegExp(
      `(?:\\s|(?<=<[a-z][^\\s/>]*)/)${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)'|([^\\s>"']+))`,
      'gi',
    )

  return html
    .replace(attr('src'), (whole, dq, sq, bare) => {
      const value = dq ?? sq ?? bare ?? ''

      return remote(value) ? ` data-blocked-src=${JSON.stringify(value)}` : whole
    })
    .replace(attr('srcset'), (whole, dq, sq, bare) => {
      const value = dq ?? sq ?? bare ?? ''

      return remote(value) ? ` data-blocked-srcset=${JSON.stringify(value)}` : whole
    })
    .replace(attr('(background|poster)'), (whole, name, dq, sq, bare) => {
      const value = dq ?? sq ?? bare ?? ''

      return remote(value) ? ` data-blocked-${name.toLowerCase()}=${JSON.stringify(value)}` : whole
    })
    .replace(/url\(\s*(['"]?)([^)'"]+)\1\s*\)/gi, (whole, _q, value) =>
      remote(value) ? 'none /* blocked */' : whole,
    )
}

// Off by default, per frame, and not remembered: loading one sender's
// images is not a standing decision about every other sender.
const showImages = ref(false)

// The browser's own block while images are held back: the rewrite above
// decides whether to show the notice, and markup it does not foresee
// (an svg image, a css image-set, an escaped url) is still refused
// here. A policy given first lands in the document's head, and one the
// message declares later can only narrow it further.
const noRemote =
  '<meta http-equiv="Content-Security-Policy" content="default-src \'none\'; img-src data: blob:; ' +
  "media-src data: blob:; style-src 'unsafe-inline'; font-src data:\">"

const embedded = computed(() => withInlineImages(props.html ?? ''))
// After a doctype, before anything else, so the document keeps its mode.
function withPolicy(html: string): string {
  const doctype = /^\s*<!doctype[^>]*>/i.exec(html)

  return doctype ? doctype[0] + noRemote + html.slice(doctype[0].length) : noRemote + html
}

const rendered = computed(() =>
  showImages.value ? embedded.value : withPolicy(withoutRemoteImages(embedded.value)),
)

// A fresh frame per document: changing srcdoc while the frame still
// loads the previous one can leave the old document on screen.
const frameKey = ref(0)
watch(rendered, () => frameKey.value++)

// No notice bar when nothing is held back.
const hasRemote = computed(
  () => showImages.value || withoutRemoteImages(embedded.value) !== embedded.value,
)

const frameStyle = computed(() =>
  props.fill ? { flex: '1', minHeight: '0' } : { minHeight: props.minHeight ?? '400px' },
)
</script>

<template>
  <div :class="fill ? 'html-preview filling' : 'html-preview'">
    <div v-if="hasRemote" class="preview-notice text-sm">
      <template v-if="!showImages">
        <span>Images from external sources are not displayed.</span>
        <button type="button" class="btn btn-sm btn-secondary" @click="showImages = true">
          Load images
        </button>
      </template>
      <template v-else>
        <span>Images from external sources are displayed.</span>
        <button type="button" class="btn btn-sm btn-secondary" @click="showImages = false">
          Hide images
        </button>
      </template>
    </div>

    <iframe
      :key="frameKey"
      class="html-preview-frame"
      :style="frameStyle"
      :srcdoc="rendered"
      sandbox=""
      :title="title ?? 'HTML preview'"
    ></iframe>
  </div>
</template>

<style scoped>
.html-preview {
  display: block;
}

.filling {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.html-preview-frame {
  width: 100%;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-sm);
  /* White, not the theme background: an email body assumes a white
     page and a dark console would render dark text on dark. */
  background: #fff;
}

.preview-notice {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 10px;
  margin-bottom: 6px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-sm);
  background: var(--bg-secondary);
  color: var(--text-secondary);
}
</style>
