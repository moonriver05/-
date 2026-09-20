<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import type { FeedbackItem } from '../types'
import { feedbackKindLabel, feedbackStatusLabel } from '../moderation'
const emit = defineEmits<{ close: [] }>()
const items = ref<FeedbackItem[]>([]), status = ref('all'), page = ref(1), total = ref(0), busy = ref(false), error = ref('')
const panel = ref<HTMLElement | null>(null)
let generation = 0
onMounted(() => { panel.value?.focus(); void load() })
onUnmounted(() => { generation++ })
async function load() {
  const version = ++generation; busy.value = true; error.value = ''
  try { const result = await api.myFeedback(status.value, page.value); if (version === generation) { items.value = result.items; total.value = result.total } }
  catch (e) { if (version === generation) error.value = e instanceof Error ? e.message : '加载失败' }
  finally { if (version === generation) busy.value = false }
}
function trapFocus(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const buttons = [...(panel.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])]
  const first = buttons[0], last = buttons.at(-1)
  if (event.shiftKey && (document.activeElement === first || document.activeElement === panel.value)) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
}
</script>
<template>
  <div class="moderation-overlay" @keydown.esc="emit('close')"><section ref="panel" class="moderation-panel" role="dialog" aria-modal="true" aria-labelledby="my-feedback-title" tabindex="-1" @keydown="trapFocus">
    <div class="community-heading"><h2 id="my-feedback-title">我的反馈</h2><button class="text-btn" @click="emit('close')">关闭</button></div>
    <p class="muted">查看你提交的补充、纠错和解释评价，以及管理员的处理结果。</p>
    <div class="moderation-tabs"><button v-for="tab in [{ key: 'all', name: '全部' }, { key: 'pending', name: '待处理' }, { key: 'resolved', name: '已处理' }, { key: 'dismissed', name: '不采纳' }]" :key="tab.key" :class="{ active: status === tab.key }" :disabled="busy" @click="status = tab.key; page = 1; load()">{{ tab.name }}</button><span>{{ total }} 条</span></div>
    <p v-if="error" class="inline-error" role="alert">{{ error }} <button @click="load">重试</button></p><p v-if="busy" role="status">正在加载…</p>
    <template v-else-if="!error"><article v-for="item in items" :key="item.id" class="moderation-note"><div class="community-heading"><strong>{{ item.articleTitle || '未关联条目' }}</strong><small>{{ feedbackKindLabel(item.kind) }} · {{ feedbackStatusLabel(item.status) }}</small></div><p>{{ item.body || '快捷评价：' + feedbackKindLabel(item.kind) }}</p><small class="muted">{{ new Date(item.createdAt).toLocaleString('zh-CN') }}</small><p v-if="item.resolution" class="feedback-resolution">管理员回复：{{ item.resolution }}</p></article><p v-if="!items.length" class="empty-state">还没有对应的反馈记录。可在条目信息面板中提出补充 / 纠错。</p></template>
    <div class="community-form-footer"><button class="text-btn" :disabled="page <= 1 || busy" @click="page--; load()">上一页</button><span>{{ page }} / {{ Math.max(1, Math.ceil(total / 50)) }}</span><button class="text-btn" :disabled="page * 50 >= total || busy" @click="page++; load()">下一页</button></div>
  </section></div>
</template>
