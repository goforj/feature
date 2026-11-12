#!/usr/bin/env python3
"""Project GoDoc and canonical executable snippets into the README."""
import hashlib
import html
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
# Each API must exist in GoDoc and be exercised inside its destination snippet.
ENTRIES = [
    ('Construction', 'NewMemorySource', 'construction', 'quickstart'),
    ('Construction', 'Open', 'construction', 'quickstart'),
    ('Evaluation', 'Enabled', 'enabled', 'quickstart'),
    ('Evaluation', 'Evaluate', 'evaluate', 'quickstart'),
    ('Consistency', 'Snapshot', 'snapshot', 'refresh'),
    ('Sources', 'Decode', 'decode', 'targeting'),
    ('Sources', 'NewFileSource', 'file', 'refresh'),
    ('Refresh', 'Replace', 'replace', 'polling'),
    ('Refresh', 'Refresh', 'refresh', 'refresh'),
    ('Refresh', 'Run', 'run', 'polling'),
    ('Inspection', 'Status', 'status', 'refresh'),
]


def fingerprint(root=ROOT):
    """Fingerprint library and example sources independently of generated outputs."""
    paths = sorted(list(root.glob('*.go')) + list((root / 'examples').rglob('*.go')) + [root / 'go.mod'])
    digest = hashlib.sha256()
    for path in paths:
        digest.update(str(path.relative_to(root)).encode())
        digest.update(path.read_bytes())
    return digest.hexdigest()


def snippet(root, example, marker):
    """Extract one marked operation without duplicating its executable source."""
    source = (root / 'examples' / example / 'main.go').read_text()
    pattern = rf'(?m)^\s*// snippet:start {marker}\n(.*?)^\s*// snippet:end {marker}$'
    matches = re.findall(pattern, source, re.S | re.M)
    if len(matches) != 1:
        raise ValueError(f'expected exactly one snippet: {marker}')
    lines = matches[0].splitlines()
    if not lines or not lines[0].startswith('\t'):
        raise ValueError(f'snippet is not inside a function: {marker}')
    return '\n'.join(line.removeprefix('\t') for line in lines).strip()


def godoc(root, name):
    """Read the name-leading comment attached to an exported function or method."""
    for path in sorted(root.glob('*.go')):
        if path.name.endswith('_test.go'):
            continue
        lines = path.read_text().splitlines()
        for index, line in enumerate(lines):
            if re.match(rf'^func (?:\([^)]*\) )?{name}\(', line):
                start = index
                while start > 0 and lines[start - 1].startswith('//'):
                    start -= 1
                text = ' '.join(value.removeprefix('//').strip() for value in lines[start:index])
                if not text.startswith(name + ' '):
                    raise ValueError(f'missing name-leading GoDoc: {name}')
                return text
    raise ValueError(f'unknown public API: {name}')


def badge(label, value, color):
    """Render compact, locally owned SVG badges without external credentials."""
    left, right = len(label) * 6 + 12, len(value) * 6 + 12
    width = left + right
    return f'''<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="20" role="img" aria-label="{html.escape(label)}: {html.escape(value)}">
<title>{html.escape(label)}: {html.escape(value)}</title><clipPath id="r"><rect width="{width}" height="20" rx="3"/></clipPath><g clip-path="url(#r)"><path fill="#555" d="M0 0h{left}v20H0z"/><path fill="{color}" d="M{left} 0h{right}v20H{left}z"/></g><g fill="#fff" text-anchor="middle" font-family="Verdana, sans-serif" font-size="11"><text x="{left / 2:g}" y="14">{html.escape(label)}</text><text x="{left + right / 2:g}" y="14">{html.escape(value)}</text></g></svg>
'''


def generate(root=ROOT):
    """Generate only from current successful unit evidence and authoritative inputs."""
    evidence = json.loads((root / 'docs' / 'unit-results.json').read_text())
    if evidence['source_fingerprint'] != fingerprint(root):
        raise ValueError('unit evidence is stale; run make test-unit')
    if evidence['covered_statements'] != evidence['statements'] or evidence['statements'] == 0:
        raise ValueError('library unit statement coverage must be exactly 100%')
    groups = {}
    sections = {}
    for group, name, marker, example in ENTRIES:
        operation = snippet(root, example, marker)
        if not re.search(rf'\b{re.escape(name)}\(', operation):
            raise ValueError(f'{marker} does not demonstrate {name}')
        description = godoc(root, name)
        groups.setdefault(group, []).append(f'[{name}](#{name.lower()})')
        entry = sections.setdefault((group, marker, example), {'names': [], 'docs': [], 'code': operation})
        entry['names'].append(name)
        entry['docs'].append(description)
    index = '| Group | Functions |\n| ---: | :--- |\n' + '\n'.join(
        f'| **{group}** | ' + ' · '.join(names) + ' |' for group, names in groups.items())
    examples = []
    last_group = None
    for (group, marker, example), entry in sections.items():
        if group != last_group:
            examples.append(f'## {group}')
            last_group = group
        if len(entry['names']) > 1:
            for name in entry['names']:
                examples.append(f'<a id="{name.lower()}"></a>')
        examples.extend(['### ' + ' · '.join(entry['names']), '\n\n'.join(entry['docs']),
                         '```go\n' + entry['code'] + '\n```'])
    template = (root / 'docs' / 'README.tmpl.md').read_text()
    quickstart = snippet(root, 'quickstart', 'construction') + '\n\n' + snippet(root, 'quickstart', 'enabled')
    readme = template.replace('{{QUICKSTART}}', quickstart).replace('{{INDEX}}', index).replace('{{EXAMPLES}}', '\n\n'.join(examples))
    if re.search(r'\{\{[A-Z]+\}\}', readme):
        raise ValueError('unresolved template marker')
    if '\u2014' in readme:
        raise ValueError('em dash is not permitted')
    (root / 'README.md').write_text(readme)
    assets = root / 'docs' / 'assets'
    assets.mkdir(exist_ok=True)
    (assets / 'unit.svg').write_text(badge('unit tests', str(evidence['passed_tests']), '#4c1'))
    (assets / 'coverage.svg').write_text(badge('library coverage', '100%', '#4c1'))
    (assets / 'status.svg').write_text(badge('release', 'unreleased', '#777'))
    (assets / 'ci.svg').write_text(badge('CI', 'prepared', '#777'))
    (assets / 'security.svg').write_text(badge('Security', 'prepared', '#777'))
    (assets / 'license.svg').write_text(badge('license', 'MIT', '#007ec6'))
    (assets / 'go.svg').write_text(badge('Go', '1.24+', '#007ec6'))


if __name__ == '__main__':
    try:
        generate()
    except (ValueError, KeyError, OSError) as error:
        print(f'generation failed: {error}', file=sys.stderr)
        sys.exit(1)
