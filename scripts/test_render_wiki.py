import json
import unittest
from pathlib import Path

from render_wiki import Renderer, render_article, variants


def article(raw):
    return render_article(dict(pageId=123, title='测试', rawWikitext=raw))


class RenderingTest(unittest.TestCase):
    def test_full_intro_and_formula_survive(self):
        result = article('导言。' * 100 + '\n\n:<math>\\frac{x^2}{2}</math>\n==定义==\n下一段。')
        intro = result['sections'][0]
        self.assertEqual(intro['position'], 0)
        self.assertIn('导言。' * 100, intro['content'])
        self.assertIn('data-tex="\\frac{x^2}{2}"', intro['contentHtml'])
        self.assertIn('data-display="true"', intro['contentHtml'])
        self.assertLess(len(result['excerpt']), len(intro['content']))

    def test_variant_and_language_templates(self):
        self.assertEqual(variants('-{zh-tw:約當;zh-hans:若尔当;}-'), '若尔当')
        result = article('-{zh-tw:高斯約當;zh-hans:高斯若尔当;}-（{{langx|en|Gauss-Jordan}}）。')
        self.assertIn('高斯若尔当（Gauss-Jordan）', result['plainText'])
        self.assertNotIn('zh-hans:', result['plainText'])

    def test_image_metadata_and_category_are_not_prose(self):
        result = article('[[File:X.png|thumb|250px|图注]]\n\n正文。\n[[Category:数学]][[de:Test]]')
        self.assertEqual(result['plainText'], '正文。')
        markup = result['sections'][0]['contentHtml']
        self.assertIn('<figure>', markup)
        self.assertNotIn('250px', markup)
        self.assertNotIn('Category:', markup)

    def test_tables_and_nested_lists(self):
        result = article('{| class="wikitable"\n|+ 标题\n! 甲 !! 乙\n|-\n| 一 || 二\n|}\n\n* 第一\n** 子项\n* 第二')
        markup = result['sections'][0]['contentHtml']
        for token in ('<table>', '<caption>', '<th>', '<td>', '<ul><li>'):
            self.assertIn(token, markup)
        self.assertNotIn('@@WIKI', markup)

    def test_references_not_merged_into_intro(self):
        result = article('定义<ref name="a">英文文献</ref>。<ref name="a"/>\n==参考==\n{{reflist}}')
        self.assertEqual(result['sections'][0]['content'], '定义。')
        refs = next(x for x in result['sections'] if x['position'] == 1000000)
        self.assertEqual(refs['contentHtml'].count('<li '), 1)
        self.assertNotIn('英文文献', result['excerpt'])

    def test_scientific_superscripts_are_kept_in_search_text(self):
        result = article('有 2<sup>16</sup> 个值<ref>文献</ref>。')
        self.assertIn('216', result['plainText'])
        self.assertNotIn('文献', result['plainText'])

    def test_template_notice_does_not_replace_list_preview(self):
        result = article('{{Infobox example|a=b}}\n\n这才是定义。')
        self.assertEqual(result['excerpt'], '这才是定义。')

    def test_unsupported_template_is_not_silently_lost(self):
        result = article('定义 {{Unknown scientific formula|x=y}}')
        self.assertTrue(result['warnings'])
        self.assertIn('x=y', result['sections'][-1]['contentHtml'])

    def test_untrusted_markup_is_not_executable(self):
        result = article('<script>alert(1)</script><div onclick="bad()">安全</div> [javascript:alert(1) 恶意链接] <math>\\href{javascript:alert(1)}{x}</math>')
        markup = result['sections'][0]['contentHtml']
        self.assertNotIn('<script', markup)
        self.assertNotIn('onclick=', markup)
        self.assertNotIn('href="javascript:', markup)

    def test_regressions_from_user_screenshots(self):
        path = Path('E:/MathWikiData/exports/core-articles.jsonl')
        if not path.exists():
            self.skipTest('Local source corpus unavailable')
        found = {}
        with path.open(encoding='utf-8') as stream:
            for line in stream:
                source = json.loads(line)
                if source['pageId'] in (1529742, 3501342, 523324):
                    found[source['pageId']] = render_article(source)
        self.assertEqual(len(found), 3)
        self.assertIn('导体', found[1529742]['plainText'])
        self.assertIn('data-tex=', found[3501342]['sections'][0]['contentHtml'])
        self.assertGreaterEqual(found[3501342]['metrics']['math'], 10)
        self.assertNotIn('zh-hans:', found[523324]['plainText'])
        self.assertIn('Gauss-Jordan Elimination', found[523324]['plainText'])


if __name__ == '__main__':
    unittest.main()
