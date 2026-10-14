<script setup lang="ts">
// An attached .eml as a message: a tab bar (HTML, Text, Headers,
// Attachments, Raw) over a body pane. The HTML goes through
// HtmlPreview, the only place a body is rendered. Headers are shown in
// MESSAGE ORDER, since the order of a Received chain is evidence.
import { computed, ref, watch } from 'vue'
import type { MailMessage } from '@/api/types'
import HtmlPreview from '@/components/HtmlPreview.vue'
import { formatDate } from '@/composables/formatDate'
import { humanSize } from '@/composables/humanSize'

type Tab = 'html' | 'text' | 'headers' | 'attachments' | 'raw'

const props = defineProps<{
  mail: MailMessage
  title: string

  /** Where the raw bytes are, for the Raw tab. */
  rawUrl: string
}>()

// Embedded images, fetched here and handed to the preview as data URIs.
// The frame cannot load them itself: it is an opaque origin, so its
// requests carry no session cookie and the file route refuses them.
const maxInlineImage = 5 * 1024 * 1024
const inlineImages = ref<Record<string, string>>({})

function asDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(blob)
  })
}

watch(
  () => props.mail.attachments,
  async (list) => {
    inlineImages.value = {}
    const wanted = (list ?? []).filter(
      (a) =>
        a.content_id &&
        a.content_type.toLowerCase().startsWith('image/') &&
        a.size <= maxInlineImage,
    )
    if (!wanted.length) return

    const found: Record<string, string> = {}
    await Promise.all(
      wanted.map(async (a) => {
        try {
          const res = await fetch(a.url, { credentials: 'same-origin' })
          if (!res.ok) return
          found[a.content_id ?? ''] = await asDataURL(await res.blob())
        } catch {
          // The image stays blank, the rest of the message is unaffected.
        }
      }),
    )

    // Another message may have been opened while these were loading.
    if (list === props.mail.attachments) inlineImages.value = found
  },
  { immediate: true },
)

// Raw bytes, read when the tab is first opened.
const rawMax = 2 << 20
const raw = ref('')
const rawState = ref<'idle' | 'loading' | 'done' | 'failed'>('idle')

async function loadRaw() {
  if (rawState.value !== 'idle') return
  rawState.value = 'loading'
  try {
    const res = await fetch(props.rawUrl, { credentials: 'same-origin' })
    if (!res.ok) throw new Error(`${res.status}`)
    const text = await res.text()
    raw.value = text.length > rawMax ? text.slice(0, rawMax) + '\n... (cut)' : text
    rawState.value = 'done'
  } catch {
    rawState.value = 'failed'
  }
}

// Open on whichever body exists: HTML on a plain-text message shows an
// empty frame and reads as a bug.
const tab = ref<Tab>(props.mail.html ? 'html' : props.mail.text ? 'text' : 'headers')

function select(next: Tab) {
  tab.value = next
  if (next === 'raw') void loadRaw()
}

const envelope = computed(() => {
  const m = props.mail
  const rows: { name: string; value: string }[] = [
    { name: 'From', value: m.from_name ? `${m.from_name} <${m.from}>` : m.from },
    { name: 'To', value: m.to.join(', ') },
  ]
  if (m.cc.length) rows.push({ name: 'Cc', value: m.cc.join(', ') })
  rows.push({ name: 'Date', value: m.date ? formatDate(m.date) : 'no date' })
  rows.push({ name: 'Subject', value: m.subject })
  if (m.message_id) rows.push({ name: 'Message-ID', value: m.message_id })
  return rows
})
</script>

<template>
  <div class="viewer">
    <div class="viewer-top">
      <div class="tabs">
        <button type="button" :class="['tab', { active: tab === 'html' }]" @click="select('html')">
          HTML
        </button>
        <button type="button" :class="['tab', { active: tab === 'text' }]" @click="select('text')">
          Text
        </button>
        <button
          type="button"
          :class="['tab', { active: tab === 'headers' }]"
          @click="select('headers')"
        >
          Headers <span class="count">{{ mail.headers.length }}</span>
        </button>
        <button
          type="button"
          :class="['tab', { active: tab === 'attachments' }]"
          @click="select('attachments')"
        >
          Attachments <span class="count">{{ mail.attachments.length }}</span>
        </button>
        <button type="button" :class="['tab', { active: tab === 'raw' }]" @click="select('raw')">
          Raw
        </button>
      </div>
      <div class="subject text-sm" :title="mail.subject">{{ mail.subject || '(no subject)' }}</div>
    </div>

    <div class="viewer-body">
      <template v-if="tab === 'html'">
        <HtmlPreview
          v-if="mail.html"
          :html="mail.html"
          :title="title"
          :inline-images="inlineImages"
          fill
        />
        <p v-else class="text-sm text-muted">This message has no HTML part.</p>
      </template>

      <template v-else-if="tab === 'text'">
        <pre v-if="mail.text" class="sheet">{{ mail.text }}</pre>
        <p v-else class="text-sm text-muted">This message has no plain text part.</p>
      </template>

      <template v-else-if="tab === 'headers'">
        <div class="table-wrapper">
          <table>
            <tbody>
              <tr v-for="row in envelope" :key="row.name" class="envelope">
                <td class="name">{{ row.name }}</td>
                <td class="value">{{ row.value }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <h4 v-if="mail.received.length" class="text-xs text-muted section">Received chain</h4>
        <ol v-if="mail.received.length" class="received text-xs">
          <li v-for="(hop, i) in mail.received" :key="i">{{ hop }}</li>
        </ol>
        <h4 class="text-xs text-muted section">All headers, in message order</h4>
        <div class="table-wrapper">
          <table>
            <tbody>
              <tr v-for="(h, i) in mail.headers" :key="i">
                <td class="name">
                  <code>{{ h.name }}</code>
                </td>
                <td class="value">{{ h.value }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <template v-else-if="tab === 'attachments'">
        <div v-if="mail.attachments.length" class="table-wrapper">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Type</th>
                <th>Size</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="a in mail.attachments" :key="a.index">
                <td>
                  {{ a.name || '(unnamed)' }}
                  <span v-if="a.content_id" class="text-xs text-muted">inline</span>
                </td>
                <td>
                  <code>{{ a.content_type }}</code>
                </td>
                <td>{{ humanSize(a.size) }}</td>
                <td class="text-right">
                  <a class="btn btn-sm btn-secondary" :href="a.url + '?download=1'">Download</a>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="text-sm text-muted">This message has no attachments.</p>
      </template>

      <template v-else>
        <pre v-if="rawState === 'done'" class="sheet">{{ raw }}</pre>
        <p v-else-if="rawState === 'failed'" class="text-sm text-muted">
          The file could not be read.
        </p>
        <p v-else class="text-sm text-muted">Loading...</p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.viewer {
  display: flex;
  flex-direction: column;
  width: min(100%, 1000px);
  height: calc(100vh - 100px);
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
  color: var(--text-primary);
}

.viewer-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 12px 0;
  border-bottom: 1px solid var(--border-primary);
}

.subject {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-secondary);
}

.count {
  margin-left: 4px;
  color: var(--text-muted);
}

.viewer-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 12px;
  overflow: auto;
}

.sheet {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.section {
  margin: 14px 0 6px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.name {
  width: 1%;
  white-space: nowrap;
  color: var(--text-muted);
  vertical-align: top;
}

.value {
  overflow-wrap: anywhere;
}

.received {
  margin: 0;
  padding-left: 20px;
  overflow-wrap: anywhere;
}

.received li {
  margin-bottom: 4px;
}
</style>
