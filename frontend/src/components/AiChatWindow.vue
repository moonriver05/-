<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { handleComposerKey } from '../explanationStream'
import type { ChatTurn, ChatWindow } from '../sharedChats'
import type { Explanation } from '../types'
import AiMessage from './AiMessage.vue'
import { safeSourceURL } from '../aiText'
import { answerBadge, groundingNotice, webSearchNotice } from '../answerStatus'

const props = defineProps<{
  chat: ChatWindow; windows: ChatWindow[]; turns: ChatTurn[]; pending?: ChatTurn
  active: boolean; articleTitle: string; sharedRounds: number; aiConfigured: boolean
  searchConfigured: boolean
}>()
const emit = defineEmits<{
  send: [id: number]; stop: [id: number]; close: [id: number]; minimize: [id: number]
  activate: [id: number]; add: []; citation: [id: number, cite: Explanation['citations'][number]]
  feedback: [kind: string]
}>()
const root = ref<HTMLElement>()
const input = ref<HTMLTextAreaElement>()
let drag: { id: number; x: number; y: number; left: number; top: number } | null = null

function clampPosition() {
  if (!root.value || window.innerWidth <= 850) return
  const rect = root.value.getBoundingClientRect()
  props.chat.x = Math.max(8, Math.min(props.chat.x, window.innerWidth - rect.width - 8))
  props.chat.y = Math.max(60, Math.min(props.chat.y, window.innerHeight - rect.height - 8))
}
function startDrag(event: PointerEvent) {
  if (event.button !== 0 || window.innerWidth <= 850 || (event.target as Element).closest('button,select')) return
  drag = { id: event.pointerId, x: event.clientX, y: event.clientY, left: props.chat.x, top: props.chat.y }
  const header = event.currentTarget as HTMLElement
  header.setPointerCapture(event.pointerId)
  event.preventDefault()
}
function moveDrag(event: PointerEvent) {
  if (!drag || drag.id !== event.pointerId) return
  props.chat.x = drag.left + event.clientX - drag.x
  props.chat.y = drag.top + event.clientY - drag.y
  clampPosition()
}
function focusInput() { nextTick(() => { if (props.active && !props.chat.minimized) input.value?.focus({ preventScroll: true }) }) }
onMounted(() => { clampPosition(); focusInput(); window.addEventListener('resize', clampPosition) })
onUnmounted(() => window.removeEventListener('resize', clampPosition))
watch(() => [props.active, props.chat.minimized], focusInput)
</script>

