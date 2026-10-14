<script setup lang="ts">
// The icon of an entity type in its colour.
import { computed } from 'vue'
import { iconFor } from '@/composables/entityIcons'
import { useCatalogStore } from '@/stores/catalog'

const props = withDefaults(defineProps<{ type: string; size?: number }>(), { size: 14 })
const catalog = useCatalogStore()
const entity = computed(() => catalog.type(props.type))
const icon = computed(() => iconFor(entity.value?.icon))
const color = computed(() => entity.value?.color ?? catalog.color(props.type))
</script>

<template>
  <component :is="icon" class="entity-icon" :size="size" :stroke-width="2" :style="{ color }" />
</template>

<style scoped>
.entity-icon {
  flex: none;
}
</style>
