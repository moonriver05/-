import DOMPurify from 'dompurify'

export type MediaEntry = { url: string; author: string; license: string; licenseUrl: string; sourceUrl: string }
type MediaManifest = Record<string, MediaEntry>
export type ImageMode = 'external' | 'mirror-only' | 'off'
const buildEnv = import.meta.env ?? {}
export const imageMode: ImageMode = ['external', 'mirror-only', 'off'].includes(buildEnv.VITE_WIKI_IMAGE_MODE)
  ? buildEnv.VITE_WIKI_IMAGE_MODE : 'external'
const allowedOrigins = new Set<string>((buildEnv.VITE_MEDIA_ALLOWED_ORIGINS ?? '').split(',').map((s: string) => s.trim()).filter(Boolean))
let manifestPromise: Promise<MediaManifest> | undefined

function https(value: string) {
  try { return new URL(value).protocol === 'https:' } catch { return false }
}

export function wikiFilename(src: string): string | null {
  try {
    const url = new URL(src)
    if (url.origin !== 'https://commons.wikimedia.org' || !url.pathname.startsWith('/wiki/Special:FilePath/')) return null
    return decodeURIComponent(url.pathname.slice('/wiki/Special:FilePath/'.length)).replace(/ /g, '_')
  } catch { return null }
}

export function loadMediaManifest(): Promise<MediaManifest> {
  if (manifestPromise) return manifestPromise
  const configured = buildEnv.VITE_MEDIA_MANIFEST_URL as string | undefined
  if (!configured) return Promise.resolve({})
  manifestPromise = (async () => {
    try {
      const url = new URL(configured, window.location.origin)
      if (url.origin !== window.location.origin && (url.protocol !== 'https:' || !allowedOrigins.has(url.origin))) return {}
      const response = await fetch(url.href, { signal: AbortSignal.timeout(8000), credentials: 'omit', referrerPolicy: 'no-referrer' })
      if (!response.ok) return {}
      const body: unknown = await response.json()
      if (!body || typeof body !== 'object' || Array.isArray(body)) return {}
      const result: MediaManifest = {}
      for (const [name, item] of Object.entries(body)) {
        if (!item || typeof item !== 'object') continue
        const entry = item as MediaEntry
        if (![entry.url, entry.author, entry.license, entry.licenseUrl, entry.sourceUrl].every(v => typeof v === 'string' && v.trim())) continue
        if (!https(entry.url) || !allowedOrigins.has(new URL(entry.url).origin) || !https(entry.licenseUrl) || !https(entry.sourceUrl)) continue
        result[name.replace(/ /g, '_')] = entry
      }
      return result
    } catch { return {} }
  })()
  return manifestPromise
}

// Validate URLs before inserting HTML: checking after v-html is too late to stop requests.
export function safeWikiHTML(value: string, manifest: MediaManifest = {}, mode: ImageMode = imageMode): string {
  const fragment = DOMPurify.sanitize(value, {
    RETURN_DOM_FRAGMENT: true,
    ALLOWED_TAGS: ['p', 'div', 'span', 'a', 'b', 'strong', 'i', 'em', 'u', 's', 'small', 'sup', 'sub', 'br', 'hr', 'ul', 'ol', 'li', 'dl', 'dt', 'dd', 'table', 'caption', 'thead', 'tbody', 'tr', 'th', 'td', 'figure', 'img', 'figcaption', 'blockquote', 'pre', 'code', 'details', 'summary'],
    // src is deliberately disallowed; a narrow hook below captures and validates it.
    ALLOWED_ATTR: ['href', 'alt', 'title', 'class', 'id', 'colspan', 'rowspan', 'loading', 'referrerpolicy', 'data-tex', 'data-display', 'aria-label', 'data-wiki-src'],
    ALLOW_DATA_ATTR: false,
  })
  for (const anchor of fragment.querySelectorAll('a[href]')) {
    const href = anchor.getAttribute('href') ?? ''
    if (/^#wiki-ref-\d+-\d+$/.test(href)) continue
    if (!/^https?:\/\//i.test(href)) anchor.removeAttribute('href')
    else { anchor.setAttribute('target', '_blank'); anchor.setAttribute('rel', 'noopener noreferrer') }
  }
  for (const img of fragment.querySelectorAll<HTMLImageElement>('img')) {
    const original = img.getAttribute('data-wiki-src') ?? ''
    const filename = wikiFilename(original)
    const entry = filename ? manifest[filename] : undefined
    const caption = img.closest('figure')?.querySelector('figcaption')
    if (filename && img.alt === filename.replace(/_/g, ' ') && caption) {
      // A raw filename is useful as alt text, but is not an article caption.
      for (const child of [...caption.childNodes]) {
        if (child.nodeType === 3) child.remove()
      }
    }
    img.removeAttribute('data-wiki-src')
    img.setAttribute('loading', 'lazy')
    img.setAttribute('decoding', 'async')
    img.setAttribute('referrerpolicy', 'no-referrer')
    if (mode === 'off' || !filename || (mode === 'mirror-only' && !entry)) {
      img.remove()
      continue
    }
    img.src = entry?.url ?? original
    if (entry) {
      if (caption) {
        caption.querySelector('small')?.remove()
        const credit = document.createElement('small')
        credit.textContent = `作者：${entry.author} · `
        const license = document.createElement('a')
        license.href = entry.licenseUrl
        license.textContent = entry.license
        license.target = '_blank'
        license.rel = 'noopener noreferrer'
        credit.append(license)
        caption.append(credit)
      }
    }
  }
  const container = document.createElement('div')
  container.append(fragment)
  return container.innerHTML
}

// Original HTML is parsed in DOMPurify's inert document; only vetted Commons
// URLs survive as inert data attributes, never as an active untrusted src.
DOMPurify.addHook('uponSanitizeAttribute', (node, data) => {
  if (node.nodeName === 'IMG' && data.attrName === 'src') {
    if (wikiFilename(data.attrValue)) node.setAttribute('data-wiki-src', data.attrValue)
    data.keepAttr = false
  }
})