<template>
  <section v-show="!chat.minimized" ref="root" class="floating-chat" :class="{ 'is-current': active }" :style="{ left: chat.x + 'px', top: chat.y + 'px', '--chat-left': chat.x + 'px', '--chat-top': chat.y + 'px', zIndex: active ? 90 : 80 }" :data-chat-window="chat.id" role="dialog" :aria-label="`AI ${chat.name}：${articleTitle}`" @pointerdown="emit('activate', chat.id)">
    <header class="floating-chat-header" @pointerdown="startDrag" @pointermove="moveDrag" @pointerup="drag = null" @pointercancel="drag = null" @lostpointercapture="drag = null">
      <strong>✦ AI · {{ chat.name }}</strong>
      <select class="chat-mobile-switch" aria-label="切换 AI 窗口" :value="chat.id" @change="emit('activate', Number(($event.target as HTMLSelectElement).value))"><option v-for="item in windows" :key="item.id" :value="item.id">{{ item.name }}</option></select>
      <div class="floating-chat-actions">
        <button type="button" aria-label="新建子窗口" title="新建子窗口" :disabled="windows.length >= 4" @click.stop="emit('add')">＋</button>
        <button type="button" :aria-label="`收起${chat.name}`" title="收起窗口，回答继续进行" @click.stop="emit('minimize', chat.id)">−</button>
        <button type="button" :aria-label="`关闭${chat.name}`" title="关闭窗口；已完成问答仍保留在共享上下文" @click.stop="emit('close', chat.id)">×</button>
      </div>
    </header>
    <div class="floating-chat-topic" :title="articleTitle">{{ articleTitle }}</div>
    <div class="chat-scroll" aria-live="polite" aria-relevant="additions text">
      <p v-if="!aiConfigured" class="service-notice">尚未配置模型，当前仅提供词条引用示例。</p>
      <div v-if="!turns.length" class="floating-chat-empty"><span>✦</span><strong>从另一个角度继续问</strong><p>这里单独展示问答，也能接着其他窗口已完成的回答追问。</p></div>
      <article v-for="turn in turns" :key="turn.id" class="conversation-turn">
        <p class="user-question">{{ turn.question }}</p>
        <div v-if="turn.result" class="answer-card">
          <div v-if="turn.result.kind !== 'conversation'" class="answer-head"><span class="verified-badge">{{ answerBadge(turn.result) }}</span></div>
          <p v-if="turn.result.kind !== 'conversation' && groundingNotice(turn.result)" class="grounding-notice">{{ groundingNotice(turn.result) }}</p>
          <p v-if="webSearchNotice(turn.result)" class="web-search-notice">{{ webSearchNotice(turn.result) }}</p>
          <details v-if="turn.result.webSearch?.sources?.length" class="web-source-list"><summary>联网资料</summary><ul><li v-for="source in turn.result.webSearch.sources" :key="source.url"><a v-if="safeSourceURL(source.url)" :href="safeSourceURL(source.url)" target="_blank" rel="noopener noreferrer">{{ source.title }} ↗</a><small>{{ source.summaryOnly ? '搜索摘要，非完整正文' : '网页正文节选' }}</small></li></ul></details>
          <AiMessage class="answer-summary" :text="turn.result.summary" />
          <p v-if="turn.result.retrieval && turn.result.kind !== 'conversation' && turn.result.mode === 'model'" class="retrieval-status">{{ turn.result.retrieval.mode === 'chroma' ? `知识检索 · ${turn.result.retrieval.count} 段证据` : '依据当前词条回答' }}</p>
          <div class="answer-steps"><div v-for="(step, index) in turn.result.steps" :key="index"><b>{{ index + 1 }}</b><section><strong>{{ step.title }}</strong><AiMessage :text="step.content" /></section></div></div>
          <details v-if="turn.result.citations.length"><summary>依据与核验 <span>{{ turn.result.citations.length }} 条引用</span></summary><div v-for="(cite, index) in turn.result.citations" :key="index" class="citation"><span>{{ cite.section }}</span><p>“{{ cite.quote }}”</p><a v-if="cite.sourceType === 'web' && safeSourceURL(cite.sourceUrl)" class="web-source-link" :href="safeSourceURL(cite.sourceUrl)" target="_blank" rel="noopener noreferrer">查看网页来源 ↗</a><button v-else-if="cite.articleSlug && cite.sectionId" class="text-btn" @click="emit('citation', chat.id, cite)">查看原文</button></div></details>
          <div v-if="turn.result.kind !== 'conversation'" class="answer-actions"><span>这条解释有帮助吗？</span><button @click="emit('feedback', 'helpful')">有帮助</button><button @click="emit('feedback', 'needs_improvement')">需改进</button></div>
        </div>
        <template v-else>
          <AiMessage v-if="turn.text" class="stream-answer" :text="turn.text" />
          <p v-if="turn.status === 'queued' || turn.status === 'running'" class="stream-status" role="status">{{ turn.status === 'queued' ? '正在排队，将接着前面完成的回答继续…' : turn.phase === 'searching' ? '正在联网补充资料…' : turn.phase === 'verifying' ? '正在核验引用与结论…' : turn.phase === 'retrieving' ? '正在检索相关知识…' : turn.text ? '正在回答…' : '正在连接模型…' }}</p>
          <p v-if="turn.error" class="inline-error" role="alert">{{ turn.error }}</p>
        </template>
      </article>
    </div>
    <footer class="floating-chat-composer">
      <div class="chat-shared-label">◈ 共用本词条上下文 · 最近 {{ sharedRounds }} 轮</div>
      <details class="chat-preferences"><summary>讲解设置 <span>{{ chat.audience }} · {{ chat.answerMode === 'strict' ? '严格依据资料' : '智能讲解' }}{{ chat.webSearch ? ' · 联网已开' : '' }}</span></summary>
      <div class="chat-window-settings"><select v-model="chat.audience" aria-label="子窗口学习程度"><option>小学生</option><option>高中生</option><option>本科生</option><option>专业学习者</option></select><select v-model="chat.style" aria-label="子窗口解释方式"><option>直觉 + 严谨</option><option>生活类比</option><option>公式推导</option><option>例题演示</option></select></div>
      <div class="answer-scope-controls"><label>范围<select v-model="chat.answerMode" aria-label="子窗口讲解范围"><option value="balanced">智能讲解</option><option value="strict">严格依据资料</option></select></label><label class="web-search-option" :title="searchConfigured ? '向 DeepSeek 发送问题及少量上下文，按次计费' : '服务器尚未启用联网搜索'"><input v-model="chat.webSearch" type="checkbox" :disabled="!searchConfigured" />联网补充<small>{{ searchConfigured ? "按次计费" : "暂不可用" }}</small></label></div>
      </details>
      <textarea ref="input" v-model="chat.prompt" rows="3" maxlength="2000" :aria-label="`${chat.name}的问题`" placeholder="独立提问，也可以接着其他窗口追问…" @keydown="handleComposerKey($event, () => emit('send', chat.id))" />
      <div class="floating-chat-send"><span>Enter 发送 · Ctrl + Enter 换行</span><button v-if="pending" class="text-btn" @click="emit('stop', chat.id)">{{ pending.status === 'queued' ? '取消排队' : '停止生成' }}</button><button v-else class="primary-btn" :disabled="!chat.prompt.trim()" @click="emit('send', chat.id)">发送</button></div>
    </footer>
  </section>
</template>
