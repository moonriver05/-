// Offsets use DOM/JavaScript UTF-16 units, not UTF-8 bytes or Unicode code points.
export type TextAnchor = {
  quote: string
  prefix?: string
  suffix?: string
  startOffset?: number
  endOffset?: number
}
export type Highlight = TextAnchor & { id: number }

export function locateQuote(text: string, anchor: TextAnchor): { start: number; end: number } | null {
  if (!anchor.quote) return null
  const matches: number[] = []
  let position = text.indexOf(anchor.quote)
  while (position !== -1) {
    matches.push(position)
    position = text.indexOf(anchor.quote, position + 1)
  }
  const contextual = matches.filter(start =>
    (!anchor.prefix || text.slice(0, start).endsWith(anchor.prefix)) &&
    (!anchor.suffix || text.slice(start + anchor.quote.length).startsWith(anchor.suffix)))
  const offset = contextual.find(start => start === anchor.startOffset && start + anchor.quote.length === anchor.endOffset)
  // Never highlight every occurrence of a repeated quote. Old notes without an
  // anchor only resolve when the text is unique in their section.
  const start = offset ?? (contextual.length === 1 ? contextual[0] : matches.length === 1 ? matches[0] : undefined)
  return start === undefined ? null : { start, end: start + anchor.quote.length }
}

export function anchorAt(text: string, start: number, end: number): TextAnchor {
  return {
    quote: text.slice(start, end), startOffset: start, endOffset: end,
    prefix: [...text.slice(0, start)].slice(-32).join(''), suffix: [...text.slice(end)].slice(0, 32).join('')
  }
}

export function captureAnchor(root: HTMLElement, range: Range): TextAnchor | null {
  if (!root.contains(range.startContainer) || !root.contains(range.endContainer)) return null
  if ([...root.querySelectorAll('[data-tex], .katex')].some(element => range.intersectsNode(element))) return null
  const before = range.cloneRange()
  before.selectNodeContents(root)
  before.setEnd(range.startContainer, range.startOffset)
  const raw = range.toString()
  const quote = raw.trim()
  if (!quote) return null
  const start = before.toString().length + raw.indexOf(quote)
  return anchorAt(root.textContent ?? '', start, start + quote.length)
}

export function clearHighlights(root: HTMLElement) {
  for (const mark of root.querySelectorAll('mark[data-annotation-ids]')) mark.replaceWith(...mark.childNodes)
  root.normalize()
}

export function paintHighlights(root: HTMLElement, notes: Highlight[], activeId?: number | null) {
  clearHighlights(root)
  const text = root.textContent ?? ''
  const ranges = notes.flatMap(note => {
    const range = locateQuote(text, note)
    return range ? [{ ...range, id: note.id }] : []
  })
  if (!ranges.length) return
  const walker = root.ownerDocument.createTreeWalker(root, 4 /* SHOW_TEXT */)
  const nodes: { node: Text; start: number; end: number; blocked: boolean }[] = []
  let offset = 0
  for (let node = walker.nextNode() as Text | null; node; node = walker.nextNode() as Text | null) {
    const end = offset + node.length
    nodes.push({ node, start: offset, end, blocked: !!node.parentElement?.closest('[data-tex], .katex, script, style') })
    offset = end
  }
  const safeRanges = ranges.filter(range => !nodes.some(n => n.blocked && n.start < range.end && n.end > range.start))
  for (const { node, start, end } of nodes) {
    const intersecting = safeRanges.filter(range => range.start < end && range.end > start)
    if (!intersecting.length) continue
    const boundaries = [...new Set([start, end, ...intersecting.flatMap(r => [Math.max(start, r.start), Math.min(end, r.end)])])].sort((a, b) => a - b)
    const fragment = root.ownerDocument.createDocumentFragment()
    for (let i = 0; i < boundaries.length - 1; i++) {
      const from = boundaries[i]!, to = boundaries[i + 1]!
      const ids = intersecting.filter(r => r.start <= from && r.end >= to).map(r => r.id)
      const slice = root.ownerDocument.createTextNode(node.data.slice(from - start, to - start))
      if (!ids.length) { fragment.append(slice); continue }
      const mark = root.ownerDocument.createElement('mark')
      mark.dataset.annotationIds = ids.join(' ')
      mark.className = 'annotation-highlight'
      mark.classList.toggle('is-active', activeId != null && ids.includes(activeId))
      mark.classList.toggle('is-draft', ids.includes(-1))
      if (ids.some(id => id !== -1)) {
        mark.tabIndex = 0
        mark.setAttribute('role', 'button')
        mark.setAttribute('aria-label', activeId != null && ids.includes(activeId) ? '取消批注高亮' : `查看此处批注（${ids.filter(id => id !== -1).length} 条）`)
      }
      mark.append(slice)
      fragment.append(mark)
    }
    node.replaceWith(fragment)
  }
}
