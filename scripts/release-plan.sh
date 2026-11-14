#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
version="${1:-}"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'usage: scripts/release-plan.sh vX.Y.Z' >&2
  exit 2
fi
while read -r classification directory; do
  [[ "$classification" == published ]] || continue
  if [[ "$directory" == . ]]; then
    printf '%s  %s\n' "$version" "$directory"
  else
    printf '%s/%s  %s\n' "$directory" "$version" "$directory"
  fi
done < scripts/module-manifest.txt
cat <<'PLAN'
Preflight every remote tag and verify CI/Security for the exact committed candidates.
Publish and verify the root first. Then update driver requirements to the root release,
preserve development replacements, verify the next candidate, and publish both driver tags.
Run make verify-release VERSION=<version> with normal public proxy/checksum verification.
This command does not create tags or publish a release.
PLAN
