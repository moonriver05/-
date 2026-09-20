"""通过 multistream 索引只解压候选中文页面，并生成最终核心教学库。"""

from __future__ import annotations

import argparse
import bz2
import json
import math
import re
import sys
import time
import xml.etree.ElementTree as ET
from collections import Counter, defaultdict
from pathlib import Path

from render_wiki import render_article, variants

try:
    import mwparserfromhell
    from opencc import OpenCC
except ImportError as error:
    raise SystemExit("缺少解析依赖，请先执行: pip install -r scripts/requirements.txt") from error


PAGE_RE = re.compile(r"<page>.*?</page>", re.DOTALL)
HEADING_RE = re.compile(r"^(={2,6})\s*(.*?)\s*\1\s*$", re.MULTILINE)
CATEGORY_RE = re.compile(r"\[\[(?:Category|分类|分類):([^\]|]+)", re.IGNORECASE)
CHINESE_RE = re.compile(r"[\u3400-\u9fff]")
VARIANT_RE = re.compile(r"-\{(.*?)-\}", re.DOTALL)
NOISE_TITLE_RE = re.compile(
    r"(?:列表|名录|名單|年表|时间线)$|^(?:\d+|\d{3,4}年)$|"
    r"(?:县|縣|区|區|市|乡|鄉|镇|鎮|村|省)各级文物|"
    r"(?:行政区|行政區|撞击坑|撞擊坑|古塔|车站|車站|机场|機場)列表"
)
NOISE_CATEGORY_PATTERNS = (
    "数学家", "數學家", "统计学家", "統計學家", "逻辑学家", "邏輯學家",
    "人物", "出生", "逝世", "校友", "教师", "教授", "奖项", "獎項",
    "文物", "行政区", "行政區", "各国", "各國", "各世纪", "各世紀",
    "期刊", "学校", "學校", "大学", "大學", "公司", "软件公司", "軟體公司",
    "维护", "維護", "需要", "维基百科", "維基百科",
)
TEACHING_TERMS = (
    "定义", "定義", "定理", "证明", "證明", "算法", "演算法", "复杂度", "複雜度",
    "公式", "方程", "函数", "函數", "矩阵", "矩陣", "概率", "機率", "统计", "統計",
    "集合", "逻辑", "邏輯", "图论", "圖論", "数据结构", "資料結構", "计算", "計算",
    "递归", "遞迴", "排序", "搜索", "搜尋", "优化", "最佳化", "几何", "幾何",
)


