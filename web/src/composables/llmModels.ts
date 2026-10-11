// The language models a person may pick for the assistant and the
// template generator, and the one picked, remembered in this browser.
// A pick that is gone falls back to the default, as the server does.
import { computed, ref } from 'vue'
import { listModels } from '@/api/catalog'
import type { LlmModel } from '@/api/types'
import { storageGet, storageSet } from '@/composables/storage'

const KEY = 'gosint.llm.model'

const models = ref<LlmModel[]>([])
const picked = ref(storageGet(KEY) ?? '')
const loaded = ref(false)

const current = computed<LlmModel | null>(
  () =>
    models.value.find((m) => m.id === picked.value) ??
    models.value.find((m) => m.default) ??
    models.value[0] ??
    null,
)

async function load() {
  models.value = await listModels()
  loaded.value = true
}

function pick(id: string) {
  picked.value = id
  storageSet(KEY, id)
}

export function useLlmModels() {
  return { models, current, loaded, load, pick }
}
