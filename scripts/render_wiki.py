"""Offline, conservative WikiText rendering; never executes templates or source HTML.

The dump does not include expanded templates/media. Unsupported templates are
retained in an explicit appendix, not guessed or silently replaced with prose.
"""
from __future__ import annotations

import argparse
import html
import json
import re
from collections import Counter
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import quote, urlparse

import mwparserfromhell as mw
from mwparserfromhell import nodes
from opencc import OpenCC

CC = OpenCC('t2s')
HEADINGS = re.compile(r'^(={2,6})\s*(.*?)\s*\1\s*$', re.M)
VARIANT = re.compile(r'-\{(.*?)\}-', re.S)
SKIP_TEMPLATE = re.compile(r'^(?:noteta|note ta|short description|use .*|DEFAULTSORT|reflist|references|refbegin|refend|clear|toc.*|multiple issues|expand|refimprove|unreferenced|more citations needed|citation needed|cn|fact|copy edit|cleanup|wikify|authority control|commons.*|wikisource.*|portal.*|navbox|.*导航|.*小作品|.*stub|about|other uses|main other|hatnote|pp-.*|featured article|good article|FA|GA|maths rating)$', re.I)


def variants(value: str) -> str:
    def replace(match):
        body = re.sub(r'^[A-Za-z-]+\|', '', match[1])
        for variant in ('zh-hans', 'zh-cn', 'zh-sg', 'zh', 'zh-hant', 'zh-tw', 'zh-hk'):
            found = re.search(r'(?:^|;)\s*' + variant + r'\s*:\s*([^;]+)', body, re.I)
            if found:
                return found[1].strip()
        return body.strip()
    return VARIANT.sub(replace, value)


def safe_url(value):
    value = html.unescape(value.strip())
    if value.startswith('//'):
        value = 'https:' + value
    return value if urlparse(value).scheme.lower() in ('http', 'https') else ''


class TextReader(HTMLParser):
    def __init__(self):
        super().__init__()
        self.parts = []
        self.skip = []

    def handle_starttag(self, tag, attrs):
        if tag in ('figure', 'details') or (tag == 'sup' and dict(attrs).get('class') == 'wiki-ref'):
            self.skip.append(tag)
        if not self.skip and tag in ('p', 'div', 'li', 'tr', 'br', 'blockquote'):
            self.parts.append('\n')

    def handle_endtag(self, tag):
        if self.skip and tag == self.skip[-1]:
            self.skip.pop()
        if not self.skip and tag in ('p', 'li', 'div', 'tr', 'blockquote'):
            self.parts.append('\n')

    def handle_data(self, data):
        if not self.skip:
            self.parts.append(data)


def plain_html(value):
    parser = TextReader()
    parser.feed(value)
    return re.sub(r'\n[ \t]*\n(?:[ \t]*\n)*', '\n\n', ''.join(parser.parts)).strip()


