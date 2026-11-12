#!/usr/bin/env python3
"""Regenerate twice and compare bytes, including an uncommitted initial checkout."""
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[1]
paths = [root / 'README.md'] + sorted((root / 'docs' / 'assets').glob('*.svg'))
before = {str(path): path.read_bytes() for path in paths}
for _ in range(2):
    subprocess.run(['python3', 'tools/generate.py'], cwd=root, check=True)
    after = {str(path): path.read_bytes() for path in paths}
    if before != after:
        raise SystemExit('generated files differ; run make generate and review the changes')
print('Generated README and SVGs match both generations.')
