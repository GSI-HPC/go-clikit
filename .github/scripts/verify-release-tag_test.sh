#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
# SPDX-License-Identifier: Apache-2.0
#
# Tests the scripts of the release workflow against tags made in a scratch
# repository, one case per way a tag has been shown to pass that should not:
# verify-release-tag.sh, audit-release-tags.sh, module-proxy.sh,
# check-release-version.sh and publish-release.sh. The release workflow runs
# only on a tag push and on its schedule, so this is where its steps are
# tested.
#
# Needs git, ssh-keygen, gpg and jq. Run from anywhere:
#
#   .github/scripts/verify-release-tag_test.sh

set -euo pipefail

scripts="$(cd "$(dirname "$0")" && pwd)"
check="$scripts/verify-release-tag.sh"
scratch="$(mktemp -d)"
cleanup() {
  local home
  for home in "$scratch"/gnupg-*; do
    gpgconf --homedir "$home" --kill all > /dev/null 2>&1 || true
  done
  rm -rf "$scratch"
}
trap cleanup EXIT

export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=Maintainer GIT_AUTHOR_EMAIL=maintainer@example.org
export GIT_COMMITTER_NAME=Maintainer GIT_COMMITTER_EMAIL=maintainer@example.org

ssh-keygen -q -t ed25519 -N '' -C listed -f "$scratch/listed"
ssh-keygen -q -t ed25519 -N '' -C unlisted -f "$scratch/unlisted"
listed="maintainer@example.org namespaces=\"git\" $(cat "$scratch/listed.pub")"
unlisted="intruder@example.org namespaces=\"git\" $(cat "$scratch/unlisted.pub")"

# Makes an OpenPGP key without a passphrase in a GnuPG home of its own:
# pgp_key <name>
pgp_key() {
  local home="$scratch/gnupg-$1"
  mkdir -m 0700 "$home"
  GNUPGHOME="$home" gpg --batch --quiet --pinentry-mode loopback --passphrase '' \
    --quick-gen-key "$1 <$1@example.org>" ed25519 sign never 2> /dev/null
}
pgp_key listed
pgp_key unlisted
pgp_listed="$(GNUPGHOME="$scratch/gnupg-listed" gpg --batch --armor --export)"
pgp_unlisted_private="$(GNUPGHOME="$scratch/gnupg-unlisted" gpg --batch --armor \
  --pinentry-mode loopback --passphrase '' --export-secret-keys)"

repo="$scratch/repo"
git init -q "$repo"
cd "$repo"
git commit -q --allow-empty -m 'first'
first="$(git rev-parse HEAD)"

# Signs a tag with the given key, an SSH key by its file name or an OpenPGP
# key as pgp:<name>: sign <key> <git tag arguments...>
sign() {
  local key="$1"
  shift
  case "$key" in
    pgp:*)
      GNUPGHOME="$scratch/gnupg-${key#pgp:}" git -c gpg.format=openpgp \
        -c user.signingKey="${key#pgp:}@example.org" tag -s "$@"
      ;;
    *)
      git -c gpg.format=ssh -c user.signingKey="$scratch/$key" tag -s "$@"
      ;;
  esac
}

failures=0

# run <name> <want: pass|fail> <tag> <sha> <SSH signers> <OpenPGP keys> [<pattern in output>]
run() {
  local name="$1" want="$2" tag="$3" sha="$4" signers="$5" pgp_keys="$6" pattern="${7:-}"
  local out got=pass
  out="$(GITHUB_REF_NAME="$tag" GITHUB_SHA="$sha" ALLOWED_SIGNERS="$signers" \
    ALLOWED_PGP_KEYS="$pgp_keys" GITHUB_OUTPUT="$scratch/output" "$check" 2>&1)" || got=fail
  if [ "$got" != "$want" ]; then
    echo "FAIL $name: want $want, got $got"
    printf '%s\n' "$out" | sed 's/^/    /'
    failures=$((failures + 1))
  elif [ -n "$pattern" ] && ! grep -qF -- "$pattern" <<< "$out"; then
    echo "FAIL $name: output does not say '$pattern'"
    printf '%s\n' "$out" | sed 's/^/    /'
    failures=$((failures + 1))
  else
    echo "ok   $name"
  fi
}

# The release as it is meant to be cut.
sign listed v1.0.0 -m 'release v1.0.0'
: > "$scratch/output"
run 'a tag signed by a listed signer' pass v1.0.0 "$first" "$listed" '' 'is signed by a listed signer'
if ! grep -qx "object=$(git rev-parse refs/tags/v1.0.0)" "$scratch/output" ||
  ! grep -qx "commit=$first" "$scratch/output"; then
  echo "FAIL the verified tag object and commit are not in GITHUB_OUTPUT"
  failures=$((failures + 1))
