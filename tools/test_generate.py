"""Exercise generation ownership and failure detection using isolated fixtures."""
import json
from pathlib import Path
import re
import shutil
import tempfile
import unittest

import generate


class GeneratorTests(unittest.TestCase):
    """Keep generated navigation tied to real APIs and executable operations."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for path in generate.ROOT.glob('*.go'):
            shutil.copy2(path, self.root / path.name)
        shutil.copy2(generate.ROOT / 'go.mod', self.root / 'go.mod')
        shutil.copytree(generate.ROOT / 'examples', self.root / 'examples')
        (self.root / 'docs').mkdir()
        shutil.copytree(generate.ROOT / 'docs' / 'assets', self.root / 'docs' / 'assets')
        shutil.copy2(generate.ROOT / 'docs' / 'README.tmpl.md', self.root / 'docs' / 'README.tmpl.md')
        self.record_evidence()

    def record_evidence(self):
        """Use explicit test-only evidence to isolate generator behavior."""
        self.evidence = {
            'source_fingerprint': generate.fingerprint(self.root),
            'statements': 1,
            'covered_statements': 1,
            'passed_tests': 1,
        }
        self.write_evidence()

    def write_evidence(self):
        (self.root / 'docs' / 'unit-results.json').write_text(json.dumps(self.evidence))

    def test_idempotence_and_navigation(self):
        generate.generate(self.root)
        first = (self.root / 'README.md').read_bytes()
        generate.generate(self.root)
        self.assertEqual(first, (self.root / 'README.md').read_bytes())
        readme = first.decode()
        self.assertNotIn('{{', readme)
        for group, name, marker, example in generate.ENTRIES:
            self.assertIn(f'[{name}](#api-{name.lower()})', readme)
            self.assertEqual(readme.count(f'<a id="api-{name.lower()}"></a>'), 1)
            heading = re.search(r'(?m)^### .*\b' + name + r'\b.*$', readme)
            self.assertIsNotNone(heading)
            section = readme[heading.end():].split('\n##', 1)[0]
            self.assertRegex(section, rf'\b{name}\(')
        for image in re.findall(r'<img src="([^"]+)"', readme):
            if image.startswith('docs/assets/') and not image.endswith('logo.png'):
                self.assertTrue((self.root / image).is_file(), image)

    def test_stale_evidence(self):
        (self.root / 'doc.go').write_text('// changed\npackage feature\n')
        with self.assertRaisesRegex(ValueError, 'stale'):
            generate.generate(self.root)

    def test_incomplete_coverage(self):
        self.evidence['covered_statements'] = 0
        self.write_evidence()
        with self.assertRaisesRegex(ValueError, 'exactly 100'):
            generate.generate(self.root)

    def test_missing_snippet(self):
        with self.assertRaisesRegex(ValueError, 'exactly one snippet'):
            generate.snippet(self.root, 'quickstart', 'missing')

    def test_snippet_outside_function(self):
        path = self.root / 'examples' / 'quickstart' / 'main.go'
        path.write_text(path.read_text().replace('\t// snippet:start enabled\n\tsubject', '\t// snippet:start enabled\nsubject'))
        with self.assertRaisesRegex(ValueError, 'inside a function'):
            generate.snippet(self.root, 'quickstart', 'enabled')

    def test_missing_api(self):
        with self.assertRaisesRegex(ValueError, 'unknown public API'):
            generate.godoc(self.root, 'MissingAPI')

    def test_missing_godoc(self):
        path = self.root / 'client.go'
        path.write_text(path.read_text().replace('// Open loads', '// Wrong loads'))
        with self.assertRaisesRegex(ValueError, 'name-leading'):
            generate.godoc(self.root, 'Open')

    def test_wrong_demonstrated_operation(self):
        path = self.root / 'examples' / 'quickstart' / 'main.go'
        path.write_text(path.read_text().replace('features.Enabled(', 'features.Other('))
        self.record_evidence()
        with self.assertRaisesRegex(ValueError, 'does not demonstrate'):
            generate.generate(self.root)

    def test_unresolved_marker(self):
        path = self.root / 'docs' / 'README.tmpl.md'
        path.write_text(path.read_text() + '\n{{MISSING}}\n')
        with self.assertRaisesRegex(ValueError, 'unresolved'):
            generate.generate(self.root)

    def test_style_guard(self):
        path = self.root / 'docs' / 'README.tmpl.md'
        path.write_text(path.read_text() + chr(0x2014))
        with self.assertRaisesRegex(ValueError, 'em dash'):
            generate.generate(self.root)

    def test_badge_escaping(self):
        svg = generate.badge('a&b', '<value>', '#555')
        self.assertIn('a&amp;b', svg)
        self.assertIn('&lt;value&gt;', svg)
