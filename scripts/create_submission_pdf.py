"""Render the reviewed Markdown report as a paginated, embedded-font PDF.

No network, cloud credentials, or production database are used. Diagrams are
drawn as vectors; user-provided screenshots are placed without modification.
"""
import argparse
import html
import re
import shutil
from pathlib import Path

from pypdf import PdfReader
from reportlab.lib import colors
from reportlab.lib.enums import TA_LEFT
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, PageBreak, Table, TableStyle,
    Image, Flowable, KeepTogether,
)

ROOT = Path(__file__).resolve().parents[1]
INK = colors.HexColor('#142b3a')
TEAL = colors.HexColor('#087e8b')
LIGHT = colors.HexColor('#eef5f6')


class Diagram(Flowable):
    def __init__(self, kind):
        super().__init__()
        self.kind, self.width, self.height = kind, 487, 170

    def draw(self):
        c = self.canv

        def box(x, y, w, title, subtitle='', accent=False):
            c.setFillColor(TEAL if accent else LIGHT)
            c.setStrokeColor(TEAL)
            c.roundRect(x, y, w, 42, 5, fill=1, stroke=1)
            c.setFillColor(colors.white if accent else INK)
            c.setFont('CJK', 10)
            c.drawCentredString(x+w/2, y+25, title)
            c.setFont('CJK', 8)
            c.drawCentredString(x+w/2, y+10, subtitle)

        def arrow(x1, y1, x2, y2):
            c.setStrokeColor(TEAL)
            c.setLineWidth(1.1)
            c.line(x1, y1, x2, y2)
            if x1 == x2:
                c.line(x2, y2, x2-3, y2+5)
                c.line(x2, y2, x2+3, y2+5)
            else:
                c.line(x2, y2, x2-5, y2-3)
                c.line(x2, y2, x2-5, y2+3)

        if self.kind == 'workspace':
            for x,w,title,sub in [(0,100,'词条列表','搜索 · 筛选'),(112,218,'正文阅读','章节 · 公式 · 来源'),(342,138,'AI / 批注','程度 · 讲法 · 追问')]:
                box(x,112,w,title,sub,accent=x==342)
            c.setFillColor(LIGHT)
            for x,w in [(0,100),(112,218),(342,138)]:
                c.roundRect(x,18,w,82,4,fill=1,stroke=0)
            c.setFillColor(INK); c.setFont('CJK',9)
            for x,y,text in [(10,78,'分类与标题'),(10,55,'点击才切换'),(124,78,'原文内容与章节锚点'),(124,55,'选中句子后讨论'),(352,78,'流式解释与追问'),(352,55,'依据与原文跳转')]:
                c.drawString(x,y,text)
        elif self.kind == 'architecture':
            box(0, 120, 138, 'Vue / H5', '阅读 · 追问 · 批注')
            box(171, 120, 138, 'Nginx + Go API', '权限 · 编排 · SSE', True)
            box(342, 120, 138, 'DeepSeek 云 API', '初稿 · 模型自检')
            arrow(138, 141, 171, 141)
            arrow(309, 141, 342, 141)
            for x, title, sub in [(0,'MySQL','原文 · 账户 · 业务'),(171,'Chroma','1024 维向量 · 定位'),(342,'豆包 Embedding','云端文本向量化')]:
                box(x, 28, 138, title, sub)
                arrow(240, 120, x+69, 70) if x == 171 else None
            c.setStrokeColor(TEAL)
            c.lines([(69,70,69,91),(69,91,411,91),(411,91,411,70),(240,91,240,120)])
        else:
            items = [('本轮问题','有限追问历史'),('云端向量','1024 维'),('向量候选','当前词条 + 全库'),('原文回查','状态 + 内容摘要')]
            for i,(title,sub) in enumerate(items):
                x = i*123
                box(x,112,111,title,sub,accent=i==3)
                if i<3: arrow(x+111,133,x+123,133)
            items = [('证据片段','去重 · 最多 5 段'),('流式初稿','教学要求 + 对话'),('模型自检','结构化答案'),('引文校验','可信定位 · 原文跳转')]
            for i,(title,sub) in enumerate(items):
                x = i*123
                box(x,25,111,title,sub,accent=i==3)
                if i<3: arrow(x+111,46,x+123,46)
            c.setStrokeColor(TEAL)
            c.lines([(424,112,424,94),(424,94,55,94),(55,94,55,67)])


