#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
# SPDX-License-Identifier: Apache-2.0
#
# Refuses every release tag, every v* tag in the checkout, that is not a
# signed statement by a listed signer, checking each with
# verify-release-tag.sh.
#
# A tag push runs the release workflow of the tagged commit, so a tag on a
# commit from before the workflow, or on one that changes it, verifies
# nothing when it is pushed. This runs from the default branch, on a
# schedule, and finds such a tag whatever commit it names. Run in a checkout
# with all tags:
#
#   ALLOWED_SIGNERS, ALLOWED_PGP_KEYS  as for verify-release-tag.sh

set -euo pipefail

verify="$(cd "$(dirname "$0")" && pwd)/verify-release-tag.sh"

tags=()
while IFS= read -r tag; do
  tags+=("$tag")
done < <(git for-each-ref --format='%(refname:strip=2)' 'refs/tags/v*')

if [ "${#tags[@]}" -eq 0 ]; then
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
  exit 1
fi
echo "::notice::all ${#tags[@]} release tag(s) are signed by a listed signer"
