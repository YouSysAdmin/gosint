// useDebounced follows a value and settles ms after its last change, so a
// search typed into does not refilter on every keystroke.
import { onBeforeUnmount, ref, watch, type Ref } from 'vue'

export function useDebounced<T>(source: () => T, ms: number): Ref<T> {
  const out = ref(source()) as Ref<T>
  let timer: ReturnType<typeof setTimeout> | null = null
  watch(source, (v) => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = null
      out.value = v
    }, ms)
  })
  onBeforeUnmount(() => {
    if (timer) clearTimeout(timer)
  })

  return out
}