fi
run 'the push event naming the tag object' pass v1.0.0 "$(git rev-parse refs/tags/v1.0.0)" "$listed" ''
run 'a signer list with a comment and blank lines' pass v1.0.0 "$first" \
  "$(printf '# release signers\n\n%s\n' "$listed")" ''

# The keys come from outside the checkout, and none is a failure.
run 'no key lists' fail v1.0.0 "$first" '' '' 'RELEASE_ALLOWED_SIGNERS and RELEASE_ALLOWED_PGP_KEYS'
run 'blank key lists' fail v1.0.0 "$first" $'  \n' $'\n' 'RELEASE_ALLOWED_SIGNERS and RELEASE_ALLOWED_PGP_KEYS'

mkdir .github
printf '%s\n' "$unlisted" > .github/allowed_signers
git add .github/allowed_signers
git commit -q -m 'add my key'
added="$(git rev-parse HEAD)"
sign unlisted v1.0.1 -m 'release v1.0.1'
run 'a tagged commit listing its own signer' fail v1.0.1 "$added" "$listed" '' 'is not signed by a key'

git rm -q .github/allowed_signers
git commit -q -m 'drop the signer list'
dropped="$(git rev-parse HEAD)"
sign unlisted v1.0.5 -m 'release v1.0.5'
run 'a tagged commit deleting the signer list' fail v1.0.5 "$dropped" "$listed" '' 'is not signed by a key'

# The name inside the signed object is the name pushed.
git update-ref refs/tags/v9.9.9 "$(git rev-parse refs/tags/v1.0.0)"
run 'a signed tag pushed under another name' fail v9.9.9 "$first" "$listed" '' "named 'v1.0.0' but was pushed as v9.9.9"

sign listed v1.1.0 -m 'release v1.1.0' -m 'tag v9.9.8' "$first"
git update-ref refs/tags/v9.9.8 "$(git rev-parse refs/tags/v1.1.0)"
run 'a tag message naming the pushed ref' fail v9.9.8 "$first" "$listed" '' "named 'v1.1.0'"

sign listed v1.2.0 -m 'release v1.2.0' "$first"
run 'a tag of another commit than the pushed one' fail v1.2.0 "$dropped" "$listed" '' "points at $first"

# A signature is what git verifies, not text that looks like one.
git tag -a v1.3.0 "$first" -F - << 'MSG'
release v1.3.0

-----BEGIN SSH SIGNATURE-----
not a signature
-----END SSH SIGNATURE-----
MSG
run 'an unsigned tag quoting a signature' fail v1.3.0 "$first" "$listed" "$pgp_listed" 'is not signed by a key'

git tag -a v1.4.0 -m 'release v1.4.0' "$first"
run 'an unsigned annotated tag' fail v1.4.0 "$first" "$listed" "$pgp_listed" 'is not signed by a key'

git tag v1.5.0 "$first"
run 'a lightweight tag' fail v1.5.0 "$first" "$listed" "$pgp_listed" 'lightweight'

run 'a tag that does not exist' fail v1.6.0 "$first" "$listed" "$pgp_listed" 'not a tag'

# OpenPGP: a listed key passes, alone or next to SSH keys. Any other key does
# not, and neither does a signature in a format no key is listed for.
sign pgp:listed v1.7.0 -m 'release v1.7.0' "$first"
run 'a tag signed by a listed OpenPGP key' pass v1.7.0 "$first" '' "$pgp_listed" 'is signed by a listed signer'
run 'an OpenPGP tag with SSH keys listed too' pass v1.7.0 "$first" "$listed" "$pgp_listed"
run 'an OpenPGP tag with only SSH keys listed' fail v1.7.0 "$first" "$listed" '' 'is not signed by a key'
run 'an SSH tag with only OpenPGP keys listed' fail v1.0.0 "$first" '' "$pgp_listed" 'is not signed by a key'

sign pgp:unlisted v1.7.1 -m 'release v1.7.1' "$first"
run 'a tag signed by an unlisted OpenPGP key' fail v1.7.1 "$first" "$listed" "$pgp_listed" 'is not signed by a key'

# The OpenPGP list holds public keys and nothing else.
run 'an OpenPGP list holding a private key' fail v1.7.0 "$first" '' "$pgp_unlisted_private" 'private key'
run 'an OpenPGP list holding no key' fail v1.7.0 "$first" '' 'not a key' 'cannot import'

