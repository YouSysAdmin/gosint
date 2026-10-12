<script setup lang="ts">
// The tools the transforms use: a program's path, so nothing depends
// on the server's PATH, and each tool's own switch to reach private
// addresses, for a lab. The scripts of transform files get a sandbox
// instead, with the paths it lets through. Where a program is found and
// whether the sandbox works are this node's answers, every node of a
// team install needs the same settings.
import { onMounted, ref } from 'vue'
import { getTools, saveTools } from '@/api/admin'
import { apiErrorMessage } from '@/api/client'
import type { ToolInput, ToolSetting } from '@/api/types'
import FormField from '@/components/FormField.vue'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const tools = ref<ToolSetting[]>([])
const busy = ref(false)

// The path lists as typed, one per line, by tool name.
const reads = ref<Record<string, string>>({})
const writes = ref<Record<string, string>>({})

function take(list: ToolSetting[]) {
  tools.value = list
  for (const t of list) {
    if (!t.sandbox) continue
    reads.value[t.name] = t.sandbox.read_paths.join('\n')
    writes.value[t.name] = t.sandbox.write_paths.join('\n')
  }
}

function lines(text: string | undefined): string[] {
  return (text ?? '')
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
}

function input(t: ToolSetting): ToolInput {
  const out: ToolInput = { path: t.path.trim(), allow_private: t.allow_private }
  if (t.sandbox) {
    out.sandbox = t.sandbox.enabled
    out.read_paths = lines(reads.value[t.name])
    out.write_paths = lines(writes.value[t.name])
  }
  return out
}

async function load() {
  try {
    take(await getTools())
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function save() {
  busy.value = true
  try {
    take(await saveTools(Object.fromEntries(tools.value.map((t) => [t.name, input(t)]))))
    toast.success('Tools saved')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <PageHeader title="Tools" />
  <AdminNav />

  <form class="card" @submit.prevent="save">
    <div class="card-body">
      <div v-for="t in tools" :key="t.name" class="tool">
        <div class="head">
          <strong>{{ t.title }}</strong>
          <template v-if="t.binary">
            <span v-if="t.found" class="badge badge-success ml-2">found</span>
            <span v-else class="badge badge-warning ml-2">not found on this node</span>
          </template>
        </div>
        <p class="text-xs text-muted mb-2">{{ t.description }}</p>
        <FormField
          v-if="t.binary"
          label="Path"
          :hint="
            t.found
              ? `Runs ${t.found}`
              : `An absolute path, empty looks ${t.binary} up on the server's PATH`
          "
        >
          <input
            v-model="t.path"
            class="form-input code-font"
            :placeholder="`/usr/bin/${t.binary}`"
            maxlength="1024"
          />
        </FormField>
        <template v-if="t.private_switch">
          <label class="checkbox-label">
            <input v-model="t.allow_private" type="checkbox" />
            May reach private addresses (loopback, private ranges, link-local), for a lab
          </label>
          <p class="text-xs text-muted hint">
            Off, a sketch cannot reach the network the server sits in. A cloud's metadata service
            stays refused either way.
          </p>
        </template>
        <template v-if="t.sandbox">
          <label class="checkbox-label">
            <input v-model="t.sandbox.enabled" type="checkbox" />
            Run scripts in the sandbox
            <span v-if="t.sandbox.supported" class="badge badge-success ml-2">available here</span>
            <span v-else class="badge badge-warning ml-2">not available on this node</span>
          </label>
          <p v-if="t.sandbox.supported" class="text-xs text-muted hint">
            A script reads the system, its own directory and the paths below, writes only a scratch
            directory of its own, and never reaches the server's data, config or environment. Its
            network is not limited.
          </p>
          <p v-else class="text-xs text-muted hint">
            The sandbox needs Linux with Landlock (5.13 or newer). Here scripts run as the server's
            own user and can read whatever it can. Switched on, they refuse to run.
          </p>
          <div v-if="t.sandbox.enabled" class="paths">
            <FormField
              label="May also read"
              hint="Absolute paths, one per line, a wordlist or a database"
            >
              <textarea v-model="reads[t.name]" class="form-input code-font" rows="3"></textarea>
            </FormField>
            <FormField label="May also write" hint="Absolute paths, one per line">
              <textarea v-model="writes[t.name]" class="form-input code-font" rows="2"></textarea>
            </FormField>
          </div>
        </template>
      </div>
      <div class="flex justify-end">
        <button class="btn btn-primary" :disabled="busy">Save</button>
      </div>
    </div>
  </form>
</template>

<style scoped>
.tool {
  padding: 12px 0 16px;
  border-bottom: 1px solid var(--border-primary);
  margin-bottom: 12px;
}

.head {
  display: flex;
  align-items: center;
  margin-bottom: 4px;
}

.hint {
  margin-left: 24px;
}

.paths {
  margin: 8px 0 0 24px;
}
</style>
