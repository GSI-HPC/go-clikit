#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
# SPDX-License-Identifier: Apache-2.0
#
# Refuses every release tag, every v* tag in the checkout, that is neither
# the tag object pinned for it nor a signed statement by a listed signer,
# checking the latter with verify-release-tag.sh; every version the module
# proxy lists, or release pinned, that has no tag in the checkout; and every
# version the module proxy serves from another commit than its tag names,
# asking module-proxy.sh.
#
# A tag push runs the release workflow of the tagged commit, so a tag on a
# commit from before the workflow, or on one that changes it, verifies
# nothing when it is pushed. This runs from the default branch, on a
# schedule, and finds such a tag whatever commit it names. A tag deleted
# again before this runs is gone from the checkout, but the module proxy
# goes on serving a version it has fetched once, so the versions it lists
# are held to the tags as well, and to the commits they name: a tag deleted
# and pushed again under the same name names another commit than the one
# the proxy serves.
#
# A pinned release is held to the tag object verified when it was pushed
# rather than to today's keys, so that a key is retired by removing it
# without failing the releases it signed. Run in a checkout with all tags:
#
#   ALLOWED_SIGNERS, ALLOWED_PGP_KEYS  as for verify-release-tag.sh
#   VERIFIED_TAGS  the pinned releases, from the RELEASE_VERIFIED_TAGS
#                  repository variable: one line each, the tag and the id
#                  of its tag object; # starts a comment line, after
#                  any blanks
#   GO_MOD, MODULE_PROXY  as for module-proxy.sh

set -euo pipefail

scripts="$(cd "$(dirname "$0")" && pwd)"
verify="$scripts/verify-release-tag.sh"
module_proxy="$scripts/module-proxy.sh"

fail() {
  echo "::error::$*"
  exit 1
}

declare -A pins=()
n=0
while IFS= read -r line || [ -n "$line" ]; do
  n=$((n + 1))
  line="${line%$'\r'}"
  # A blank line or a comment may be indented; read strips the blanks
  # around a pin.
  line="${line#"${line%%[![:space:]]*}"}"
  case "$line" in
    '' | '#'*) continue ;;
  esac
  read -r pin object rest <<< "$line"
  if [ -n "$rest" ] || ! [[ "$object" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]]; then
    fail "RELEASE_VERIFIED_TAGS line $n is not a tag and the id of its tag object"
  fi
  pins["$pin"]="$object"
done <<< "${VERIFIED_TAGS:-}"

tags=()
while IFS= read -r tag; do
  tags+=("$tag")
done < <(git for-each-ref --format='%(refname:strip=2)' 'refs/tags/v*')

# The versions the module proxy serves: each has a tag, and the proxy serves
# it from the commit its tag names. A failure to read the list leaves that
# part of the audit undone, and fails it once the tags are verified.
unread=0
if ! versions="$("$module_proxy" list)"; then
  printf '%s\n' "$versions"
  versions=''
  unread=1
fi

untagged=0
moved=0
while IFS= read -r version; do
  [ -n "$version" ] || continue
  if ! git rev-parse --verify --quiet "refs/tags/$version" > /dev/null; then
    echo "::error::the module proxy serves $version, but there is no tag $version"
    untagged=$((untagged + 1))
    continue
  fi
  commit="$(git rev-parse --verify --quiet "refs/tags/$version^{commit}")" || commit="(none)"
  if ! "$module_proxy" check "$version" "$commit"; then
    moved=$((moved + 1))
  fi
done <<< "$versions"

gone=0
for pin in "${!pins[@]}"; do
  if ! git rev-parse --verify --quiet "refs/tags/$pin" > /dev/null; then
    echo "::error::$pin is pinned, but there is no tag $pin"
    gone=$((gone + 1))
  fi
done

if [ "${#tags[@]}" -eq 0 ] && [ "$untagged" -eq 0 ] && [ "$moved" -eq 0 ] && [ "$gone" -eq 0 ] &&
  [ "$unread" -eq 0 ]; then
  echo "::notice::no release tags to verify"
  exit 0
fi

failed=0
for tag in "${tags[@]}"; do
  if [ -n "${pins[$tag]+pinned}" ]; then
    if [ "$(git rev-parse --verify "refs/tags/$tag")" = "${pins[$tag]}" ]; then
      echo "::notice::$tag is the tag object verified at its release"
    else
      echo "::error::$tag is not the tag object verified at its release, ${pins[$tag]}"
      failed=$((failed + 1))
    fi
    continue
  fi
  # The commit a tag names is the one it is verified against; a tag that
  # names none fails in verify-release-tag.sh.
  commit="$(git rev-parse --verify --quiet "refs/tags/$tag^{commit}")" || commit="$tag"
  if ! GITHUB_REF_NAME="$tag" GITHUB_SHA="$commit" GITHUB_OUTPUT='' "$verify"; then
    failed=$((failed + 1))
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "::error::$failed of ${#tags[@]} release tag(s) are neither pinned nor signed by a listed signer; see doc/release.md"
fi
if [ "$untagged" -ne 0 ]; then
  echo "::error::$untagged version(s) on the module proxy have no tag; see doc/release.md"
fi
if [ "$moved" -ne 0 ]; then
  echo "::error::$moved version(s) on the module proxy are not served from the commit their tag names; see doc/release.md"
fi
if [ "$gone" -ne 0 ]; then
  echo "::error::$gone pinned release(s) have no tag; see doc/release.md"
fi
if [ "$unread" -ne 0 ]; then
  echo "::error::the versions the module proxy serves are unchecked, since its list could not be read"
fi
if [ "$failed" -ne 0 ] || [ "$untagged" -ne 0 ] || [ "$moved" -ne 0 ] || [ "$gone" -ne 0 ] ||
  [ "$unread" -ne 0 ]; then
  exit 1
fi
echo "::notice::all ${#tags[@]} release tag(s) are pinned or signed by a listed signer, and the module proxy serves no other version or commit"
