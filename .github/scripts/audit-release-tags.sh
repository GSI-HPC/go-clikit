#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
# SPDX-License-Identifier: Apache-2.0
#
# Refuses every release tag, every v* tag in the checkout, that is not a
# signed statement by a listed signer, checking each with
# verify-release-tag.sh, and every version the module proxy lists that has
# no tag in the checkout.
#
# A tag push runs the release workflow of the tagged commit, so a tag on a
# commit from before the workflow, or on one that changes it, verifies
# nothing when it is pushed. This runs from the default branch, on a
# schedule, and finds such a tag whatever commit it names. A tag deleted
# again before this runs is gone from the checkout, but the module proxy
# goes on serving a version it has fetched once, so the versions it lists
# are held to the tags as well. Run in a checkout with all tags:
#
#   ALLOWED_SIGNERS, ALLOWED_PGP_KEYS  as for verify-release-tag.sh
#   GO_MOD        the go.mod naming the module; go.mod if not set
#   MODULE_PROXY  the module proxy; https://proxy.golang.org if not set

set -euo pipefail

verify="$(cd "$(dirname "$0")" && pwd)/verify-release-tag.sh"
go_mod="${GO_MOD:-go.mod}"
proxy="${MODULE_PROXY:-https://proxy.golang.org}"

fail() {
  echo "::error::$*"
  exit 1
}

tags=()
while IFS= read -r tag; do
  tags+=("$tag")
done < <(git for-each-ref --format='%(refname:strip=2)' 'refs/tags/v*')

# The versions the module proxy serves, from its list, which spells a module
# path with each upper-case letter as ! and the letter in lower case. A proxy
# that has never fetched the module answers 404 or 410; any other answer than
# 200 leaves the audit undone, and fails it.
module="$(sed -n 's/^module[[:space:]]*//p' "$go_mod" | tr -d '"[:space:]')"
[ -n "$module" ] || fail "$go_mod names no module"
escaped=''
for ((i = 0; i < ${#module}; i++)); do
  c="${module:i:1}"
  case "$c" in
    [ABCDEFGHIJKLMNOPQRSTUVWXYZ]) escaped+="!${c,,}" ;;
    *) escaped+="$c" ;;
  esac
done
url="$proxy/$escaped/@v/list"
list="$(mktemp)"
trap 'rm -f "$list"' EXIT
status="$(curl --silent --show-error --location --retry 3 --output "$list" \
  --write-out '%{http_code}' "$url")" || fail "cannot fetch $url"
case "$status" in
  200) ;;
  404 | 410) : > "$list" ;;
  *) fail "$url answered $status" ;;
esac

untagged=0
while IFS= read -r version; do
  [ -n "$version" ] || continue
  if ! git rev-parse --verify --quiet "refs/tags/$version" > /dev/null; then
    echo "::error::the module proxy serves $version, but there is no tag $version"
    untagged=$((untagged + 1))
  fi
done < <(tr -d '\r' < "$list")

if [ "${#tags[@]}" -eq 0 ] && [ "$untagged" -eq 0 ]; then
  echo "::notice::no release tags to verify"
  exit 0
fi

failed=0
for tag in "${tags[@]}"; do
  # The commit a tag names is the one it is verified against; a tag that
  # names none fails in verify-release-tag.sh.
  commit="$(git rev-parse --verify --quiet "refs/tags/$tag^{commit}")" || commit="$tag"
  if ! GITHUB_REF_NAME="$tag" GITHUB_SHA="$commit" GITHUB_OUTPUT='' "$verify"; then
    failed=$((failed + 1))
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "::error::$failed of ${#tags[@]} release tag(s) are not signed by a listed signer; see doc/release.md"
fi
if [ "$untagged" -ne 0 ]; then
  echo "::error::$untagged version(s) on the module proxy have no tag; see doc/release.md"
fi
if [ "$failed" -ne 0 ] || [ "$untagged" -ne 0 ]; then
  exit 1
fi
echo "::notice::all ${#tags[@]} release tag(s) are signed by a listed signer, and the module proxy serves no other version"
