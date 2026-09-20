import bz2
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace


MODULE_PATH = Path(__file__).with_name("extract-core-content.py")
SPEC = importlib.util.spec_from_file_location("extract_core_content", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
SPEC.loader.exec_module(MODULE)


class ExtractionTest(unittest.TestCase):
    def test_variant_prefers_simplified_chinese(self):
        source = "矩陣又称- {bad}-和-{zh-hant:線性代數;zh-hans:线性代数;}-"
        self.assertIn("线性代数", MODULE.resolve_variant_markup(source))

    def test_extracts_one_indexed_page(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "dumps").mkdir()
            (root / "exports").mkdir()
            candidate = {
                "pageId": 123, "revisionId": 456, "title": "線性代數", "sourceTitle": "線性代數",
                "sourceUrl": "https://zh.wikipedia.org/wiki/test", "roots": ["線性代數"],
                "priority": True, "score": 100, "language": "zh-Hans"
            }
            (root / "exports" / "core-candidates.jsonl").write_text(json.dumps(candidate, ensure_ascii=False) + "\n", encoding="utf-8")
            xml = """<page><title>線性代數</title><ns>0</ns><id>123</id><revision><id>456</id><timestamp>2026-09-01T00:00:00Z</timestamp><text>線性代數研究向量空间。\n== 定義 ==\n矩陣是重要工具。\n[[Category:線性代數]]</text></revision></page>"""
            (root / "dumps" / "zhwiki-latest-pages-articles-multistream.xml.bz2").write_bytes(bz2.compress(xml.encode()))
            (root / "dumps" / "zhwiki-latest-pages-articles-multistream-index.txt.bz2").write_bytes(bz2.compress("0:123:線性代數\n".encode()))
            summary = MODULE.extract(SimpleNamespace(data_root=str(root), limit=5))
            self.assertEqual(summary["finalArticles"], 1)
            result = json.loads((root / "exports" / "core-articles.jsonl").read_text(encoding="utf-8"))
            self.assertEqual(result["title"], "线性代数")
            self.assertEqual(result["revisionId"], 456)
            self.assertEqual(result["language"], "zh-Hans")


if __name__ == "__main__":
    unittest.main()

