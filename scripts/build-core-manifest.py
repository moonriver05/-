"""从中文维基 SQL 元数据构建数学/算法核心教学条目清单。

只处理元数据，不下载或解析正文。中间 SQLite 和结果均写入数据盘。
"""

from __future__ import annotations

import argparse
import gzip
import json
import math
import sqlite3
import sys
import time
import urllib.parse
from collections import defaultdict, deque
from pathlib import Path
from typing import Iterator


def decode_sql_escape(char: str) -> str:
    return {"0": "\0", "b": "\b", "n": "\n", "r": "\r", "t": "\t", "Z": "\x1a"}.get(char, char)


def iter_insert_rows(path: Path, table: str) -> Iterator[list[str | None]]:
    prefix = f"INSERT INTO `{table}` VALUES "
    # SQL 中 varbinary 排序键并不保证是合法 UTF-8；这些字段不会进入结果。
    # 使用替换模式可保留行结构，同时避免无关排序键中断元数据盘点。
    with gzip.open(path, "rt", encoding="utf-8", errors="replace", newline="") as stream:
        for line in stream:
            if not line.startswith(prefix):
                continue
            payload = line[len(prefix):]
            row: list[str | None] = []
            token: list[str] = []
            in_row = False
            quoted = False
            escaped = False
            for char in payload:
                if not in_row:
                    if char == "(":
                        in_row = True
                        row, token = [], []
                    continue
                if quoted:
                    if escaped:
                        token.append(decode_sql_escape(char))
                        escaped = False
                    elif char == "\\":
                        escaped = True
                    elif char == "'":
                        quoted = False
                    else:
                        token.append(char)
                    continue
                if char == "'":
                    quoted = True
                elif char == ",":
                    raw = "".join(token)
                    row.append(None if raw == "NULL" else raw)
                    token = []
                elif char == ")":
                    raw = "".join(token)
                    row.append(None if raw == "NULL" else raw)
                    yield row
                    in_row = False
                elif char not in "\r\n":
                    token.append(char)


def batches(iterator, size: int):
    batch = []
    for value in iterator:
        batch.append(value)
        if len(batch) >= size:
            yield batch
            batch = []
    if batch:
        yield batch


def import_pages(db: sqlite3.Connection, path: Path) -> int:
    db.execute("DROP TABLE IF EXISTS pages")
    db.execute("CREATE TABLE pages(id INTEGER PRIMARY KEY, ns INTEGER, title TEXT, redirect INTEGER, revision INTEGER, bytes INTEGER)")
    total = 0
    rows = (
        (int(r[0]), int(r[1]), str(r[2]).replace("_", " "), int(r[3]), int(r[8]), int(r[9]))
        for r in iter_insert_rows(path, "page") if int(r[1]) in (0, 14)
    )
    for chunk in batches(rows, 20_000):
        db.executemany("INSERT INTO pages VALUES (?,?,?,?,?,?)", chunk)
        total += len(chunk)
        if total % 200_000 == 0:
            print(f"  页面记录 {total:,}", flush=True)
        db.commit()
    db.execute("CREATE INDEX pages_ns_title ON pages(ns,title)")
    db.commit()
    return total


def import_targets(db: sqlite3.Connection, path: Path) -> int:
    db.execute("DROP TABLE IF EXISTS targets")
    db.execute("CREATE TABLE targets(id INTEGER PRIMARY KEY, title TEXT UNIQUE)")
    total = 0
    rows = (
        (int(r[0]), str(r[2]).replace("_", " "))
        for r in iter_insert_rows(path, "linktarget") if int(r[1]) == 14
    )
    for chunk in batches(rows, 20_000):
        db.executemany("INSERT OR IGNORE INTO targets VALUES (?,?)", chunk)
        total += len(chunk)
        db.commit()
    db.execute("CREATE INDEX targets_title ON targets(title)")
    db.commit()
    return total


def import_links(db: sqlite3.Connection, path: Path) -> int:
    db.execute("DROP TABLE IF EXISTS links")
    db.execute("CREATE TABLE links(source INTEGER, target INTEGER, kind TEXT)")
    total = 0
    valid_targets = {row[0] for row in db.execute("SELECT id FROM targets")}
    rows = (
        (int(r[0]), int(r[6]), str(r[4]))
        for r in iter_insert_rows(path, "categorylinks")
        if str(r[4]) in ("page", "subcat") and int(r[6]) in valid_targets
    )
    for chunk in batches(rows, 30_000):
        db.executemany("INSERT INTO links VALUES (?,?,?)", chunk)
        total += len(chunk)
        if total % 500_000 == 0:
            print(f"  分类关系 {total:,}", flush=True)
        db.commit()
    db.execute("CREATE INDEX links_target ON links(target)")
    db.commit()
    return total


