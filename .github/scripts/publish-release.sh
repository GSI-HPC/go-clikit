#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
# SPDX-License-Identifier: Apache-2.0
#
# Publishes the GitHub release of a verified tag. The body of the tag
# message, everything after its first line, is the release notes;
# %(contents:body) stops before the signature. A tag without a body gets
# GitHub's list of the pull requests instead. A tag with a pre-release is
# published as a pre-release.
#
# A release that exists already is left as it is, so that a run that failed
# after creating it can be run again. Run in a checkout with the tag:
#
#   TAG       the verified tag, v1.4.0
#   GH_TOKEN  a token that may create releases, for gh

set -euo pipefail

tag="${TAG:?the tag to publish}"

if gh release view "$tag" > /dev/null 2>&1; then
  echo "::notice::the GitHub release $tag exists already and is left as it is"
  exit 0
fi

notes="$(mktemp)"
trap 'rm -f "$notes"' EXIT
git for-each-ref --format='%(contents:body)' "refs/tags/$tag" > "$notes"

args=(--verify-tag --title "$tag")
if grep -q '[^[:space:]]' "$notes"; then
  args+=(--notes-file "$notes")
else
  args+=(--generate-notes)
fi
if [[ "$tag" == *-* ]]; then
  args+=(--prerelease)
fi
gh release create "$tag" "${args[@]}"
