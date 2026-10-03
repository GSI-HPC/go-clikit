#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
# SPDX-License-Identifier: Apache-2.0
#
# Refuses a release tag the go command would not take as a version of this
# module. The go command takes a canonical semantic version only: vX.Y.Z with
# no leading zeros, an optional pre-release of dot-separated identifiers, of
# which a numeric one has no leading zero either, and no build metadata. A
# tag it does not take is no release: the module proxy answers it with a
# pseudo-version. From v2 on the module path has to end in the major version.
#
#   TAG     the tag that was pushed, v1.4.0
#   GO_MOD  the go.mod of the tagged commit; go.mod if not set

set -euo pipefail

tag="${TAG:?the tag to check}"
go_mod="${GO_MOD:-go.mod}"

fail() {
  echo "::error::$*"
  exit 1
}

semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$'
if ! [[ "$tag" =~ $semver ]]; then
  fail "$tag is not a semantic version such as v1.2.3 or v1.2.3-rc.1"
fi
major="${BASH_REMATCH[1]}"
prerelease="${BASH_REMATCH[5]}"

if [ -n "$prerelease" ]; then
  IFS=. read -ra identifiers <<< "$prerelease"
  for identifier in "${identifiers[@]}"; do
    if [[ "$identifier" =~ ^0[0-9]+$ ]]; then
      fail "$tag has the numeric pre-release identifier $identifier with a leading zero, which the go command does not accept"
    fi
  done
fi

module="$(sed -n 's/^module //p' "$go_mod")"
suffix="$(sed -n 's#^.*/v\([0-9][0-9]*\)$#\1#p' <<< "$module")"
if { [ "$major" -ge 2 ] && [ "$suffix" != "$major" ]; } || { [ "$major" -lt 2 ] && [ -n "$suffix" ]; }; then
  fail "$tag does not match the module path $module"
fi