class Renderer:
    def __init__(self, page_id=0):
        self.page_id = page_id
        self.references = []
        self.named_refs = {}
        self.unsupported = {}
        self.math_count = 0
        self.image_count = 0
        self.table_count = 0

    def text(self, value):
        return html.escape(CC.convert(str(value)), quote=True)

    def link(self, href, label):
        href = safe_url(href)
        return f'<a href="{html.escape(href, quote=True)}" target="_blank" rel="noreferrer noopener">{label}</a>' if href else label

    def render(self, value, depth=0):
        if depth > 35:
            return '<code>' + self.text(value) + '</code>'
        if isinstance(value, mw.wikicode.Wikicode):
            code = value
        else:
            source = variants(str(value))
            try:
                code = mw.parse(source)
            except (AttributeError, IndexError, ValueError):
                try:
                    code = mw.parse(source, skip_style_tags=True)
                except (AttributeError, IndexError, ValueError):
                    self.unsupported[str(value)] = '复杂维基片段解析失败'
                    return '<span class="wiki-template-marker">〔此段结构暂无法解析，完整原文保留在文末〕</span>'
        return ''.join(self.node(node, depth + 1) for node in code.nodes)

    def math(self, value, display=False):
        self.math_count += 1
        # Do not apply Chinese conversion or strip markup inside TeX.
        tex = html.unescape(str(value).strip())
        return f'<span class="wiki-math" data-tex="{html.escape(tex, quote=True)}" data-display="{str(display).lower()}">{html.escape(tex)}</span>'

    def image(self, title, options, depth):
        self.image_count += 1
        filename = str(title).split(':', 1)[-1].strip().replace(' ', '_')
        parts = str(options or '').split('|')
        ignored = r'^(?:thumb|thumbnail|frame|framed|frameless|border|right|left|center|none|upright(?:=.*)?|\d*(?:x\d+)?px|alt=.*|link=.*|class=.*|lang=.*|page=.*)$'
        captions = [p.strip() for p in parts if p.strip() and not re.match(ignored, p.strip(), re.I)]
        caption = self.render(captions[-1], depth) if captions else self.text(filename.replace('_', ' '))
        source = 'https://commons.wikimedia.org/wiki/File:' + quote(filename, safe='')
        image = 'https://commons.wikimedia.org/wiki/Special:FilePath/' + quote(filename, safe='') + '?width=720'
        return '<figure><img loading="lazy" referrerpolicy="no-referrer" src="' + html.escape(image, quote=True) + '" alt="' + html.escape(plain_html(caption), quote=True) + '"><figcaption>' + caption + ' · ' + self.link(source, '图源与授权') + '<small>插图需联网加载；无法显示时可查看图源。</small></figcaption></figure>'

    def template(self, node, depth):
        name = str(node.name).strip()
        low = name.lower().replace('_', ' ')
        params = {str(p.name).strip(): str(p.value).strip() for p in node.params}
        p = lambda key, default='': params.get(str(key), default)
        render = lambda value: self.render(value, depth)
        if SKIP_TEMPLATE.match(low):
            return ''
        if low in ('math', 'mvar', 'var', 'maths'):
            return '<span class="wiki-inline-math">' + render(p(1)) + '</span>'
        if low in ('nowrap', 'nobr', 'small', 'smaller', 'big', 'larger', 'center', 'resize', 'color'):
            return render(p(2) if low in ('resize', 'color') else p(1))
        if low in ('le', 'internal link helper/en'):
            return self.link('https://zh.wikipedia.org/wiki/' + quote(p(1), safe=''), render(p(3, p(1))))
        if low == 'wayback':
            url = safe_url(p('url', p(1)))
            stamp = re.sub(r'\D', '', p('date', p(2)))
            return '（' + self.link('https://web.archive.org/web/' + stamp + '/' + url, '存档') + '）' if url else ''
        if low in ('code', 'kbd', 'tt'):
            return '<code>' + render(p(1)) + '</code>'
        if low in ('blue', 'red', 'green'):
            return render(p(1))
        if low in ('isbn', 'issn', 'doi'):
            return low.upper() + ' ' + render(p(1))
        if low == 'rp':
            return '<span class="wiki-citation">（页 ' + render(p('page', p(1))) + '）</span>'
        if low == 'r':
            return ''.join(self.render('<ref name="' + html.escape(str(param.value).strip(), quote=True) + '"/>', depth) for param in node.params if str(param.name).strip().isdigit())
        if low == 'numblk':
            return '<div class="wiki-indented">' + render(p(2)) + (' <span>（' + render(p(3)) + '）</span>' if p(3) else '') + '</div>'
        if low == 'val':
            tex = p(1) + (' \\times 10^{' + p('e') + '}' if p('e') else '')
            return self.math(tex) + (' ' + render(p('u')) if p('u') else '')
        if low in ('lang', 'langx') or low.startswith('lang-'):
            return render(p(2) if low in ('lang', 'langx') else p(1))
        if low.startswith(('link-', 'tsl')) or low in ('ilh', 'ill', 'interlanguage link'):
            return render(p(3, p(1))) if low.startswith('link-') else render(p(1))
        if low in ('frac', 'sfrac'):
            values = [p(i) for i in range(1, 4) if p(i)]
            if len(values) >= 2:
                tex = (values[0] if len(values) == 3 else '') + r'\frac{' + values[-2] + '}{' + values[-1] + '}'
                return self.math(tex)
            return self.math(r'\frac{1}{' + p(1) + '}')
        if low in ('sqrt',):
            return self.math(r'\sqrt{' + p(1) + '}')
        if low in ('sup', 'sub'):
            return f'<{low}>' + render(p(1)) + f'</{low}>'
        if low in ('sfn', 'sfnp', 'harv', 'harvnb', 'harvtxt'):
            return '<span class="wiki-citation">（' + '，'.join(render(str(x.value)) for x in node.params) + '）</span>'
        if low.startswith(('cite ', 'citation', 'cite_')):
            title = p('title', p('chapter', p('urlname', name)))
            author = p('author', p('author1', ' '.join(filter(None, [p('first'), p('last')]))))
            url = p('url', ('https://arxiv.org/abs/' + p('eprint')) if p('eprint') else '')
            body = [render(author), self.link(url, render(title)) if url else render(title), render(p('journal', p('publisher'))), render(p('year', p('date'))), render(p('pages'))]
            return ' · '.join(x for x in body if x)
        if low in ('main', 'see also', 'further'):
            return '另见：' + '、'.join(self.link('https://zh.wikipedia.org/wiki/' + quote(str(x.value).strip()), render(str(x.value))) for x in node.params)
        if low in ('!', '!!', '='):
            return {'!': '|', '!!': '||', '=': '='}[low]
        if low == 'br':
            return '<br>'
        if low in ('nbsp', 'space'):
            return ' '
        # Preserve unsupported template source in a readable, explicit appendix.
        source = str(node)
        if source not in self.unsupported:
            self.unsupported[source] = name
        return '<span class="wiki-template-marker">〔模板待展开：' + self.text(name) + '〕</span>'

    def reference(self, node, depth):
        attrs = {str(a.name).lower(): str(a.value or '') for a in node.attributes}
        name = attrs.get('name')
        content = str(node.contents or '').strip()
        if name and name in self.named_refs:
            index = self.named_refs[name]
            if content:
                self.references[index - 1] = self.render(content, depth)
        else:
            index = len(self.references) + 1
            self.references.append(self.render(content, depth) if content else '原文未提供此引用的详细内容。')
            if name:
                self.named_refs[name] = index
        return f'<sup class="wiki-ref"><a href="#wiki-ref-{self.page_id}-{index}" aria-label="参考文献 {index}">[{index}]</a></sup>'

    def table(self, node, depth):
        self.table_count += 1
        rows, pending, captions = [], [], []
        for child in node.contents.nodes:
            tag = str(getattr(child, 'tag', '')).lower()
            if tag in ('td', 'th'):
                if str(child.contents).lstrip().startswith('+'):
                    captions.append(self.render(str(child.contents).lstrip()[1:], depth))
                else:
                    pending.append(self.node(child, depth))
            elif tag == 'tr':
                if pending:
                    rows.append('<tr>' + ''.join(pending) + '</tr>')
                    pending = []
                rows.append(self.node(child, depth))
            elif str(child).strip():
                captions.append(self.node(child, depth))
        if pending:
            rows.append('<tr>' + ''.join(pending) + '</tr>')
        caption = '<caption>' + ' '.join(captions) + '</caption>' if captions else ''
        return '<div class="wiki-table-scroll"><table>' + caption + '<tbody>' + ''.join(rows) + '</tbody></table></div>'

    def node(self, node, depth):
        if isinstance(node, nodes.Text):
            return self.text(node.value)
        if isinstance(node, nodes.Comment):
            return ''
        if isinstance(node, nodes.HTMLEntity):
            return html.escape(node.normalize())
        if isinstance(node, nodes.Template):
            return self.template(node, depth)
        if isinstance(node, nodes.Wikilink):
            title = str(node.title).strip()
            if re.match(r'^(?:Category|分类|分類|[a-z-]{2,12}):', title, re.I) and not re.match(r'^(?:File|Image):', title, re.I):
                return ''
            if re.match(r'^(?:File|Image|文件|檔案|档案|图像|圖像):', title, re.I):
                return self.image(title, node.text, depth)
            return self.link('https://zh.wikipedia.org/wiki/' + quote(title.lstrip(':').replace(' ', '_'), safe=''), self.render(node.text if node.text is not None else title.lstrip(':'), depth))
        if isinstance(node, nodes.ExternalLink):
            return self.link(str(node.url), self.render(node.title, depth) if node.title else self.text(node.url))
        if isinstance(node, nodes.Heading):
            return f'<h{node.level}>' + self.render(node.title, depth) + f'</h{node.level}>'
        if isinstance(node, nodes.Tag):
            tag = str(node.tag).strip().lower()
            attrs = {str(a.name).lower(): str(a.value or '') for a in node.attributes}
            if tag == 'math':
                return self.math(node.contents, attrs.get('display') == 'block')
            if tag == 'ref':
                return self.reference(node, depth)
            if tag in ('references', 'script', 'style', 'iframe', 'object'):
                return ''
            if tag == 'table':
                return self.table(node, depth)
            if tag == 'gallery':
                return ''.join(self.image(line.split('|', 1)[0], line.split('|', 1)[1] if '|' in line else '', depth) for line in str(node.contents).splitlines() if line.strip() and ':' in line)
            if node.wiki_markup in ('*', '#', ':', ';'):
                return str(node.wiki_markup)
            if tag in ('syntaxhighlight', 'source', 'pre'):
                return '<pre><code>' + html.escape(str(node.contents)) + '</code></pre>'
            if tag in ('nowiki', 'code'):
                return '<code>' + self.text(node.contents) + '</code>'
            if tag in ('br', 'hr'):
                return f'<{tag}>'
            if tag in ('b', 'strong', 'i', 'em', 'sup', 'sub', 's', 'u', 'span', 'div', 'p', 'blockquote', 'ul', 'ol', 'li', 'dl', 'dt', 'dd', 'tr', 'th', 'td', 'caption', 'small', 'center'):
                tag = 'div' if tag == 'center' else tag
                spans = ''.join(f' {a}="{attrs[a]}"' for a in ('colspan', 'rowspan') if tag in ('td', 'th') and attrs.get(a, '').isdigit() and 0 < int(attrs[a]) <= 100)
                return f'<{tag}{spans}>' + self.render(node.contents, depth) + f'</{tag}>'
            return self.render(node.contents or '', depth)
        return self.text(node)

    def blocks(self, raw):
        rendered = self.render(raw)
        # Protect complete block elements and multi-line math before splitting lines.
        blocks = []
        def protect(match):
            index = len(blocks)
            blocks.append(match[0])
            return f'\n\n@@WIKIBLOCK{index}@@\n\n'
        rendered = re.sub(r'<(figure|pre|div|table|blockquote)\b[^>]*>.*?</\1>', protect, rendered, flags=re.S)
        # Formula attributes can contain newlines; retain them as one inline token.
        maths = []
        def protect_math(match):
            maths.append(match[0])
            return f'@@WIKIMATH{len(maths)-1}@@'
        rendered = re.sub(r'<span class="wiki-math".*?</span>', protect_math, rendered, flags=re.S)
        output, paragraph, lists = [], [], []
        def flush():
            if paragraph:
                output.append('<p>' + ' '.join(paragraph) + '</p>')
                paragraph.clear()
        def close_lists():
            while lists:
                output.append('</li></' + lists.pop() + '>')
        for line in rendered.splitlines():
            line = line.strip()
            if not line:
                flush()
                close_lists()
                continue
            block = re.fullmatch(r'@@WIKIBLOCK(\d+)@@', line)
            if block:
                flush(); close_lists()
                output.append(blocks[int(block[1])])
                continue
            item = re.match(r'^([*#]+)\s*(.*)', line)
            if item:
                flush()
                wanted = ['ul' if x == '*' else 'ol' for x in item[1]]
                common = 0
                while common < min(len(lists), len(wanted)) and lists[common] == wanted[common]:
                    common += 1
                while len(lists) > common:
                    output.append('</li></' + lists.pop() + '>')
                if len(lists) == len(wanted):
                    output.append('</li><li>')
                while len(lists) < len(wanted):
                    lists.append(wanted[len(lists)])
                    output.append('<' + lists[-1] + '><li>')
                output.append(item[2])
            elif line.startswith(':'):
                flush(); close_lists()
                value = line.lstrip(':').strip()
                value = re.sub(r'@@WIKIMATH(\d+)@@', lambda m: maths[int(m[1])].replace('data-display="false"', 'data-display="true"'), value)
                output.append('<div class="wiki-indented">' + value + '</div>')
            elif line.startswith(';'):
                flush(); close_lists()
                output.append('<p><strong>' + line.lstrip(';') + '</strong></p>')
            else:
                close_lists()
                paragraph.append(line)
        flush(); close_lists()
        return re.sub(r'@@WIKIMATH(\d+)@@', lambda m: maths[int(m[1])], ''.join(output))


