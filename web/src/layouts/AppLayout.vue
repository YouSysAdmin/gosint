<script setup lang="ts">
// The shell: a slim bar on top with the pages and the account, the
// page below. A rail on the side cost a fifth of the width for four
// links, so the links sit in the bar instead.
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import BrandMark from '@/components/BrandMark.vue'
import { modKey } from '@/composables/modKey'
import CommandPalette from '@/components/CommandPalette.vue'
import UserMenu from '@/components/UserMenu.vue'
import { useAuthStore } from '@/stores/auth'
import { getIcon } from './icons'

const route = useRoute()
const auth = useAuthStore()

// The admin page shows to administrators only, the server refuses the
// rest anyway.
const nav = computed(() => [
  { to: '/investigations', label: 'Investigations', icon: 'briefcase' },
  { to: '/flows', label: 'Flows', icon: 'layers' },
  { to: '/transforms', label: 'Transforms', icon: 'plug' },
  { to: '/secrets', label: 'Secrets', icon: 'key' },
  ...(auth.user?.admin ? [{ to: '/admin/users', label: 'Admin', icon: 'users' }] : []),
])

// A link covers its page's sub-pages too: Transforms holds the
// templates, Admin every admin page.
const here = (to: string) =>
  route.path.startsWith(to) ||
  (to === '/transforms' && route.path.startsWith('/templates')) ||
  (to === '/admin/users' && route.path.startsWith('/admin'))
const isGraph = computed(() => route.name === 'sketch')
</script>

<template>
  <div class="shell">
    <header class="bar">
      <router-link to="/investigations" class="brand"><BrandMark />gosint</router-link>
      <nav>
        <router-link
          v-for="n in nav"
          :key="n.to"
          :to="n.to"
          class="link"
          :class="{ here: here(n.to) }"
        >
          <span class="glyph" aria-hidden="true" v-html="getIcon(n.icon)"></span>
          <span>{{ n.label }}</span>
        </router-link>
      </nav>
      <span class="hint text-xs text-muted">{{ modKey }}+J to jump anywhere</span>
      <UserMenu />
    </header>
    <main class="main" :class="{ full: isGraph }">
      <!-- Keyed by path, so moving from one sketch to another mounts a
           fresh page instead of reusing the last one's state. -->
      <router-view :key="$route.path" />
    </main>
    <CommandPalette />
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  background: var(--bg-secondary);
}

.bar {
  display: flex;
  align-items: center;
  gap: 16px;
  height: var(--topbar);
  padding: 0 16px;
  background: var(--bg-sidebar);
  border-bottom: 1px solid var(--sidebar-border);
  position: sticky;
  top: 0;
  z-index: var(--z-topbar);
}

.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 16px;
  letter-spacing: -0.01em;
  color: var(--text-primary);
  text-decoration: none;
  margin-right: 8px;
}

nav {
  display: flex;
  gap: 2px;
}

.link {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 10px;
  border-radius: var(--radius-md);
  color: var(--sidebar-text);
  text-decoration: none;
  font-size: 13px;
}

.link:hover {
  background: var(--sidebar-hover);
}

.link.here {
  background: var(--sidebar-active-bg);
  color: var(--sidebar-text-active);
}

.glyph {
  display: inline-flex;
}

.glyph :deep(svg) {
  width: 15px;
  height: 15px;
}

.hint {
  margin-left: auto;
}

.main {
  flex: 1;
  min-width: 0;
  padding: 24px 32px;
}

.main.full {
  padding: 0;
}

@media (max-width: 900px) {
  .link span:last-child,
  .hint {
    display: none;
  }

  .main {
    padding: 16px;
  }
}
</style>
