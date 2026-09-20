"""List original illustration files for a future, license-aware CDN import.

Does not download, upload or create cloud resources. The result is an inventory,
NOT a publishable media-manifest.json: author/license metadata still needs review.
"""
import argparse
import json
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlparse, quote


class Images(HTMLParser):
    def __init__(self):
        super().__init__()
        self.names = set()

    def handle_starttag(self, tag, attrs):
        if tag != 'img':
            return
        parsed = urlparse(dict(attrs).get('src', ''))
        prefix = '/wiki/Special:FilePath/'
        if parsed.scheme == 'https' and parsed.netloc == 'commons.wikimedia.org' and parsed.path.startswith(prefix):
            self.names.add(unquote(parsed.path[len(prefix):]).replace(' ', '_'))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    if Path(args.input).resolve() == Path(args.output).resolve():
        raise SystemExit('Input and output must differ')
    files = {}
    count = 0
    with open(args.input, encoding='utf-8') as source:
        for line in source:
            article = json.loads(line)
            images = Images()
            for section in article['sections']:
                images.feed(section['contentHtml'])
            for name in images.names:
                record = files.setdefault(name, dict(filename=name, sourceUrl='https://commons.wikimedia.org/wiki/File:' + quote(name, safe=''), status='needs-license-and-metadata-review', articles=[]))
                record['articles'].append(article['pageId'])
            count += 1
    result = dict(articles=count, uniqueFiles=len(files), downloaded=False, uploaded=False, files=sorted(files.values(), key=lambda f:f['filename']))
    with open(args.output, 'x', encoding='utf-8') as output:
        json.dump(result, output, ensure_ascii=False, indent=2)
    print(json.dumps({k:v for k,v in result.items() if k!='files'}, ensure_ascii=False))


if __name__ == '__main__':
    main()