# expect <name> <want: pass|fail> <pattern in output, or ''> <command...>
expect() {
  local name="$1" want="$2" pattern="$3" out got=pass
  shift 3
  out="$("$@" 2>&1)" || got=fail
  if [ "$got" != "$want" ]; then
    echo "FAIL $name: want $want, got $got"
    printf '%s\n' "$out" | sed 's/^/    /'
    failures=$((failures + 1))
  elif [ -n "$pattern" ] && ! grep -qF -- "$pattern" <<< "$out"; then
    echo "FAIL $name: output does not say '$pattern'"
    printf '%s\n' "$out" | sed 's/^/    /'
    failures=$((failures + 1))
  else
    echo "ok   $name"
  fi
}

# The audit, from the default branch, of every release tag in the repository:
# a tag on a commit whose release workflow verifies nothing, or that has none,
# is found all the same.
git init -q "$scratch/audit"
cd "$scratch/audit"
git commit -q --allow-empty -m 'before the release workflow'
old="$(git rev-parse HEAD)"
git commit -q --allow-empty -m 'with the release workflow'
sign listed v1.0.0 -m 'release v1.0.0'
git tag not-a-release "$old"
# The module proxy is a stand-in for curl that logs the URL it is asked for,
# or fails with CURL_EXIT. It answers a list with PROXY_STATUS and the file
# PROXY_LIST, and the .info of a version with INFO_STATUS and the file of
# that name in PROXY_INFO, or with 404 if there is none. The proxy lists no
# version unless a case says otherwise.
mkdir "$scratch/proxy-bin" "$scratch/proxy-info"
cat > "$scratch/proxy-bin/curl" << 'CURL'
#!/usr/bin/env bash
set -euo pipefail
out=''
while [ "$#" -gt 1 ]; do
  if [ "$1" = '--output' ]; then
    out="$2"
    shift
  fi
  shift
done
echo "$1" >> "$CURL_LOG"
if [ "${CURL_EXIT:-0}" -ne 0 ]; then
  echo "curl: ($CURL_EXIT) could not connect" >&2
  exit "$CURL_EXIT"
fi
case "$1" in
  */@v/list)
    cp "$PROXY_LIST" "$out"
    printf '%s' "$PROXY_STATUS"
    ;;
  *.info)
    if [ -f "$PROXY_INFO/${1##*/}" ]; then
      cp "$PROXY_INFO/${1##*/}" "$out"
      printf '%s' "${INFO_STATUS:-200}"
    else
      printf 'not found' > "$out"
      printf 404
    fi
    ;;
esac
CURL
chmod +x "$scratch/proxy-bin/curl"
printf 'module github.com/Example-Org/kit\n\ngo 1.26.0\n' > "$scratch/audit.mod"
: > "$scratch/proxy-list"
export PROXY_LIST="$scratch/proxy-list" PROXY_STATUS=200 CURL_LOG="$scratch/curl.log"
export PROXY_INFO="$scratch/proxy-info"
# Has the proxy serve a version from a commit: info <version> <commit>
info() {
  printf '{"Version":"%s","Time":"2026-10-01T00:00:00Z","Origin":{"VCS":"git","URL":"https://example.org/kit","Hash":"%s","Ref":"refs/tags/%s"}}' \
    "$1" "$2" "$1" > "$PROXY_INFO/$1.info"
}
audit() {
  PATH="$scratch/proxy-bin:$PATH" GO_MOD="${AUDIT_MOD:-$scratch/audit.mod}" \
    MODULE_PROXY=https://proxy.example.org ALLOWED_SIGNERS="$1" ALLOWED_PGP_KEYS="$2" \
    VERIFIED_TAGS="${3:-}" GITHUB_OUTPUT="$scratch/output" "$scripts/audit-release-tags.sh"
}
expect 'an audit of signed release tags' pass 'all 1 release tag(s)' audit "$listed" ''
expect 'an audit without key lists' fail 'RELEASE_ALLOWED_SIGNERS and RELEASE_ALLOWED_PGP_KEYS' audit '' ''
git tag -a v0.0.1 -m 'release v0.0.1' "$old"
expect 'an audit finding an unsigned tag on an older commit' fail 'v0.0.1 is not signed by a key' audit "$listed" ''
git tag -d v0.0.1 > /dev/null
sign unlisted v0.0.2 -m 'release v0.0.2' "$old"
expect 'an audit finding a tag by an unlisted signer' fail '1 of 2 release tag(s)' audit "$listed" ''
git tag -d v0.0.2 > /dev/null
git tag v0.0.3 "$old"
expect 'an audit finding a lightweight release tag' fail 'v0.0.3 is a lightweight tag' audit "$listed" ''
git tag -d v0.0.3 > /dev/null
expect 'an audit after the bad tags are gone' pass '' audit "$listed" ''
# A tag deleted again after the module proxy fetched it is found in the
# proxy's list of versions.
: > "$CURL_LOG"
printf 'v1.0.0\n' > "$PROXY_LIST"
released="$(git rev-parse 'refs/tags/v1.0.0^{commit}')"
info v1.0.0 "$released"
expect 'an audit of a version the module proxy serves with its tag' pass 'serves no other version' audit "$listed" ''
want="$(printf '%s\n' 'https://proxy.example.org/github.com/!example-!org/kit/@v/list' \
  'https://proxy.example.org/github.com/!example-!org/kit/@v/v1.0.0.info')"
