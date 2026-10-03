#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
# SPDX-License-Identifier: Apache-2.0
#
# Asks the module proxy about the module of a go.mod:
#
#   module-proxy.sh list               prints the versions the proxy lists,
#                                      one a line, and none for a module it
#                                      has never fetched
#   module-proxy.sh check <v> <commit> fails unless the proxy serves version
#                                      <v> from the commit <commit>
#
# The proxy names the commit it fetched a version from as Origin.Hash in the
# version's .info, and goes on serving that content whatever the tag names
# later. A version it serves from another commit than its tag names, or
# without saying which, is refused. Reads:
#
#   GO_MOD        the go.mod naming the module; go.mod if not set
#   MODULE_PROXY  the module proxy; https://proxy.golang.org if not set

set -euo pipefail

go_mod="${GO_MOD:-go.mod}"
proxy="${MODULE_PROXY:-https://proxy.golang.org}"

fail() {
  echo "::error::$*"
  exit 1
}

# The proxy spells a path or a version with each upper-case letter as ! and
# the letter in lower case.
escape() {
  local s="$1" c out='' i
  for ((i = 0; i < ${#s}; i++)); do
    c="${s:i:1}"
    case "$c" in
      [ABCDEFGHIJKLMNOPQRSTUVWXYZ]) out+="!${c,,}" ;;
      *) out+="$c" ;;
    esac
  done
  printf '%s' "$out"
}

module="$(sed -n 's/^module[[:space:]]*//p' "$go_mod" | tr -d '"[:space:]')"
[ -n "$module" ] || fail "$go_mod names no module"
base="$proxy/$(escape "$module")/@v"

body="$(mktemp)"
trap 'rm -f "$body"' EXIT

# get <url>: the body into $body, the HTTP status into $status.
get() {
  status="$(curl --silent --show-error --location --retry 3 --output "$body" \
    --write-out '%{http_code}' "$1")" || fail "cannot fetch $1"
}

case "${1:-}" in
  list)
    [ "$#" -eq 1 ] || fail "usage: $0 list"
    url="$base/list"
    get "$url"
    case "$status" in
      200) tr -d '\r' < "$body" | sed '/^$/d' ;;
      # A proxy that has never fetched the module answers 404 or 410.
      404 | 410) ;;
      *) fail "$url answered $status" ;;
    esac
    ;;
  check)
    [ "$#" -eq 3 ] || fail "usage: $0 check <version> <commit>"
    version="$2" commit="$3"
    url="$base/$(escape "$version").info"
    get "$url"
    [ "$status" = 200 ] || fail "$url answered $status"
    served="$(jq -r '.Origin.Hash // empty | strings' "$body" 2> /dev/null)" || served=''
    if ! [[ "$served" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]]; then
      fail "the module proxy does not say which commit it serves $version from"
    fi
    if [ "$served" != "$commit" ]; then
      fail "the module proxy serves $version from commit $served, but the tag $version names $commit"
    fi
    echo "::notice::the module proxy serves $version from $commit"
    ;;
  *)
    fail "usage: $0 list | check <version> <commit>"
    ;;
esac
