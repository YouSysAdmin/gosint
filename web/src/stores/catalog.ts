import { defineStore } from 'pinia'
import { ref } from 'vue'
import { listTypes } from '@/api/catalog'
import type { EntityType, TypeCategory } from '@/api/types'

// The entity type catalog, fetched once per document.
export const useCatalogStore = defineStore('catalog', () => {
  const types = ref<EntityType[]>([])
  const categories = ref<TypeCategory[]>([])
  let loading: Promise<void> | null = null

  function load(): Promise<void> {
    loading ??= listTypes().then((r) => {
      types.value = r.types
      categories.value = r.categories
    })
    return loading
  }

  function type(name: string): EntityType | undefined {
    return types.value.find((t) => t.name === name)
  }

  // A stable colour per type: the catalog's, or one derived from the name.
  function color(name: string): string {
    const c = type(name)?.color
    if (c) return c
    let h = 0
    for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) % 360
    return `hsl(${h} 55% 55%)`
  }

  // reload reads the catalog again, after an administrator changed the
  // custom types.
  function reload(): Promise<void> {
    loading = null
    return load()
  }

  return { types, categories, load, reload, type, color }
})
