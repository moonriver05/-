import katex from 'katex'

const escapeHTML = (value: string) => value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')
const mathCache = new Map<string, string>()

// Model output is always escaped. Only KaTeX-generated markup enters v-html;
// trust:false rejects HTML commands, arbitrary links and external resources.
export function renderAIText(value: string): string {
  const text = value.slice(0, 100_000)
  const tokens = /```[\s\S]*?(?:```|$)|`[^`\n]+`|\$\$([\s\S]+?)\$\$|\\\[([\s\S]+?)\\\]|\\\(([\s\S]+?)\\\)|(?<!\\)\$([^\n$]+?)\$/g
  let html = '', from = 0, formulaCount = 0
  const prose = (source: string) => escapeHTML(source).replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
  for (const match of text.matchAll(tokens)) {
    html += prose(text.slice(from, match.index))
    const raw = match[0]
    if (raw.startsWith('```')) {
      html += `<pre><code>${escapeHTML(raw.slice(3).replace(/```$/, '').replace(/^[a-zA-Z0-9_-]+\n/, ''))}</code></pre>`
    } else if (raw.startsWith('`')) {
      html += `<code>${escapeHTML(raw.slice(1, -1))}</code>`
    } else {
      const display = match[1] != null || match[2] != null
      const formula = match[1] ?? match[2] ?? match[3] ?? match[4] ?? ''
      if (++formulaCount > 48 || formula.length > 2000) { html += escapeHTML(raw) }
      else {
        const key = `${display}:${formula}`
        let rendered = mathCache.get(key)
        if (!rendered) {
          try {
            rendered = katex.renderToString(formula, { displayMode: display, throwOnError: true, trust: false, strict: 'ignore', maxExpand: 500, maxSize: 20 })
          } catch { rendered = `<span class="ai-math-fallback" title="公式暂未成功排版，保留原文">${escapeHTML(raw)}</span>` }
          if (rendered.length > 80_000) rendered = escapeHTML(raw)
          if (mathCache.size >= 64) mathCache.delete(mathCache.keys().next().value!)
          mathCache.set(key, rendered)
        }
        html += rendered
      }
    }
    from = match.index + raw.length
  }
  return html + prose(text.slice(from))
}

export function safeSourceURL(value?: string): string | undefined {
  if (!value) return undefined
  try {
    const url = new URL(value)
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) return undefined
    return url.href
  } catch { return undefined }
}
