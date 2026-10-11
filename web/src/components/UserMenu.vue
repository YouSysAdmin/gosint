<script setup lang="ts">
// The account menu at the top right: the theme as three icons, the
// profile and signing out.
import { Monitor, Moon, Sun } from 'lucide-vue-next'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { signOut, useAuthStore } from '@/stores/auth'
import { useThemeStore, type ThemeMode } from '@/stores/theme'

const auth = useAuthStore()
const theme = useThemeStore()
const open = ref(false)
const host = ref<HTMLElement | null>(null)

const modes: { value: ThemeMode; icon: typeof Sun; title: string }[] = [
  { value: 'light', icon: Sun, title: 'Light' },
  { value: 'dark', icon: Moon, title: 'Dark' },
  { value: 'system', icon: Monitor, title: 'Follow the system' },
]

function initials(): string {
  const u = auth.user
  if (!u) return '?'
  const a = (u.first_name?.[0] ?? '') + (u.last_name?.[0] ?? '')
  return (a || u.email.split('@')[0].slice(0, 2)).toUpperCase()
}

function away(ev: MouseEvent) {
  if (host.value && !host.value.contains(ev.target as Node)) open.value = false
}

function key(ev: KeyboardEvent) {
  if (ev.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('mousedown', away, true)
  document.addEventListener('keydown', key)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', away, true)
  document.removeEventListener('keydown', key)
})
</script>

<template>
  <div ref="host" class="user">
    <button class="trigger" :title="auth.user?.email" :aria-expanded="open" @click="open = !open">
      <img v-if="auth.user?.avatar_url" :src="auth.user.avatar_url" class="avatar" alt="" />
      <span v-else class="avatar initials">{{ initials() }}</span>
    </button>
    <div v-if="open" class="menu">
      <div class="who">
        <div class="fw-medium">
          {{
            auth.user?.first_name || auth.user?.last_name
              ? `${auth.user?.first_name ?? ''} ${auth.user?.last_name ?? ''}`.trim()
              : auth.user?.email
          }}
        </div>
        <div v-if="auth.user?.first_name || auth.user?.last_name" class="text-xs text-muted">
          {{ auth.user?.email }}
        </div>
      </div>
      <div class="theme">
        <span class="text-xs text-muted">Theme</span>
        <span class="modes">
          <button
            v-for="m in modes"
            :key="m.value"
            class="mode"
            :class="{ on: theme.mode === m.value }"
            :title="m.title"
            @click="theme.setMode(m.value)"
          >
            <component :is="m.icon" :size="15" />
          </button>
        </span>
      </div>
      <div class="sep"></div>
      <router-link class="item" to="/profile" @click="open = false">Profile</router-link>
      <button class="item" @click="signOut()">Sign out</button>
    </div>
  </div>
</template>

<style scoped>
.user {
  position: relative;
}

.trigger {
  display: flex;
  padding: 2px;
  border: 0;
  border-radius: 50%;
  background: none;
  cursor: pointer;
}

.avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  object-fit: cover;
  border: 1px solid var(--border-primary);
}

.initials {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 600;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.menu {
  position: absolute;
  right: 0;
  top: calc(100% + 6px);
  z-index: 60;
  width: 240px;
  padding: 6px;
  display: flex;
  flex-direction: column;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
}

.who {
  padding: 6px 10px 8px;
  overflow: hidden;
}

.who div {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.theme {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 10px;
}

.modes {
  display: inline-flex;
  border: 1px solid var(--border-button);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.mode {
  display: inline-flex;
  padding: 4px 8px;
  border: 0;
  background: var(--bg-primary);
  color: var(--text-secondary);
  cursor: pointer;
}

.mode + .mode {
  border-left: 1px solid var(--border-button);
}

.mode.on {
  background: var(--bg-active);
  color: var(--text-primary);
}

.sep {
  height: 1px;
  margin: 4px 0;
  background: var(--border-primary);
}

.item {
  padding: 7px 10px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-primary);
  text-align: left;
  text-decoration: none;
  font-size: 13px;
  font-family: inherit;
  cursor: pointer;
}

.item:hover {
  background: var(--bg-hover);
}
</style>