def ensure_metadata(db: sqlite3.Connection, dump_dir: Path, rebuild: bool) -> dict[str, int]:
    expected = {
        "pages": dump_dir / "zhwiki-latest-page.sql.gz",
        "targets": dump_dir / "zhwiki-latest-linktarget.sql.gz",
        "links": dump_dir / "zhwiki-latest-categorylinks.sql.gz",
    }
    for path in expected.values():
        if not path.exists():
            raise FileNotFoundError(f"缺少中文维基元数据：{path}")

    counts: dict[str, int] = {}
    existing = {row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='table'")}
    indexes = {row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='index'")}
    if rebuild or "pages" not in existing:
        print("导入中文页面表…", flush=True)
        counts["pages"] = import_pages(db, expected["pages"])
    else:
        counts["pages"] = db.execute("SELECT count(*) FROM pages").fetchone()[0]
    if rebuild or "targets" not in existing:
        print("导入中文分类目标表…", flush=True)
        counts["targets"] = import_targets(db, expected["targets"])
    else:
        counts["targets"] = db.execute("SELECT count(*) FROM targets").fetchone()[0]
    # links_target 只在完整导入后创建，可可靠识别被中断的半成品表。
    if rebuild or "links" not in existing or "links_target" not in indexes:
        print("导入中文分类关系表…", flush=True)
        counts["links"] = import_links(db, expected["links"])
    else:
        counts["links"] = db.execute("SELECT count(*) FROM links").fetchone()[0]
    return counts


def excluded(title: str, patterns: list[str]) -> bool:
    return any(pattern in title for pattern in patterns)


def build_manifest(db: sqlite3.Connection, config: dict, output_dir: Path) -> dict:
    traversal = config["categoryTraversal"]
    rules = config["contentRules"]
    priority = {title.replace("_", " ") for title in config["priorityTitles"]}
    patterns = traversal["excludeCategoryPatterns"]
    roots = list(dict.fromkeys(traversal["roots"] + traversal["algorithmRoots"]))
    max_depth = int(traversal["maxDepth"])

    target_by_title = {title: target_id for target_id, title in db.execute("SELECT id,title FROM targets")}
    missing_roots = [root for root in roots if root not in target_by_title]
    queue = deque((root, target_by_title[root], 0, root) for root in roots if root in target_by_title)
    visited_targets: set[int] = set()
    category_depths: dict[int, int] = {}
    article_paths: dict[int, set[str]] = defaultdict(set)
    article_depth: dict[int, int] = {}

    while queue:
        category, target_id, depth, root = queue.popleft()
        if target_id in visited_targets:
            continue
        visited_targets.add(target_id)
        category_depths[target_id] = depth
        members = db.execute(
            "SELECT p.id,p.ns,p.title,p.redirect,p.bytes,t.id "
            "FROM links l JOIN pages p ON p.id=l.source "
            "LEFT JOIN targets t ON t.title=p.title "
            "WHERE l.target=?", (target_id,)
        )
        for page_id, ns, title, redirect, byte_len, child_target in members:
            if ns == 0 and not redirect and byte_len >= int(rules["minimumBytes"]):
                article_paths[page_id].add(root)
                article_depth[page_id] = min(depth, article_depth.get(page_id, depth))
            elif ns == 14 and depth < max_depth and child_target and not excluded(title, patterns):
                queue.append((title, child_target, depth + 1, root))

    records = []
    for page_id, roots_for_page in article_paths.items():
        title, revision, byte_len = db.execute(
            "SELECT title,revision,bytes FROM pages WHERE id=?", (page_id,)
        ).fetchone()
        depth = article_depth[page_id]
        score = 100 if title in priority else 0
        score += max(0, 42 - depth * 9)
        score += min(20, round(math.log2(max(byte_len, 1))))
        score += min(12, (len(roots_for_page) - 1) * 3)
        records.append({
            "pageId": page_id,
            "revisionId": revision,
            "title": title,
            "byteLength": byte_len,
            "depth": depth,
            "roots": sorted(roots_for_page),
            "priority": title in priority,
            "score": score,
            "sourceUrl": "https://zh.wikipedia.org/wiki/" + urllib.parse.quote(title.replace(" ", "_")),
            "language": "zh-Hans",
        })

    records.sort(key=lambda r: (-r["priority"], -r["score"], r["title"]))
    maximum = int(config["targetArticles"]["maximum"])
    selected = records[:maximum]
    output_dir.mkdir(parents=True, exist_ok=True)
    manifest_path = output_dir / "core-candidates.jsonl"
    with manifest_path.open("w", encoding="utf-8", newline="\n") as stream:
        for record in selected:
            stream.write(json.dumps(record, ensure_ascii=False) + "\n")

    summary = {
        "edition": config["edition"],
        "language": config["language"],
        "generatedAt": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "rootCategories": len(roots),
        "missingRoots": missing_roots,
        "categoriesVisited": len(visited_targets),
        "candidateArticlesBeforeCap": len(records),
        "candidatesSelectedForContentReview": len(selected),
        "priorityArticlesFound": sum(1 for r in records if r["priority"]),
        "selectedSourceBytes": sum(r["byteLength"] for r in selected),
        "depthDistribution": dict(sorted({d: sum(1 for r in selected if r["depth"] == d) for d in set(r["depth"] for r in selected)}.items())),
        "manifest": str(manifest_path),
    }
    (output_dir / "core-summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    return summary


def main() -> int:
    parser = argparse.ArgumentParser(description="盘点中文维基数学与算法核心教学条目")
    parser.add_argument("--data-root", default=r"E:\MathWikiData")
    parser.add_argument("--config", default=str(Path(__file__).parents[1] / "config" / "core-curriculum.json"))
    parser.add_argument("--rebuild", action="store_true")
    args = parser.parse_args()

    data_root = Path(args.data_root)
    config = json.loads(Path(args.config).read_text(encoding="utf-8"))
    if config.get("edition") != "zhwiki" or config.get("language") != "zh-Hans":
        raise ValueError("核心教学库必须使用中文维基 zhwiki，并输出 zh-Hans 简体中文")
    db_path = data_root / "exports" / "wiki-metadata.sqlite3"
    db_path.parent.mkdir(parents=True, exist_ok=True)
    db = sqlite3.connect(db_path)
    db.execute("PRAGMA journal_mode=WAL")
    db.execute("PRAGMA synchronous=NORMAL")
    db.execute("PRAGMA temp_store=MEMORY")
    try:
        counts = ensure_metadata(db, data_root / "dumps", args.rebuild)
        summary = build_manifest(db, config, data_root / "exports")
        summary["metadataRows"] = counts
        print(json.dumps(summary, ensure_ascii=False, indent=2))
    finally:
        db.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
