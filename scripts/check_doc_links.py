#!/usr/bin/env python3
"""Check local Markdown navigation without running product or model tests."""

import re
import sys
from pathlib import Path
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parents[1]
LINK = re.compile(r'!?\[[^\]\n]*\]\((<[^>\n]+>|[^\s)]+)(?:\s+"[^"\n]*")?\)')


def prose(text):
    text = re.sub(r'^(`{3,}|~{3,})[^\n]*\n.*?^\1[^\n]*$', '', text, flags=re.M | re.S)
    return re.sub(r'(`+).*?\1', '', text, flags=re.S)


def main():
    errors = []
    files = [
        ROOT / 'README.md',
        ROOT / 'AGENTS.md',
        ROOT / 'web/README.md',
        *sorted((ROOT / 'docs').rglob('*.md')),
        *sorted((ROOT / 'SteerlineFlow').rglob('*.md')),
        *sorted((ROOT / 'SteerlineProjectGuideDoc').rglob('*.md')),
        *sorted((ROOT / 'tests/e2e').rglob('*.md')),
        *sorted((ROOT / 'debug-template').rglob('*.md')),
        *sorted((ROOT / '.agents').rglob('*.md')),
    ]
    links = 0
    for path in files:
        text = prose(path.read_text(encoding='utf-8-sig'))
        for match in LINK.finditer(text):
            href = match[1].strip('<>')
            if href.startswith('#') or re.match(r'^[a-zA-Z][a-zA-Z0-9+.-]*:', href):
                continue
            target = unquote(href.split('#', 1)[0].split('?', 1)[0])
            if not target:
                continue
            links += 1
            resolved = ROOT / target.lstrip('/') if target.startswith('/') else path.parent / target
            if not resolved.exists():
                errors.append(f'{path.relative_to(ROOT)}: missing {href}')
    print(f'Markdown: {len(files)} files, {links} local navigation links, {len(errors)} errors')
    for error in errors:
        print(error)
    return bool(errors)


if __name__ == '__main__':
    sys.exit(main())