def render_article(article):
    renderer = Renderer(article['pageId'])
    raw = article['rawWikitext']
    headings = list(HEADINGS.finditer(raw))
    # Preserve the original heading positions: existing annotation section IDs survive.
    parts = [(0, '概述', 1, raw[:headings[0].start()] if headings else raw)]
    for i, heading in enumerate(headings):
        end = headings[i + 1].start() if i + 1 < len(headings) else len(raw)
        title = plain_html(renderer.render(variants(heading[2])))
        parts.append((i + 1, title, len(heading[1]), raw[heading.end():end]))
    sections = []
    for position, title, level, source in parts:
        content_html = renderer.blocks(source)
        content = plain_html(content_html)
        if not content and not re.search(r'<(?:img|table)\b', content_html):
            continue
        sections.append(dict(position=position, title=title, level=level, content=content, contentHtml=content_html))
    if renderer.references:
        refs = '<ol class="wiki-references">' + ''.join(f'<li id="wiki-ref-{article["pageId"]}-{i}">{value}</li>' for i, value in enumerate(renderer.references, 1)) + '</ol>'
        sections.append(dict(position=1000000, title='参考文献', level=2, content=plain_html(refs), contentHtml=refs))
    warnings = []
    if renderer.unsupported:
        warnings.append(f'{len(renderer.unsupported)} 个模板未在本地展开，原始内容保留在文末，可对照原词条。')
        templates = ''.join('<details class="wiki-template-source"><summary>' + renderer.text(name) + '（原始模板）</summary><pre>' + html.escape(source) + '</pre></details>' for source, name in renderer.unsupported.items())
        sections.append(dict(position=1000001, title='待解析模板', level=2, content='', contentHtml=templates))
    intro = next((s['content'] for s in sections if s['position'] == 0), '')
    # Only the list preview is shortened. The full introduction is a real section.
    preview_text = re.sub(r'〔模板待展开：[^〕]*〕', '', intro).strip()
    excerpt = preview_text.split('\n\n')[0] if preview_text else next((s['content'] for s in sections if s['content']), '')
    excerpt = re.sub(r'\s+', ' ', excerpt)
    excerpt = excerpt[:160] + ('…' if len(excerpt) > 160 else '')
    plain = '\n\n'.join(s['content'] for s in sections if s['position'] < 1000000)
    return dict(pageId=article['pageId'], title=article['title'], excerpt=excerpt, plainText=plain, sections=sections, warnings=warnings, contentVersion=2, metrics=dict(math=renderer.math_count, images=renderer.image_count, tables=renderer.table_count, unsupported=len(renderer.unsupported)))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--input', default='E:/MathWikiData/exports/core-articles.jsonl')
    parser.add_argument('--output', required=True)
    parser.add_argument('--limit', type=int, default=0)
    args = parser.parse_args()
    if Path(args.input).resolve() == Path(args.output).resolve():
        raise SystemExit('Refusing to overwrite the source export')
    metrics = Counter()
    with open(args.input, encoding='utf-8') as source, open(args.output, 'x', encoding='utf-8') as target:
        for index, line in enumerate(source, 1):
            result = render_article(json.loads(line))
            target.write(json.dumps(result, ensure_ascii=False) + '\n')
            metrics.update(result['metrics'])
            if index % 250 == 0:
                print(f'Rendered {index} articles', flush=True)
            if args.limit and index >= args.limit:
                break
    print(json.dumps(dict(articles=index, **metrics), ensure_ascii=False), flush=True)


if __name__ == '__main__':
    main()
