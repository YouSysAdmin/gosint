<script setup lang="ts">
// The tools the transforms use: a program's path, so nothing depends
// on the server's PATH, and each tool's own switch to reach private
// addresses, for a lab. Where the program is found is this node's
// answer, every node of a team install needs the same paths.
import { onMounted, ref } from 'vue'
import { getTools, saveTools } from '@/api/admin'
import { apiErrorMessage } from '@/api/client'
import type { ToolSetting } from '@/api/types'
import FormField from '@/components/FormField.vue'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const tools = ref<ToolSetting[]>([])
const busy = ref(false)

async function load() {
  try {
    tools.value = await getTools()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function save() {
  busy.value = true
  try {
    tools.value = await saveTools(
      Object.fromEntries(
        tools.value.map((t) => [t.name, { path: t.path.trim(), allow_private: t.allow_private }]),
      ),
    )
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
        <label class="checkbox-label">
          <input v-model="t.allow_private" type="checkbox" />
          May reach private addresses (loopback, private ranges, link-local), for a lab
        </label>
        <p class="text-xs text-muted hint">
          Off, a sketch cannot reach the network the server sits in. A cloud's metadata service
          stays refused either way.
        </p>
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
</style>