def local_name(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def child_text(element: ET.Element, name: str) -> str:
    for child in element.iter():
        if local_name(child.tag) == name:
            return child.text or ""
    return ""


def first_child(element: ET.Element, name: str) -> ET.Element | None:
    for child in element:
        if local_name(child.tag) == name:
            return child
    return None


def load_candidate_ids(path: Path) -> tuple[set[int], dict[int, dict]]:
    metadata = {}
    with path.open(encoding="utf-8") as stream:
        for line in stream:
            record = json.loads(line)
            metadata[int(record["pageId"])] = record
    return set(metadata), metadata


def read_index(index_path: Path, wanted: set[int]):
    wanted_offsets: dict[int, set[int]] = defaultdict(set)
    all_offsets: list[int] = []
    previous = -1
    with bz2.open(index_path, "rt", encoding="utf-8", errors="replace") as stream:
        for line in stream:
            try:
                offset_text, page_id_text, _title = line.rstrip("\n").split(":", 2)
                offset, page_id = int(offset_text), int(page_id_text)
            except ValueError:
                continue
            if offset != previous:
                all_offsets.append(offset)
                previous = offset
            if page_id in wanted:
                wanted_offsets[offset].add(page_id)
    next_offset = {offset: all_offsets[i + 1] for i, offset in enumerate(all_offsets[:-1])}
    return wanted_offsets, next_offset


def resolve_variant_markup(raw: str) -> str:
    return variants(raw)


def clean_wikitext(raw: str, converter: OpenCC, page_id: int = 0) -> tuple[str, list[dict], list[str]]:
    categories = [converter.convert(c.replace("_", " ").strip()) for c in CATEGORY_RE.findall(raw)]
    rendered = render_article({"pageId": page_id, "title": "", "rawWikitext": raw})
    return rendered["plainText"], rendered["sections"], categories


def teaching_score(record: dict, title: str, plain: str, sections: list[dict], categories: list[str]) -> tuple[int, list[str]]:
    reasons = []
    score = int(record.get("score", 0))
    if record.get("priority"):
        score += 160
    chinese_chars = len(CHINESE_RE.findall(plain))
    score += min(35, int(math.log2(max(chinese_chars, 1)) * 4))
    term_hits = sum(1 for term in TEACHING_TERMS if term in title or term in plain[:1800])
    score += min(45, term_hits * 5)
    useful_sections = sum(1 for section in sections if any(term in section["title"] for term in TEACHING_TERMS))
    score += min(24, useful_sections * 4)
    if "<math" in record.get("raw", ""):
        score += 4
    if chinese_chars < 250:
        reasons.append("正文中文字符不足")
    if NOISE_TITLE_RE.search(title):
        reasons.append("标题属于列表、年份或地区资料")
    bad_categories = [c for c in categories if any(pattern in c for pattern in NOISE_CATEGORY_PATTERNS)]
    if bad_categories:
        reasons.append("命中非教学分类：" + "、".join(bad_categories[:3]))
    if term_hits == 0 and not record.get("priority"):
        reasons.append("导语未出现数学或算法教学特征")
    return score, reasons


def extract(args) -> dict:
    data_root = Path(args.data_root)
    dump_dir = data_root / "dumps"
    export_dir = data_root / "exports"
    manifest = export_dir / "core-candidates.jsonl"
    dump_path = dump_dir / "zhwiki-latest-pages-articles-multistream.xml.bz2"
    index_path = dump_dir / "zhwiki-latest-pages-articles-multistream-index.txt.bz2"
    for path in (manifest, dump_path, index_path):
        if not path.exists():
            raise FileNotFoundError(path)

    wanted, metadata = load_candidate_ids(manifest)
    offsets, next_offsets = read_index(index_path, wanted)
    converter = OpenCC("t2s")
    accepted, rejected = [], Counter()
    found: set[int] = set()

    print(f"候选 {len(wanted):,} 篇，涉及 {len(offsets):,} 个压缩数据块", flush=True)
    with dump_path.open("rb") as binary:
        for block_no, offset in enumerate(sorted(offsets), 1):
            end = next_offsets.get(offset, dump_path.stat().st_size)
            binary.seek(offset)
            xml_text = bz2.decompress(binary.read(end - offset)).decode("utf-8", errors="replace")
            for fragment in PAGE_RE.findall(xml_text):
                try:
                    page = ET.fromstring(fragment)
                except ET.ParseError:
                    continue
                page_id_text = child_text(page, "id")
                if not page_id_text or int(page_id_text) not in wanted:
                    continue
                page_id = int(page_id_text)
                found.add(page_id)
                source = metadata[page_id]
                revision = first_child(page, "revision")
                if revision is None:
                    continue
                raw = child_text(revision, "text")
                title = converter.convert(child_text(page, "title"))
                plain, sections, categories = clean_wikitext(raw, converter, page_id)
                source_with_raw = {**source, "raw": raw}
                score, reasons = teaching_score(source_with_raw, title, plain, sections, categories)
                if reasons and not source.get("priority"):
                    for reason in reasons:
                        rejected[reason.split("：", 1)[0]] += 1
                    continue
                accepted.append({
                    **source,
                    "title": title,
                    "sourceTitle": child_text(page, "title"),
                    "revisionId": int(child_text(revision, "id") or source["revisionId"]),
                    "timestamp": child_text(revision, "timestamp"),
                    "language": "zh-Hans",
                    "plainText": plain,
                    "rawWikitext": raw,
                    "sections": sections,
                    "contentVersion": 2,
                    "contentWarnings": ["部分模板未在本地展开，完整原文保留在文末。"] if any(s["position"] == 1000001 for s in sections) else [],
                    "categories": categories,
                    "teachingScore": score,
                    "license": "CC BY-SA 4.0",
                })
            if block_no % 250 == 0:
                print(f"  已处理数据块 {block_no:,}/{len(offsets):,}，暂留 {len(accepted):,} 篇", flush=True)

    accepted.sort(key=lambda r: (-r["priority"], -r["teachingScore"], r["title"]))
    final = accepted[: args.limit]
    output_path = export_dir / "core-articles.jsonl"
    with output_path.open("w", encoding="utf-8", newline="\n") as stream:
        for article in final:
            stream.write(json.dumps(article, ensure_ascii=False) + "\n")

    summary = {
        "edition": "zhwiki",
        "language": "zh-Hans",
        "generatedAt": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "candidates": len(wanted),
        "pagesFound": len(found),
        "acceptedBeforeCap": len(accepted),
        "finalArticles": len(final),
        "rejected": rejected,
        "finalPlainTextBytes": sum(len(r["plainText"].encode("utf-8")) for r in final),
        "output": str(output_path),
    }
    (export_dir / "content-summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    return summary


def main() -> int:
    parser = argparse.ArgumentParser(description="提取简体中文数学与算法核心教学正文")
    parser.add_argument("--data-root", default=r"E:\MathWikiData")
    parser.add_argument("--limit", type=int, default=5000)
    args = parser.parse_args()
    print(json.dumps(extract(args), ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
