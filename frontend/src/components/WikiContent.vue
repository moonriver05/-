<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { imageMode, loadMediaManifest, safeWikiHTML, type MediaEntry } from '../wikiMedia'
import katex from 'katex'
import 'katex/dist/katex.min.css'
import { paintHighlights, type Highlight } from '../annotationHighlights'

const props = defineProps<{ html?: string; text?: string; annotations?: Highlight[]; activeAnnotationId?: number | null }>()
const emit = defineEmits<{ annotationClick: [ids: number[]] }>()
const root = ref<HTMLElement>()
const manifest = ref<Record<string, MediaEntry>>({})
const mediaReady = ref(!import.meta.env?.VITE_MEDIA_MANIFEST_URL)
const clean = computed(() => safeWikiHTML(props.html || `<p class="plain-section">${(props.text ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}</p>`, manifest.value, mediaReady.value ? imageMode : 'off'))

function highlight() {
  if (root.value) paintHighlights(root.value, props.annotations ?? [], props.activeAnnotationId)
}

function activateAnnotation(event: MouseEvent | KeyboardEvent) {
  if ('key' in event && event.key !== 'Enter' && event.key !== ' ') return
  const mark = (event.target as Element)?.closest<HTMLElement>('mark[data-annotation-ids]')
  if (!mark || !root.value?.contains(mark) || window.getSelection()?.toString()) return
  const ids = (mark.dataset.annotationIds ?? '').split(' ').map(Number).filter(id => id > 0)
  if (!ids.length) return
  event.preventDefault()
  emit('annotationClick', ids)
}

async function typeset() {
  await nextTick()
  if (!root.value) return
  for (const link of root.value.querySelectorAll<HTMLAnchorElement>('a[href]')) {
    const href = link.getAttribute('href') ?? ''
    if (href.startsWith('#wiki-ref-')) continue
    if (!/^https?:\/\//i.test(href)) {
      link.removeAttribute('href')
    } else {
      link.target = '_blank'
      link.rel = 'noopener noreferrer'
    }
  }
  for (const element of root.value.querySelectorAll<HTMLElement>('[data-tex]')) {
    const tex = element.dataset.tex ?? ''
    if (tex.length > 20000) {
      element.classList.add('math-fallback')
      element.title = '公式过长，保留原始 TeX，可对照来源阅读。'
      continue
    }
    try {
      katex.render(tex, element, {
        displayMode: element.dataset.display === 'true',
        throwOnError: true,
        trust: false,
        strict: 'ignore',
        maxExpand: 1000,
        maxSize: 20,
        macros: { '\\R': '\\mathbb{R}', '\\N': '\\mathbb{N}', '\\Z': '\\mathbb{Z}', '\\Q': '\\mathbb{Q}', '\\C': '\\mathbb{C}' },
      })
    } catch {
      element.textContent = tex
      element.classList.add('math-fallback')
      element.title = '此公式暂未成功排版，保留原始 TeX；可对照原词条。'
    }
  }
  highlight()
}

function assetError(event: Event) {
  if (event.target instanceof HTMLImageElement) event.target.classList.add('image-unavailable')
}

onMounted(typeset)
onMounted(async () => { manifest.value = await loadMediaManifest(); mediaReady.value = true })
watch(clean, typeset, { flush: 'post' })
watch(() => [props.annotations, props.activeAnnotationId], highlight, { flush: 'post', deep: true })
</script>

<template>
  <div ref="root" class="wiki-content" @error.capture="assetError" @click="activateAnnotation" @keydown="activateAnnotation" v-html="clean" />
</template>
