#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

shasum -a 256 -c .github/migration-checksums.txt

migrations=()
while IFS= read -r path; do
  migrations+=("$path")
done < <(find backend/internal/db/migrations -maxdepth 1 -type f -name '*.sql' -print | LC_ALL=C sort)
if [[ "${#migrations[@]}" -eq 0 ]]; then
  echo "no SQL migrations found" >&2
  exit 1
fi
manifest_count="$(wc -l < .github/migration-checksums.txt | tr -d ' ')"
if [[ "$manifest_count" != "${#migrations[@]}" ]]; then
  echo "migration checksum manifest must contain exactly one entry per SQL migration" >&2
  exit 1
fi

for path in "${migrations[@]}"; do
  name="${path##*/}"
  if [[ ! "$name" =~ ^[0-9]{4}_[a-z0-9_]+\.sql$ ]]; then
    echo "invalid migration filename: $name" >&2
    exit 1
  fi
  if [[ ! -s "$path" ]]; then
    echo "empty migration: $name" >&2
    exit 1
  fi
  if ! grep -Fq "  $path" .github/migration-checksums.txt; then
    echo "migration missing from checksum manifest: $name" >&2
    exit 1
  fi
done

# Migration identity is the complete filename. Two numeric prefixes were
# shipped twice before this guard existed and cannot safely be renamed because
# live schema_migrations rows contain the original filenames.
duplicate_names="$(
  printf '%s\n' "${migrations[@]##*/}" | LC_ALL=C sort | uniq -d
)"
if [[ -n "$duplicate_names" ]]; then
  echo "duplicate migration versions: $duplicate_names" >&2
  exit 1
fi
duplicate_numeric="$(
  printf '%s\n' "${migrations[@]##*/}" |
    cut -c1-4 |
    LC_ALL=C sort |
    uniq -d |
    grep -Ev '^(0014|0015)$' || true
)"
if [[ -n "$duplicate_numeric" ]]; then
  echo "duplicate numeric migration prefixes: $duplicate_numeric" >&2
  exit 1
fi

echo "validated ${#migrations[@]} immutable, uniquely named migrations"