if [ "$(cat "$CURL_LOG")" != "$want" ]; then
  echo "FAIL the audit asked the module proxy for $(cat "$CURL_LOG") instead of $want"
  failures=$((failures + 1))
fi
# A tag pushed, fetched by the proxy and deleted, and then pushed again
# under the same name on another commit, leaves the proxy serving the first.
info v1.0.0 "$old"
expect 'an audit of a version the module proxy serves from another commit than its tag' fail \
  "the module proxy serves v1.0.0 from commit $old, but the tag v1.0.0 names $released" audit "$listed" ''
expect 'an audit counting the versions served from another commit' fail \
  '1 version(s) on the module proxy are not served from the commit their tag names' audit "$listed" ''
# The proxy leaves Origin out of the .info of a version it fetched long ago:
# the version is unverified, a warning, unless the time of the commit in the
# .info is another than that of the commit the tag names.
# unrecorded <version> <time>: has the proxy serve a version from a commit
# of that time, or of none if it is empty, without saying which commit.
unrecorded() {
  jq -n --arg v "$1" --arg t "$2" '{Version: $v} + (if $t == "" then {} else {Time: $t} end)' \
    > "$PROXY_INFO/$1.info"
}
released_time="$(TZ=UTC git show -s --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ "$released")"
unrecorded v1.0.0 "$released_time"
expect 'an audit of a version the module proxy names no commit for' pass \
  '::warning::the module proxy does not say which commit it serves v1.0.0 from' audit "$listed" ''
expect 'an audit counting the versions the module proxy names no commit for' pass \
  '1 version(s) on the module proxy are unverified' audit "$listed" ''
expect 'an audit not claiming what it could not verify' pass \
  'nor another commit that it names' audit "$listed" ''
unrecorded v1.0.0 ''
expect 'an audit of a version the module proxy names no commit and no time for' pass \
  'the time it gives is none' audit "$listed" ''
printf '{"Version":"v1.0.0","Time":"%s","Origin":{"VCS":"git"}}' "$released_time" > "$PROXY_INFO/v1.0.0.info"
expect 'an audit of a version whose Origin names no commit' pass \
  '::warning::the module proxy does not say which commit it serves v1.0.0 from' audit "$listed" ''
unrecorded v1.0.0 '2000-01-01T00:00:00Z'
expect 'an audit of a version the module proxy serves from a commit of another time' fail \
  "the module proxy serves v1.0.0 from a commit of 2000-01-01T00:00:00Z, but the tag v1.0.0 names $released" \
  audit "$listed" ''
unrecorded v1.0.0 "$released_time"
git tag -a v0.0.9 -m 'release v0.0.9' "$released"
unrecorded v0.0.9 "$released_time"
printf 'v0.0.9\nv1.0.0\n' > "$PROXY_LIST"
expect 'an audit of an unsigned tag of a version the module proxy names no commit for' fail \
  'v0.0.9 is not signed by a key' audit "$listed" ''
git tag -d v0.0.9 > /dev/null
rm "$PROXY_INFO/v0.0.9.info"
printf 'v1.0.0\n' > "$PROXY_LIST"
printf '{"Version":"v1.0.0","Origin":{"Hash":"main"}}' > "$PROXY_INFO/v1.0.0.info"
expect 'an audit of a version whose Origin names no commit hash' fail \
  'names main as the commit it serves v1.0.0 from, which is no commit' audit "$listed" ''
printf 'not JSON' > "$PROXY_INFO/v1.0.0.info"
expect 'an audit of a version whose .info is no JSON' fail 'v1.0.0.info is not a JSON object' audit "$listed" ''
printf '"v1.0.0"' > "$PROXY_INFO/v1.0.0.info"
expect 'an audit of a version whose .info is no JSON object' fail 'v1.0.0.info is not a JSON object' \
  audit "$listed" ''
