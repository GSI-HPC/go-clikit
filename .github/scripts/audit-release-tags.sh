#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
# SPDX-License-Identifier: Apache-2.0
#
# Refuses every release tag, every v* tag in the checkout, that is neither
# the tag object pinned for it nor a signed statement by a listed signer,
# checking the latter with verify-release-tag.sh; every version the module
# proxy lists, or release pinned, that has no tag in the checkout; and every
# version the module proxy serves from another commit than its tag names,
# asking module-proxy.sh. A version the proxy serves without saying which
# commit it serves it from is unverified, which is a warning.
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
# without failing the releases it signed.
#
# A bad version can be withdrawn but never removed, so a version the go.mod
# retracts on its own, not as part of a range, acknowledges the alarm: what
# is wrong with it is a warning, and passes. Run in a checkout with all tags:
#
#   ALLOWED_SIGNERS, ALLOWED_PGP_KEYS  as for verify-release-tag.sh
#   VERIFIED_TAGS  the pinned releases, from the RELEASE_VERIFIED_TAGS
#                  repository variable: one line each, the tag and the id
#                  of its tag object; # starts a comment line, after
#                  any blanks
#   GO_MOD, MODULE_PROXY  as for module-proxy.sh; the retractions are
#                  read from GO_MOD

set -euo pipefail

scripts="$(cd "$(dirname "$0")" && pwd)"
verify="$scripts/verify-release-tag.sh"
module_proxy="$scripts/module-proxy.sh"
go_mod="${GO_MOD:-go.mod}"

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

# The versions go.mod retracts one by one, in a retract directive or block;
# a range acknowledges none of the versions in it.
declare -A retracts=()
while IFS= read -r version; do
  retracts["$version"]=1
done < <(awk '
  { sub(/\/\/.*/, ""); gsub(/"/, "") }
  block && $1 == ")" { block = 0; next }
  block { if (NF == 1 && $1 ~ /^v/) print $1; next }
  ($1 == "retract" && $2 == "(" && NF == 2) || ($1 == "retract(" && NF == 1) { block = 1; next }
  $1 == "retract" && NF == 2 && $2 ~ /^v/ { print $2 }
' "$go_mod")

# problem <message>: reports a problem, for check.
problem() {
  echo "::error::$*"
  return 1
}

# check <version> <command...>: runs a check of a version, and fails as it
# fails, unless go.mod retracts the version. A retraction is how a bad
# version is withdrawn, and it acknowledges the alarm: the errors of the
# check are then warnings, and it passes.
acknowledged=0
check() {
  local version="$1" out
  shift
  if out="$("$@" 2>&1)"; then
    printf '%s\n' "$out"
    return 0
  fi
  if [ -z "${retracts[$version]+retracted}" ]; then
    printf '%s\n' "$out"
    return 1
  fi
  printf '%s\n' "$out" | sed 's/^::error::/::warning::/'
  echo "::warning::$go_mod retracts $version, which acknowledges this"
  acknowledged=$((acknowledged + 1))
}

# The versions the module proxy serves: each has a tag, and the proxy serves
# it from the commit its tag names. A failure to read the list leaves that
# part of the audit undone, and fails it once the tags are verified.
unread=0
if ! versions="$("$module_proxy" list)"; then
  printf '%s\n' "$versions"
  versions=''
  unread=1
fi

# reported <output>: prints what a check reported, and fails, for check.
reported() {
  printf '%s\n' "$1"
  return 1
}

# A version whose .info does not say which commit the proxy serves it from
# is unverified, a warning: the proxy leaves that out for a version it
# fetched long ago, and that cannot be helped.
untagged=0
moved=0
unverified=0
while IFS= read -r version; do
  [ -n "$version" ] || continue
  if ! git rev-parse --verify --quiet "refs/tags/$version" > /dev/null; then
    check "$version" problem "the module proxy serves $version, but there is no tag $version" ||
      untagged=$((untagged + 1))
    continue
  fi
  commit="$(git rev-parse --verify --quiet "refs/tags/$version^{commit}")" || commit="(none)"
  if out="$("$module_proxy" check "$version" "$commit" 2>&1)"; then
    printf '%s\n' "$out"
    if grep -q '^::warning::' <<< "$out"; then
      unverified=$((unverified + 1))
    fi
    continue
  fi
  check "$version" reported "$out" || moved=$((moved + 1))
done <<< "$versions"

gone=0
for pin in "${!pins[@]}"; do
  if ! git rev-parse --verify --quiet "refs/tags/$pin" > /dev/null; then
    check "$pin" problem "$pin is pinned, but there is no tag $pin" || gone=$((gone + 1))
  fi
done

failed=0
for tag in "${tags[@]}"; do
  if [ -n "${pins[$tag]+pinned}" ]; then
    if [ "$(git rev-parse --verify "refs/tags/$tag")" = "${pins[$tag]}" ]; then
      echo "::notice::$tag is the tag object verified at its release"
    else
      check "$tag" problem "$tag is not the tag object verified at its release, ${pins[$tag]}" ||
        failed=$((failed + 1))
    fi
    continue
  fi
  # The commit a tag names is the one it is verified against; a tag that
  # names none fails in verify-release-tag.sh.
  commit="$(git rev-parse --verify --quiet "refs/tags/$tag^{commit}")" || commit="$tag"
  check "$tag" env GITHUB_REF_NAME="$tag" GITHUB_SHA="$commit" GITHUB_OUTPUT='' "$verify" ||
    failed=$((failed + 1))
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
served="the module proxy serves no other version or commit"
if [ "$unverified" -ne 0 ]; then
  echo "::warning::$unverified version(s) on the module proxy are unverified, since the proxy does not say which commit it serves them from; see doc/release.md"
  served="the module proxy serves no other version, nor another commit that it names"
fi
# What a retraction acknowledged is reported whether or not there are tags:
# only an audit that found nothing at all says there was nothing to verify.
if [ "$acknowledged" -ne 0 ]; then
  echo "::warning::$acknowledged problem(s) are acknowledged by the versions $go_mod retracts"
  echo "::notice::all ${#tags[@]} release tag(s) but the retracted versions are pinned or signed by a listed signer, and $served"
elif [ "${#tags[@]}" -eq 0 ]; then
  echo "::notice::no release tags to verify"
else
  echo "::notice::all ${#tags[@]} release tag(s) are pinned or signed by a listed signer, and $served"
fi
