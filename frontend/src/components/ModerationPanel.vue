<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import UserAvatar from './UserAvatar.vue'
import WikiContent from './WikiContent.vue'
import AccountManagement from './AccountManagement.vue'
import { articleReviewLabel, feedbackKindLabel, feedbackStatusLabel, type ModerationSection } from '../moderation'
import type { Annotation, Article, FeedbackItem } from '../types'
const props = withDefaults(defineProps<{ initialSection?: ModerationSection; initialStatus?: string; initialQuery?: string; currentUserId?: number }>(), { initialSection: 'articles', initialStatus: 'pending', initialQuery: '' })
const emit = defineEmits<{ close: []; changed: [section: ModerationSection] }>()
const section = ref<ModerationSection>(props.initialSection), status = ref(props.initialStatus), query = ref(props.initialQuery)
const notes = ref<Annotation[]>([]), articles = ref<Article[]>([]), feedback = ref<FeedbackItem[]>([])
const total = ref(0), page = ref(1), busy = ref(false), previewBusy = ref(false), error = ref('')
const resolutions = ref<Record<number, string>>({}), preview = ref<Article | null>(null)
const panel = ref<HTMLElement | null>(null), previewElement = ref<HTMLElement | null>(null)
const managementMode = ref<'content' | 'users' | 'audits'>('content')
let generation = 0, disposed = false
onMounted(() => { panel.value?.focus(); void load() })
onUnmounted(() => { generation++; disposed = true })
const tabs = computed(() => section.value === 'articles' ? [ ['pending', '新建待审核'], ['unreviewed', '导入待核验'], ['approved', '已通过'], ['rejected', '已驳回'], ['deleted', '回收站'] ] : section.value === 'annotations' ? [ ['pending', '待审核'], ['visible', '已公开'], ['rejected', '已驳回'] ] : [ ['pending', '待处理'], ['resolved', '已处理'], ['dismissed', '不采纳'] ])
const count = computed(() => section.value === 'articles' ? articles.value.length : section.value === 'annotations' ? notes.value.length : feedback.value.length)
function close() { if (!busy.value && !previewBusy.value) emit('close') }
function trapFocus(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const controls = [...(panel.value?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), textarea:not(:disabled), a[href], summary') ?? [])].filter(el => el.getClientRects().length)
  const first = controls[0], last = controls.at(-1)
  if (!first) { event.preventDefault(); return }
  if (event.shiftKey && (document.activeElement === first || document.activeElement === panel.value)) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}
async function load() {
  const version = ++generation; busy.value = true; error.value = ''; preview.value = null
  try {
    const result = section.value === 'articles' ? await api.adminArticles(status.value, page.value, query.value) : section.value === 'annotations' ? await api.moderationQueue(status.value, page.value) : await api.adminFeedback(status.value, page.value)
    if (version !== generation) return
    total.value = result.total
    if (page.value > 1 && !result.items.length) { page.value--; await load(); return }
    if (section.value === 'articles') articles.value = result.items as Article[]
    else if (section.value === 'annotations') notes.value = result.items as Annotation[]
    else feedback.value = result.items as FeedbackItem[]
  } catch (e) { if (version === generation) error.value = e instanceof Error ? e.message : '加载失败' }
  finally { if (version === generation) busy.value = false }
}
function switchSection(next: ModerationSection) { managementMode.value = 'content'; section.value = next; status.value = 'pending'; page.value = 1; query.value = ''; void load() }
function switchStatus(next: string) { status.value = next; page.value = 1; void load() }
async function run(action: () => Promise<unknown>) {
  if (busy.value || previewBusy.value) return
  busy.value = true; error.value = ''
  try { await action(); emit('changed', section.value); await load() }
  catch (e) { error.value = e instanceof Error ? e.message : '操作失败' }
  finally { busy.value = false }
}
function actArticle(item: Article, action: 'approved' | 'rejected' | 'delete' | 'restore') {
  if (action === 'delete' && !window.confirm(`将「${item.title}」移入回收站？它将从知识库隐藏，关联讨论保留，可恢复。`)) return
  if (action === 'rejected' && item.published && !window.confirm(`驳回「${item.title}」后将不再公开显示，确定吗？`)) return
  void run(() => action === 'delete' ? api.deleteArticle(item.id) : action === 'restore' ? api.restoreArticle(item.id) : api.reviewArticle(item, action))
}
function actNote(note: Annotation, action: 'visible' | 'rejected' | 'delete') {
  if (action === 'delete' && !window.confirm('确定删除这条批注及其回复？')) return
  void run(() => action === 'delete' ? api.deleteAnnotation(note.id) : api.reviewAnnotation(note.id, action))
}
async function showPreview(id: number) {
  previewBusy.value = true; error.value = ''
  try { const result = await api.adminArticle(id); if (disposed) return; preview.value = result; await nextTick(); previewElement.value?.scrollIntoView({ behavior: 'smooth', block: 'start' }); previewElement.value?.focus({ preventScroll: true }) }
  catch (e) { error.value = e instanceof Error ? e.message : '无法加载条目' }
  finally { previewBusy.value = false }
}
</script>
<template>
  <div class="moderation-overlay" @keydown.esc="close">
    <section ref="panel" class="moderation-panel content-management" role="dialog" aria-modal="true" aria-labelledby="moderation-title" tabindex="-1" @keydown="trapFocus">
      <div class="community-heading"><h2 id="moderation-title">审核管理</h2><button class="text-btn" :disabled="busy || previewBusy" @click="close">关闭</button></div>
      <div class="management-sections" aria-label="审核分类"><button v-for="tab in [{ key: 'articles', name: '条目审核' }, { key: 'annotations', name: '批注审核' }, { key: 'feedback', name: '纠错反馈' }]" :key="tab.key" :aria-pressed="managementMode === 'content' && section === tab.key" :class="{ active: managementMode === 'content' && section === tab.key }" :disabled="busy || previewBusy" @click="switchSection(tab.key as ModerationSection)">{{ tab.name }}</button></div>
      <div class="management-sections" aria-label="账户管理分类"><button :class="{ active: managementMode === 'users' }" :aria-pressed="managementMode === 'users'" :disabled="busy || previewBusy" @click="managementMode = 'users'">用户管理</button><button :class="{ active: managementMode === 'audits' }" :aria-pressed="managementMode === 'audits'" :disabled="busy || previewBusy" @click="managementMode = 'audits'">操作日志</button></div>
      <AccountManagement v-if="managementMode !== 'content'" :key="managementMode" :mode="managementMode" :current-user-id="currentUserId" />
      <template v-else>
      <p class="muted management-description">{{ section === 'articles' ? '新建条目通过后公开。导入内容可继续阅读，人工核验后再标记通过。删除可在回收站恢复。' : section === 'annotations' ? '批注通过后公开显示。删除会同时隐藏该批注下的回复。' : '查看用户的补充、纠错与解释反馈。处理说明会显示给提交者；标记已处理不会自动修改正文。' }}</p>
      <form v-if="section === 'articles'" class="management-search" @submit.prevent="page = 1; load()"><input v-model="query" aria-label="搜索审核条目标题" placeholder="按条目标题搜索" :disabled="busy || previewBusy" /><button class="text-btn" :disabled="busy || previewBusy">搜索</button></form>
      <div class="moderation-tabs"><button v-for="tab in tabs" :key="tab[0]" :class="{ active: status === tab[0] }" :disabled="busy || previewBusy" @click="switchStatus(tab[0])">{{ tab[1] }}</button><span>{{ total }} 条</span></div>
      <p v-if="error" class="inline-error" role="alert">{{ error }} <button :disabled="busy || previewBusy" @click="load">刷新列表</button></p>
      <p v-if="busy || previewBusy" class="muted" role="status">正在处理…</p>
      <template v-if="!busy && !error">
        <p v-if="!count" class="empty-state">当前分类下没有记录。</p>
        <article v-for="item in section === 'articles' ? articles : []" :key="item.id" class="moderation-note">
          <div class="community-heading"><strong>{{ item.title }}</strong><small>{{ item.category }} · #{{ item.id }}</small></div>
          <p>{{ item.excerpt }}</p><small class="muted">{{ status === 'deleted' ? '已删除 · 可恢复' : articleReviewLabel(item.reviewStatus) }} · {{ item.published && status !== 'deleted' ? '公开可见' : '未公开' }}</small>
          <div class="community-form-footer"><button class="text-btn" :disabled="previewBusy" @click="showPreview(item.id)">查看完整正文</button><button v-if="status === 'deleted'" class="primary-btn" :disabled="previewBusy" @click="actArticle(item, 'restore')">恢复条目</button><template v-else><button class="text-btn danger" :disabled="previewBusy" @click="actArticle(item, 'delete')">删除</button><button v-if="item.reviewStatus !== 'rejected'" class="text-btn" :disabled="previewBusy" @click="actArticle(item, 'rejected')">驳回</button><button v-if="item.reviewStatus !== 'approved'" class="primary-btn" :disabled="previewBusy" @click="actArticle(item, 'approved')">通过并公开</button></template></div>
        </article>
        <article v-for="note in section === 'annotations' ? notes : []" :key="note.id" class="moderation-note">
          <div class="community-heading"><UserAvatar :src="note.avatar" :name="note.author" /><strong>{{ note.author }}</strong><small>条目 #{{ note.articleId }} · 批注 #{{ note.id }}</small></div><blockquote>{{ note.quote }}</blockquote><p>{{ note.body }}</p>
          <div class="community-form-footer"><button class="text-btn" :disabled="previewBusy" @click="showPreview(note.articleId)">查看原条目</button><button class="text-btn danger" :disabled="previewBusy" @click="actNote(note, 'delete')">删除</button><template v-if="status === 'pending'"><button class="text-btn" :disabled="previewBusy" @click="actNote(note, 'rejected')">驳回</button><button class="primary-btn" :disabled="previewBusy" @click="actNote(note, 'visible')">通过并公开</button></template></div>
        </article>
        <article v-for="item in section === 'feedback' ? feedback : []" :key="item.id" class="moderation-note">
          <div class="community-heading"><strong>{{ feedbackKindLabel(item.kind) }} · {{ item.articleTitle || '未关联条目' }}</strong><small>#{{ item.id }} · {{ feedbackStatusLabel(item.status) }}</small></div><p>{{ item.body || '用户通过快捷按钮提交了评价，未填写文字。' }}</p><small class="muted">{{ item.author || '用户' }} · {{ new Date(item.createdAt).toLocaleString('zh-CN') }}</small>
          <p v-if="item.resolution" class="feedback-resolution">处理说明：{{ item.resolution }}</p>
          <label v-if="item.status === 'pending'" class="feedback-resolution-input">处理说明<textarea v-model="resolutions[item.id]" maxlength="2000" rows="2" placeholder="填写核对结果、修正情况或不采纳的原因，提交者可见" /></label>
          <div class="community-form-footer"><button v-if="item.articleId" class="text-btn" :disabled="previewBusy" @click="showPreview(item.articleId)">查看相关条目</button><template v-if="item.status === 'pending'"><button class="text-btn" :disabled="previewBusy || !resolutions[item.id]?.trim()" @click="run(() => api.reviewFeedback(item.id, 'dismissed', resolutions[item.id]))">不采纳</button><button class="primary-btn" :disabled="previewBusy || !resolutions[item.id]?.trim()" @click="run(() => api.reviewFeedback(item.id, 'resolved', resolutions[item.id]))">标记已处理</button></template></div>
        </article>
      </template>
      <div class="community-form-footer"><button class="text-btn" :disabled="page <= 1 || busy || previewBusy" @click="page--; load()">上一页</button><span>{{ page }} / {{ Math.max(1, Math.ceil(total / 50)) }}</span><button class="text-btn" :disabled="page * 50 >= total || busy || previewBusy" @click="page++; load()">下一页</button></div>
      <section v-if="preview" ref="previewElement" class="management-preview" tabindex="-1" aria-label="条目正文预览"><div class="community-heading"><h2>{{ preview.title }}</h2><button class="text-btn" @click="preview = null">收起正文</button></div><p class="muted">{{ articleReviewLabel(preview.reviewStatus) }}</p><div v-for="part in preview.sections" :key="part.id"><h3>{{ part.title }}</h3><WikiContent v-if="part.contentHtml" :html="part.contentHtml" /><p v-else class="plain-section">{{ part.content }}</p><pre v-if="part.formula">{{ part.formula }}</pre></div></section>
      </template>
    </section>
  </div>
</template>