rm "$PROXY_INFO/v1.0.0.info"
expect 'an audit of a version the module proxy has no .info for' fail 'v1.0.0.info answered 404' audit "$listed" ''
info v1.0.0 "$released"
INFO_STATUS=500 expect 'an audit when the module proxy fails on a .info' fail 'v1.0.0.info answered 500' \
  audit "$listed" ''
# The publishing job holds the version the proxy fetched to the commit it
# verified, before it publishes the release.
proxy_check() {
  PATH="$scratch/proxy-bin:$PATH" GO_MOD="$scratch/audit.mod" \
    MODULE_PROXY=https://proxy.example.org "$scripts/module-proxy.sh" check "$@"
}
expect 'a version the module proxy serves from the verified commit' pass \
  "the module proxy serves v1.0.0 from $released" proxy_check v1.0.0 "$released"
expect 'a version the module proxy serves from another commit' fail \
  "serves v1.0.0 from commit $released, but the tag v1.0.0 names $old" proxy_check v1.0.0 "$old"
unrecorded v1.0.0 "$released_time"
expect 'a version the module proxy names no commit for' pass \
  "::warning::the module proxy does not say which commit it serves v1.0.0 from, so it cannot be held to $released" \
  proxy_check v1.0.0 "$released"
unrecorded v1.0.0 '2000-01-01T00:00:00Z'
expect 'a version the module proxy serves from a commit of another time' fail \
  'from a commit of 2000-01-01T00:00:00Z' proxy_check v1.0.0 "$released"
unrecorded v1.0.0 ''
expect 'a version the module proxy names no commit for, and a commit that is not in the checkout' fail \
  "cannot read the commit $(printf '%040d' 0)" proxy_check v1.0.0 "$(printf '%040d' 0)"
info v1.0.0 "$released"
: > "$CURL_LOG"
info 'v1.1.0-!r!c.1' "$released"
expect 'a version with upper-case letters' pass '' proxy_check v1.1.0-RC.1 "$released"
if [ "$(cat "$CURL_LOG")" != 'https://proxy.example.org/github.com/!example-!org/kit/@v/v1.1.0-!r!c.1.info' ]; then
  echo "FAIL the check asked the module proxy for $(cat "$CURL_LOG")"
  failures=$((failures + 1))
fi
expect 'a check without a commit' fail 'usage' proxy_check v1.0.0
expect 'the module proxy asked for nothing' fail 'usage' \
  env GO_MOD="$scratch/audit.mod" "$scripts/module-proxy.sh"
printf 'v1.0.0\r\n' > "$PROXY_LIST"
expect 'an audit of a version list with CRLF line ends' pass '' audit "$listed" ''
printf 'v0.0.4\nv1.0.0\n' > "$PROXY_LIST"
expect 'an audit finding a version on the module proxy whose tag was deleted' fail \
  'the module proxy serves v0.0.4, but there is no tag v0.0.4' audit "$listed" ''
# A bad version is withdrawn by retracting it in go.mod, which acknowledges
# the alarm for that version, and for no other.
retracting() {
  printf 'module github.com/Example-Org/kit\n\ngo 1.26.0\n\n%s\n' "$1" > "$scratch/retract.mod"
}
retracting 'retract v0.0.4 // Pushed by someone else.'
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a version on the module proxy that go.mod retracts' pass \
  'retract.mod retracts v0.0.4, which acknowledges this' audit "$listed" ''
AUDIT_MOD="$scratch/retract.mod" expect 'an audit saying how many problems are acknowledged' pass \
  '1 problem(s) are acknowledged' audit "$listed" ''
retracting "$(printf 'retract (\n\tv0.0.3 // Tagged from the wrong commit.\n\t"v0.0.4" // Pushed by someone else.\n)')"
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a version go.mod retracts in a block' pass '' audit "$listed" ''
retracting 'retract [v0.0.1, v0.0.9] // Pushed by someone else.'
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a version go.mod retracts in a range' fail \
  'the module proxy serves v0.0.4, but there is no tag v0.0.4' audit "$listed" ''
retracting 'retract v0.0.3 // Tagged from the wrong commit.'
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a version go.mod does not retract' fail \
  '1 version(s) on the module proxy have no tag' audit "$listed" ''
# go.mod comments are no retractions.
retracting '// retract v0.0.4'
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a go.mod naming a version in a comment' fail \
  'no tag v0.0.4' audit "$listed" ''