def generate(font, bold):
    pdfmetrics.registerFont(TTFont('CJK',str(font),subfontIndex=0))
    pdfmetrics.registerFont(TTFont('CJKBold',str(bold),subfontIndex=0))
    pdfmetrics.registerFontFamily('CJK',normal='CJK',bold='CJKBold',italic='CJK',boldItalic='CJKBold')
    styles = {
        'body':ParagraphStyle('body',fontName='CJK',fontSize=10.6,leading=18,spaceAfter=10,wordWrap='CJK',textColor=INK),
        'h1':ParagraphStyle('h1',fontName='CJKBold',fontSize=22,leading=31,spaceAfter=22,textColor=INK,wordWrap='CJK'),
        'h2':ParagraphStyle('h2',fontName='CJKBold',fontSize=13,leading=21,spaceBefore=9,spaceAfter=9,textColor=TEAL,wordWrap='CJK'),
        'table':ParagraphStyle('table',fontName='CJK',fontSize=9,leading=14,wordWrap='CJK',textColor=INK),
        'caption':ParagraphStyle('caption',fontName='CJK',fontSize=9,leading=15,spaceAfter=12,textColor=colors.HexColor('#526c77'),wordWrap='CJK'),
    }

    def inline(text):
        text = text.replace('\u2011','-').replace('\u2013','-').replace('\u2014','-')
        return re.sub(r'\*\*(.+?)\*\*',r'<b>\1</b>',html.escape(text))

    source = ROOT/'docs/submission/technical-report.md'
    lines = source.read_text(encoding='utf-8-sig').splitlines()
    story=[]
    i=0
    while i<len(lines):
        line=lines[i].strip()
        if not line: i+=1; continue
        if line=='---page---': story.append(PageBreak()); i+=1; continue
        if line.startswith('[[architecture]]') or line.startswith('[[rag-flow]]'):
            story.extend([Diagram('architecture' if 'architecture' in line else 'rag'),Spacer(1,14)])
            i+=1; continue
        if line=='[[workspace]]':
            story.extend([Diagram('workspace'),Spacer(1,10)])
            i+=1; continue
        if line.startswith('|'):
            rows=[]
            while i<len(lines) and lines[i].strip().startswith('|'):
                cells=[c.strip() for c in lines[i].strip().strip('|').split('|')]
                if not all(re.fullmatch(r'[:\- ]+',c) for c in cells):
                    rows.append([Paragraph(inline(c),styles['table']) for c in cells])
                i+=1
            widths=[487/len(rows[0])]*len(rows[0])
            table=Table(rows,colWidths=widths,repeatRows=1,hAlign='LEFT')
            table.setStyle(TableStyle([
                ('BACKGROUND',(0,0),(-1,0),LIGHT),('VALIGN',(0,0),(-1,-1),'TOP'),
                ('LINEBELOW',(0,0),(-1,0),1,TEAL),
                ('LINEBELOW',(0,1),(-1,-1),.35,colors.HexColor('#d2dfe3')),
                ('LEFTPADDING',(0,0),(-1,-1),8),('RIGHTPADDING',(0,0),(-1,-1),8),
                ('TOPPADDING',(0,0),(-1,-1),8),('BOTTOMPADDING',(0,0),(-1,-1),8),
            ]))
            story.extend([table,Spacer(1,15)]); continue
        if line.startswith('# '): style='h1'; text=line[2:]
        elif line.startswith('## '): style='h2'; text=line[3:]
        else: style='caption' if line.startswith('图：') else 'body'; text=line
        if text.startswith('- '): text='• '+text[2:]
        story.append(Paragraph(inline(text),styles[style]))
        i+=1

    output=ROOT/'output/pdf/MathWiki-technical-report.pdf'
    output.parent.mkdir(parents=True,exist_ok=True)
    def decorate(canvas,doc):
        canvas.saveState()
        canvas.setStrokeColor(TEAL); canvas.setLineWidth(2)
        canvas.line(54,801,541,801)
        canvas.setFont('CJK',8); canvas.setFillColor(TEAL)
        canvas.drawString(54,815,'MathWiki Lab   /   技术文档   /   2026.10')
        canvas.setFillColor(colors.HexColor('#526c77'))
        canvas.drawString(54,31,'陈子扬 · 郑竣杰   |   数学与算法交互式维基论坛')
        canvas.drawRightString(541,31,str(doc.page))
        canvas.restoreState()
    doc=SimpleDocTemplate(str(output),pagesize=A4,leftMargin=54,rightMargin=54,topMargin=65,bottomMargin=54,
        title='MathWiki Lab 技术文档',author='陈子扬、郑竣杰',pageCompression=1)
    doc.build(story,onFirstPage=decorate,onLaterPages=decorate)
    reader=PdfReader(output)
    if len(reader.pages)>30: raise RuntimeError('Report exceeds 30 pages')
    if any(not page.extract_text().strip() for page in reader.pages): raise RuntimeError('Empty PDF page')
    target=ROOT/'docs/submission/technical-report.pdf'
    shutil.copyfile(output,target)
    print(f'PDF verified: {len(reader.pages)} pages; {output.stat().st_size} bytes')
    return output


if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('--font',type=Path,default=Path('C:/Windows/Fonts/msyh.ttc'))
    parser.add_argument('--bold-font',type=Path)
    args=parser.parse_args()
    bold=args.bold_font or (Path('C:/Windows/Fonts/msyhbd.ttc') if args.font==Path('C:/Windows/Fonts/msyh.ttc') else args.font)
    if not args.font.is_file() or not bold.is_file(): parser.error('Provide a Chinese TrueType font using --font / --bold-font')
    generate(args.font,bold)
