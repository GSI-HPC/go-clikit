<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Releases

A release is a signed tag ([decision 4](decisions.md#4-a-release-is-a-signed-tag)). Only the
maintainers create tags.

## Cutting a release

1. Pick the commit on `main`, with CI green on it, and the version: a
   breaking change in v0 raises the minor version, and so does a higher
   `go` line.
2. Write the message to a file. Its first line is the title; the rest is
   Markdown and becomes the release notes:

   ```markdown
   go-clikit v0.2.0

   ## Changes

   - ...
   ```

3. Tag and push. git's default cleanup deletes every line that starts with
   `#`, Markdown headings included, so keep whitespace cleanup only:

   ```console
   $ git tag -s v0.2.0 --cleanup=whitespace -F notes.md <commit>
   $ git push origin v0.2.0
   ```

The Release workflow verifies the tag and its version, tests the tagged
commit on both Go lines, fetches the version through the module proxy, after
which pkg.go.dev lists it, checks that the proxy serves it from the verified
commit, and publishes the GitHub release. The version is
one the go command accepts: `vX.Y.Z`, optionally with a pre-release such as
`-rc.1`, with no leading zeros in any number, `-rc.01` included, and no
build metadata.

If the publishing job fails, run it again: it fetches the version once more
and leaves a GitHub release that an earlier attempt created as it is.

Once the release is published, pin it: add the line the summary of the
publishing job shows to `RELEASE_VERIFIED_TAGS`, as under *Pinning releases
and retiring a key*.

## The daily audit

A tag push runs the Release workflow as the tagged commit has it. A tag on a
commit from before the workflow, or on a commit that changes it, therefore
verifies nothing when it is pushed. So the workflow also runs every day from
`main`, and can be started by hand, and then verifies every `v*` tag in the
repository against the listed keys. A tag that was pushed, fetched through
the module proxy and deleted again between two runs is no longer in the
repository, but the proxy goes on serving its version; so the audit also
reads the versions proxy.golang.org lists for the module, and fails for each
one that has no tag. The proxy also keeps the content it fetched first: a
tag deleted and pushed again under the same name on another commit, even a
signed one, leaves the proxy serving the first commit. So the audit also
fails for each version whose `.info` on the proxy names, as `Origin.Hash`,
another commit than its tag, or none; the publishing job checks the same
before it publishes. A failed run of the job *Verify every release tag is
signed* is the alarm: its log names the tag or the version. Investigate it
as a compromise, and withdraw the version as below. GitHub disables
scheduled workflows in a repository with no activity for 60 days; enable
the workflow again when that happens.

The audit reads the proxy's list of the module's versions, not the feed at
index.golang.org. The feed records every version the proxy has fetched, but
of every module in one stream that cannot be asked for one module, so the
audit would read all of it every day. The list goes on listing a version
the proxy has fetched, retracted or not; the proxy drops one only in rare
cases, such as a legal request. When the proxy cannot be read, the audit
verifies the tags all the same, and fails, saying that the versions are
unchecked.

The audit finds a bad release, up to a day late; it does not prevent one.
What prevents one is the tag ruleset under *Setting up verification*, which
lets only the maintainers create `v*` tags and nobody delete them.

## Pinning releases and retiring a key

The audit checks a tag against today's keys unless the
`RELEASE_VERIFIED_TAGS` repository variable pins it. A pin is one line: the
tag and the id of the tag object the Release workflow verified when the tag
was pushed, which the summary of the publishing job shows, and which
`git rev-parse v0.2.0` prints as well. Blank lines are skipped, and a line
whose first character other than a blank is `#` is a comment.

```
v0.2.0 3f1c2a9d0e4b8c7f6a5d4e3c2b1a09f8e7d6c5b4
```

A pinned tag passes the audit while it names that object, whatever the
keys, and fails it once it names another or is gone. Pin each release once
it is published.

To retire a key, pin every release it signed and then remove the key from
`RELEASE_ALLOWED_SIGNERS` or `RELEASE_ALLOWED_PGP_KEYS`. A key that may have
leaked is removed at once, and only the releases known to be genuine are
pinned. Do not retire an SSH key with a `valid-before` option instead: git
checks it against the date in the tag, which whoever holds the key writes,
so a tag dated before it verifies. An OpenPGP key that expires stops
verifying the tags it signed earlier as well as new ones, so pin its
releases before it expires.

## Withdrawing a release

Never move or delete a pushed tag: the module proxy and the checksum
database keep it regardless. Add a `retract` directive with the reason to
`go.mod` and ship it in the next release:

```go
retract v0.2.0 // Tagged from the wrong commit.
```

## Setting up verification

The workflow checks the signature against two repository variables, and
publishes nothing while both are empty. They are variables rather than files,
because a file in the tagged commit could name its own signers. Set them
under *Settings → Secrets and variables → Actions → Variables*. A variable is
no secret, since a workflow can print it and GitHub does not mask it in the
log, so these hold public keys only. A third variable,
`RELEASE_VERIFIED_TAGS`, pins the published releases for the daily audit.

`RELEASE_ALLOWED_SIGNERS` lists the SSH keys, one line per signer in the
format of git's `gpg.ssh.allowedSignersFile`:

```
name@example.org namespaces="git" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA...
```

Sign with the matching key:

```console
$ git config gpg.format ssh
$ git config user.signingKey ~/.ssh/id_ed25519.pub
```

`RELEASE_ALLOWED_PGP_KEYS` holds the OpenPGP public keys, ASCII armored, one
block after the other, as `gpg --armor --export <fingerprint>` prints them.
Any key in it may sign, however gpg would otherwise trust it. The workflow
knows only what the variable holds: a key that has expired no longer
verifies, but a revoked key verifies until the variable holds its
revocation, so export the key again after revoking it, or remove it, having
pinned its releases first. Sign with the key:

```console
$ git config gpg.format openpgp
$ git config user.signingKey <fingerprint>
```

The publishing job runs in the `release` environment. Restrict its
deployment branches and tags to `v*`, and protect `v*` tags with a ruleset
that lets only the maintainers create them and nobody update or delete
them.