# A retraction answers the version as the module proxy serves it: a tag
# pushed again later is held to the commit the proxy serves.
retracting 'retract v0.0.4 // Pushed by someone else.'
info v0.0.4 "$old"
git tag -a v0.0.4 -m 'release v0.0.4' "$released"
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a retracted version pushed again on another commit' fail \
  'retracts v0.0.4, which does not acknowledge this: the tag of a retracted version still has to name the commit' \
  audit "$listed" ''
AUDIT_MOD="$scratch/retract.mod" expect 'an audit counting a retracted version pushed again on another commit' fail \
  '1 version(s) on the module proxy are not served from the commit their tag names' audit "$listed" ''
git tag -d v0.0.4 > /dev/null
git tag -a v0.0.4 -m 'release v0.0.4' "$old"
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a retracted version pushed again on the commit the proxy serves' pass \
  '::warning::v0.0.4 is not signed by a key' audit "$listed" ''
git tag -d v0.0.4 > /dev/null
rm "$PROXY_INFO/v0.0.4.info"
printf 'v1.0.0\n' > "$PROXY_LIST"
info v1.0.0 "$old"
retracting 'retract v1.0.0 // Pushed again on another commit.'
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a version served from another commit that go.mod retracts' fail \
  'retracts v1.0.0, which does not acknowledge this' audit "$listed" ''
info v1.0.0 "$released"
git tag -a v0.0.6 -m 'release v0.0.6' "$old"
retracting 'retract v0.0.6 // Not signed.'
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a retracted unsigned tag the module proxy has not fetched' fail \
  'the module proxy does not say that it serves v0.0.6 from the commit the tag names' audit "$listed" ''
printf 'v0.0.6\nv1.0.0\n' > "$PROXY_LIST"
info v0.0.6 "$old"
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of an unsigned tag that go.mod retracts' pass \
  '::warning::v0.0.6 is not signed by a key' audit "$listed" ''
git tag -a v0.0.7 -m 'release v0.0.7' "$old"
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of an unsigned tag next to one that go.mod retracts' fail \
  '1 of 3 release tag(s)' audit "$listed" ''
git tag -d v0.0.7 > /dev/null
# A retracted tag moved later names content that nobody withdrew.
git tag -f -a v0.0.6 -m 'release v0.0.6' "$released" > /dev/null
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a retracted unsigned tag moved to another commit' fail \
  "the module proxy serves v0.0.6 from commit $old, but the tag v0.0.6 names $released" audit "$listed" ''
unrecorded v0.0.6 "$released_time"
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a retracted unsigned tag the module proxy names no commit for' fail \
  'the module proxy does not say that it serves v0.0.6 from the commit the tag names' audit "$listed" ''
git tag -d v0.0.6 > /dev/null
rm "$PROXY_INFO/v0.0.6.info"
printf 'v1.0.0\n' > "$PROXY_LIST"
retracting 'retract v0.0.8 // Deleted.'
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a pinned release whose tag is gone that go.mod retracts' pass \
  'retracts v0.0.8' audit "$listed" '' "v0.0.8 $(printf '%040d' 0)"
git tag -a v0.0.8 -m 'release v0.0.8' "$old"
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a retracted pinned release whose tag is pushed again' fail \
  'the module proxy does not say that it serves v0.0.8 from the commit the tag names' \
  audit "$listed" '' "v0.0.8 $(printf '%040d' 0)"
git tag -d v0.0.8 > /dev/null
PROXY_STATUS=404 expect 'an audit of a module the proxy has not fetched' pass '' audit "$listed" ''
PROXY_STATUS=410 expect 'an audit of a module the proxy refuses as gone' pass '' audit "$listed" ''
PROXY_STATUS=500 expect 'an audit when the module proxy fails' fail 'answered 500' audit "$listed" ''
CURL_EXIT=6 expect 'an audit when the module proxy cannot be reached' fail 'cannot fetch' audit "$listed" ''
PROXY_STATUS=500 expect 'an audit saying what a failing module proxy leaves unchecked' fail \
  'the versions the module proxy serves are unchecked' audit "$listed" ''
# A module proxy that fails leaves the tags verified all the same.
git tag -a v0.0.5 -m 'release v0.0.5' "$old"
PROXY_STATUS=500 expect 'an audit finding an unsigned tag while the module proxy fails' fail \
  'v0.0.5 is not signed by a key' audit "$listed" ''
CURL_EXIT=6 expect 'an audit counting the bad tags while the module proxy cannot be reached' fail \
  '1 of 2 release tag(s)' audit "$listed" ''
