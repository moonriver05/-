<script setup lang="ts">
import { computed, ref, watch } from 'vue'
const props = withDefaults(defineProps<{ src?: string; name: string; size?: number }>(), { size: 28 })
const failed = ref(false)
watch(() => props.src, () => { failed.value = false })
const source = computed(() => {
  try { const u = new URL(props.src || ''); return u.protocol === 'https:' && !u.username && !u.password ? u.href : '' }
  catch { return '' }
})
</script>
<template>
  <span class="user-avatar" :style="{ width: `${size}px`, height: `${size}px`, fontSize: `${Math.round(size * .4)}px` }">
    <img v-if="source && !failed" :src="source" :alt="`${name}的头像`" referrerpolicy="no-referrer" @error="failed = true" />
    <span v-else aria-hidden="true">{{ Array.from(name.trim())[0] || '学' }}</span>
  </span>
</template>
<style scoped>
.user-avatar { display: inline-flex; align-items: center; justify-content: center; vertical-align: middle; flex-shrink: 0; overflow: hidden; border-radius: 50%; background: var(--red-soft); color: var(--red); font-weight: 500; }
.user-avatar img { width: 100%; height: 100%; object-fit: cover; }
</style>
