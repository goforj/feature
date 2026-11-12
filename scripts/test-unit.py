#!/usr/bin/env python3
"""Run unit tests and instrumented programs, then enforce exact library coverage."""
import json
import os
from pathlib import Path
import subprocess
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'tools'))
from generate import ROOT, fingerprint

artifacts = ROOT / '.artifacts'
artifacts.mkdir(exist_ok=True)
coverage_dir = artifacts / 'example-coverage'
coverage_dir.mkdir(exist_ok=True)
for old in coverage_dir.iterdir():
    old.unlink()
env = dict(os.environ, GOWORK='off', FEATURE_EXAMPLE_COVERAGE_DIR=str(coverage_dir))
command = ['go', 'test', '-json', '-count=1', '-coverpkg=github.com/goforj/feature',
           '-coverprofile=' + str(artifacts / 'root.cover'), './...']
result = subprocess.run(command, cwd=ROOT, env=env, capture_output=True, text=True)
(artifacts / 'unit.json').write_text(result.stdout)
events = [json.loads(line) for line in result.stdout.splitlines()]
if result.returncode:
    for event in events:
        if 'Output' in event:
            print(event['Output'], end='')
    print(result.stderr, file=sys.stderr)
    sys.exit(result.returncode)
if not list(coverage_dir.glob('covcounters.*')):
    raise SystemExit('example coverage was not recorded')
subprocess.run(['go', 'tool', 'covdata', 'textfmt', '-i=' + str(coverage_dir),
                '-o=' + str(artifacts / 'examples.cover')], check=True, cwd=ROOT)
blocks = {}
for name in ['root.cover', 'examples.cover']:
    for line in (artifacts / name).read_text().splitlines()[1:]:
        location, statements, count = line.split()
        relative = location.removeprefix("github.com/goforj/feature/").split(":")[0]
        if "/" in relative:
            continue
        previous = blocks.get(location, (int(statements), 0))
        if previous[0] != int(statements):
            raise ValueError('coverage source mismatch')
        blocks[location] = (int(statements), max(previous[1], int(count)))
profile = 'mode: set\n' + ''.join(f'{key} {size} {int(count > 0)}\n' for key,(size,count) in sorted(blocks.items()))
(artifacts / 'unit.cover').write_text(profile)
statements = sum(size for size,count in blocks.values())
covered = sum(size for size,count in blocks.values() if count)
if not statements or covered != statements:
    print(f'Coverage incomplete: {covered}/{statements} library statements', file=sys.stderr)
    subprocess.run(['go', 'tool', 'cover', '-func=' + str(artifacts / 'unit.cover')], cwd=ROOT)
    sys.exit(1)
report = {
    'scope': 'github.com/goforj/feature runtime package; tooling and executable scaffolding are not runtime library code',
    'source_fingerprint': fingerprint(),
    'statements': statements,
    'covered_statements': covered,
    'passed_tests': sum(e.get('Action') == 'pass' and bool(e.get('Test')) for e in events),
    'executed_programs': len(list((ROOT / 'examples').glob('*/main.go'))),
}
(ROOT / 'docs' / 'unit-results.json').write_text(json.dumps(report, indent=2, sort_keys=True) + '\n')
print(f'Passed {report["passed_tests"]} test cases and {report["executed_programs"]} programs; {covered}/{statements} library statements covered.')