git tag -d v0.0.5 > /dev/null
: > "$PROXY_LIST"
printf 'go 1.26.0\n' > "$scratch/nomodule.mod"
expect 'an audit with a go.mod naming no module' fail 'names no module' \
  env GO_MOD="$scratch/nomodule.mod" "$scripts/audit-release-tags.sh"
# valid-before does not retire a key: git checks it against the date in the
# tag, which the signer writes, so a backdated tag passes. A key is retired
# by removing it, after pinning the releases it signed.
retired="maintainer@example.org namespaces=\"git\",valid-before=\"20000101\" $(cat "$scratch/listed.pub")"
expect 'an audit of a tag signed after its key was retired' fail 'v1.0.0 is not signed by a key' audit "$retired" ''
GIT_COMMITTER_DATE='1999-06-01T00:00:00Z' sign listed v0.9.0 -m 'release v0.9.0' "$old"
git tag -d v1.0.0 > /dev/null
expect 'an audit of a tag signed before its key was retired' pass '' audit "$retired" ''
pinned="v0.9.0 $(git rev-parse refs/tags/v0.9.0)"
expect 'an audit of a pinned release after its key was removed' pass \
  'v0.9.0 is the tag object verified at its release' audit '' '' "$pinned"
expect 'an audit of a pin list with a comment and blank lines' pass '' \
  audit '' '' "$(printf '# releases\n\n%s\n' "$pinned")"
expect 'an audit of a pin list with indented comments and blank lines' pass '' \
  audit '' '' "$(printf '  # old pins\n \t \n\t%s \r\n' "$pinned")"
expect 'an audit of a pinned release with its key still listed' pass '' audit "$listed" '' "$pinned"
expect 'an audit of a pinned release whose tag was moved' fail \
  'v0.9.0 is not the tag object verified at its release' \
  audit "$listed" '' "v0.9.0 $(git rev-parse 'refs/tags/not-a-release^{commit}')"
expect 'an audit of a pinned release whose tag was deleted' fail \
  'v0.8.0 is pinned, but there is no tag v0.8.0' \
  audit '' '' "$(printf '%s\nv0.8.0 %s\n' "$pinned" "$(git rev-parse refs/tags/v0.9.0)")"
expect 'an audit of a pin list with a line that is no pin' fail 'RELEASE_VERIFIED_TAGS line 1' \
  audit '' '' 'v0.9.0'
expect 'an audit of a pin list with an object id that is none' fail 'RELEASE_VERIFIED_TAGS line 1' \
  audit '' '' 'v0.9.0 not-an-object-id'
sign unlisted v0.9.1 -m 'release v0.9.1' "$old"
expect 'an audit of a pinned release next to a tag by an unlisted key' fail \
  'v0.9.1 is not signed by a key' audit "$listed" '' "$pinned"
git tag -d v0.9.1 > /dev/null
git init -q "$scratch/untagged"
cd "$scratch/untagged"
git commit -q --allow-empty -m 'first'
expect 'an audit of a repository without release tags' pass 'no release tags' audit '' ''
PROXY_STATUS=500 expect 'an audit of a repository without release tags while the module proxy fails' fail \
  'answered 500' audit '' ''
printf 'v0.1.0\n' > "$PROXY_LIST"
expect 'an audit finding a version on the module proxy and no tags at all' fail \
  '1 version(s) on the module proxy have no tag' audit '' ''
: > "$PROXY_LIST"
expect 'an audit finding a pinned release and no tags at all' fail \
  '1 pinned release(s) have no tag' audit '' '' "v0.1.0 $(printf '%040d' 0)"
# What a retraction acknowledges is reported when there are no tags as well,
# and the audit does not say that there was nothing to verify.
retracting 'retract v0.1.0 // Deleted.'
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a retracted pinned release and no tags at all' pass \
  '1 problem(s) are acknowledged' audit '' '' "v0.1.0 $(printf '%040d' 0)"
printf 'v0.1.0\n' > "$PROXY_LIST"
AUDIT_MOD="$scratch/retract.mod" expect 'an audit of a retracted version on the module proxy and no tags at all' pass \
  '1 problem(s) are acknowledged' audit '' ''
if AUDIT_MOD="$scratch/retract.mod" audit '' '' 2>&1 | grep -qF 'no release tags'; then
  echo "FAIL an audit that acknowledged a problem says there were no release tags to verify"
  failures=$((failures + 1))
else
  echo "ok   an audit that acknowledged a problem does not say there was nothing to verify"
fi
: > "$PROXY_LIST"

