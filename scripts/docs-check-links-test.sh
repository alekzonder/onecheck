#!/usr/bin/env bash
# Tests scripts/docs-check-links.sh against small fixture trees.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd -P)/docs-check-links.sh"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
fail=0

# expect <want-exit> <name> <relative-file> <content>
expect() {
  local want="$1" name="$2" file="$3" content="$4" dir got=0
  dir="$tmp/$name"
  mkdir -p "$(dirname "$dir/$file")"
  printf '%s\n' "$content" >"$dir/$file"
  "$script" "$dir" >"$tmp/$name.out" 2>&1 || got=$?
  if [ "$got" -ne "$want" ]; then
    echo "FAIL $name: exit $got, want $want" >&2
    cat "$tmp/$name.out" >&2
    fail=1
  fi
}

expect 1 relative page.md 'See [checks](checks.md).'
expect 1 relative-anchor modules/disk.md 'See [checks](../checks.md#gt-gte-lt-lte).'
expect 1 mdx page.mdx 'See [checks](./checks.md).'
expect 0 route page.md 'See [checks](/checks#gt-gte-lt-lte).'
expect 0 external page.md 'See [syntax](https://github.com/tidwall/gjson/blob/master/SYNTAX.md).'
expect 0 superpowers superpowers/specs/x.md 'See [checks](../../checks.md).'
expect 0 node-modules node_modules/blume/README.md 'See [a](a.md).'

if ! grep -q 'page.md:1:' "$tmp/relative.out"; then
  echo "FAIL relative: output does not name file and line" >&2
  cat "$tmp/relative.out" >&2
  fail=1
fi

[ "$fail" -eq 0 ] && echo "docs-check-links: ok"
exit "$fail"
