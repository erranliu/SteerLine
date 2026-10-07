"""Read-only checks for document navigation and active/archive separation."""
from pathlib import Path
import re
import sys
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[3]
LINK = re.compile(r'!?\[[^\]\n]*\]\(([^\s)]+)\)')


def prose(text):
    text = re.sub(r'^(`{3,}|~{3,})[^\n]*\n.*?^\1[^\n]*$', '', text, flags=re.M | re.S)
    return re.sub(r'(`+).*?\1', '', text, flags=re.S)


errors = []
files = [ROOT / 'README.md', *sorted((ROOT / 'docs').rglob('*.md'))]
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

tables = []
for name in ('docs/roadmap/roadmap.md', 'docs/roadmap/execution-map.md', 'docs/spec/README.md'):
    text = (ROOT / name).read_text(encoding='utf-8')
    ids = set(re.findall(r'^\|\s*(\d+)\s*\|', text, re.M))
    tables.append(ids)
    if ids != {str(i) for i in range(40, 54)}:
        errors.append(f'{name}: unexpected active IDs {sorted(ids)}')
    for href in LINK.findall(text):
        if '/completed/' in href and href.endswith(('.spec.md', '.plan.md')):
            errors.append(f'{name}: completed task remains an execution entry: {href}')

specs = plans = 0
for path in (ROOT / 'docs/archive/spec/completed').glob('*.spec.md'):
    # three-line-constraints is a common document, not an implementation task.
    if path.name == 'three-line-constraints.spec.md':
        continue
    specs += 1
    partner = path.with_name(path.name.replace('.spec.md', '.plan.md'))
    if partner.exists():
        plans += 1
    elif not path.name.startswith('architecture-0'):
        errors.append(f'{path.relative_to(ROOT)}: missing companion Plan')

print(f'Markdown: {len(files)} files, {links} local navigation links, {len(errors)} errors')
print(f'Active task tables: 14 IDs (#40-#53), consistent={tables[0] == tables[1] == tables[2]}')
print(f'Archived task Specs: {specs}; companion Plans: {plans}; three historical architecture audit Specs had no Plan')
for error in errors:
    print(error)
sys.exit(bool(errors))