# The version: one the go command takes for this module, or none.
printf 'module example.org/kit\n\ngo 1.26.0\n' > "$scratch/go.mod"
printf 'module example.org/kit/v2\n\ngo 1.26.0\n' > "$scratch/go-v2.mod"
v1mod="$scratch/go.mod"
v2mod="$scratch/go-v2.mod"
version() {
  TAG="$1" GO_MOD="$2" "$scripts/check-release-version.sh"
}
expect 'a release version' pass '' version v1.2.3 "$v1mod"
expect 'a pre-release version' pass '' version v0.1.0-rc.1 "$v1mod"
expect 'a pre-release identifier of zero' pass '' version v0.1.0-0 "$v1mod"
expect 'alphanumeric identifiers starting with zero' pass '' version v0.1.0-01a.0a "$v1mod"
expect 'hyphens in pre-release identifiers' pass '' version v0.1.0-rc-1.x-y "$v1mod"
expect 'a numeric pre-release identifier with a leading zero' fail 'leading zero' version v0.1.0-rc.01 "$v1mod"
expect 'a leading zero in the first pre-release identifier' fail 'leading zero' version v0.1.0-00 "$v1mod"
expect 'a leading zero in the major version' fail 'not a semantic version' version v01.2.3 "$v1mod"
expect 'a version without its patch number' fail 'not a semantic version' version v1.2 "$v1mod"
expect 'an empty pre-release identifier' fail 'not a semantic version' version v1.2.3-rc..1 "$v1mod"
expect 'build metadata' fail 'not a semantic version' version v1.2.3+build "$v1mod"
expect 'v2 in a module path without /v2' fail 'does not match the module path' version v2.0.0 "$v1mod"
expect 'v2 in a module path ending in /v2' pass '' version v2.0.0 "$v2mod"
expect 'v1 in a module path ending in /v2' fail 'does not match the module path' version v1.0.0 "$v2mod"

# Publishing, against a stand-in for gh that logs what it is asked to do, the
# notes file by its content. A run that failed after the release was created
# can be run again.
mkdir "$scratch/bin"
cat > "$scratch/bin/gh" << 'GH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$1 $2" = 'release view' ]; then
  exit "$GH_VIEW_STATUS"
fi
args=()
notes=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = '--notes-file' ]; then
    notes="$(cat "$2")"
    args+=("$1" NOTES)
    shift 2
  else
    args+=("$1")
    shift
  fi
done
echo "gh ${args[*]}" >> "$GH_LOG"
if [ -n "$notes" ]; then
  printf '%s\n' "$notes" | sed 's/^/notes: /' >> "$GH_LOG"
fi
GH
chmod +x "$scratch/bin/gh"
git init -q "$scratch/publish"
cd "$scratch/publish"
git commit -q --allow-empty -m 'first'
git tag -a v0.3.0 --cleanup=whitespace -F - << 'MSG'
go-clikit v0.3.0

## Changes
MSG
git tag -a v0.4.0 -m 'go-clikit v0.4.0'
git tag -a v0.5.0-rc.1 -m 'go-clikit v0.5.0-rc.1'

# publish <tag> <exit status of gh release view> <what gh is asked to do>
publish() {
  local tag="$1" view="$2" want="$3" out got
  : > "$scratch/gh.log"
  if ! out="$(PATH="$scratch/bin:$PATH" GH_LOG="$scratch/gh.log" GH_VIEW_STATUS="$view" \
    TAG="$tag" "$scripts/publish-release.sh" 2>&1)"; then
    echo "FAIL publishing $tag: the script failed"
    printf '%s\n' "$out" | sed 's/^/    /'
    failures=$((failures + 1))
    return
  fi
  got="$(cat "$scratch/gh.log")"
  if [ "$got" != "$want" ]; then
    echo "FAIL publishing $tag: gh was asked"
    printf '%s\n' "$got" | sed 's/^/    /'
    echo "  instead of"
    printf '%s\n' "$want" | sed 's/^/    /'
    failures=$((failures + 1))
  else
    echo "ok   publishing $tag, gh release view exiting $view"
  fi
}
publish v0.3.0 1 $'gh release create v0.3.0 --verify-tag --title v0.3.0 --notes-file NOTES\nnotes: ## Changes'
publish v0.4.0 1 'gh release create v0.4.0 --verify-tag --title v0.4.0 --generate-notes'
publish v0.5.0-rc.1 1 'gh release create v0.5.0-rc.1 --verify-tag --title v0.5.0-rc.1 --generate-notes --prerelease'
publish v0.3.0 0 ''

if [ "$failures" -ne 0 ]; then
  echo "$failures case(s) failed"
  exit 1
fi
