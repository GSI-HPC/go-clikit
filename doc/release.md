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
another commit than its tag; the publishing job checks the same before it
publishes. The proxy leaves `Origin` out of the `.info` of a version it
fetched long ago, so that version cannot be held to a commit: the audit and
the publishing job report it as unverified, a warning, and pass, unless the
`Time` in the `.info`, the time of the commit the proxy fetched, is not that
of the commit the tag names, which fails. That time is the committer date,
which whoever makes a commit chooses, so it catches a tag moved to another
commit of the history but not a commit made to match it: it is a
consistency check, not a proof. The tag of an unverified version is still
verified like any other, and its signature is what holds it to a commit.
A failed run of the job *Verify every release tag is signed* is the alarm:
its log names the tag or the version. Investigate it as a compromise, and
withdraw the version as below, which clears the alarm for what the
withdrawal answers. GitHub disables scheduled workflows in a repository
with no activity for 60 days; enable the workflow again when that happens.

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

Never move or delete a tag you pushed as a release: the module proxy and the
checksum database keep it regardless. There are two exceptions
([decision 10](decisions.md#10-a-daily-audit-holds-the-releases-to-their-record)).
A tag that was not pushed as a release, such as one an intruder moved, or
deleted and pushed again, is restored to the pinned tag object or the tag
the proxy fetched, or deleted, as the audit below asks. A bad tag of a
retracted version whose commit nothing records is deleted, even one you
pushed as a release, as below. To withdraw a version, add a `retract`
directive with the reason to `go.mod` and ship it in the next release:

```go
retract v0.2.0 // Tagged from the wrong commit.
```

The retraction also acknowledges the alarm of the daily audit, which reads
`go.mod` on `main`, so it takes effect as soon as it is merged. It
acknowledges what it withdraws, the content the module proxy serves under
the version, and nothing that happens to the tag later. For a version that
`go.mod` retracts on its own, the audit reports as a warning, and passes:

- a version the proxy serves without a tag, and a pinned release whose tag
  is gone;
- a tag that is neither pinned nor signed by a listed signer, while the
  proxy says, as `Origin.Hash`, that it serves the version from the commit
  the tag names.

It still fails a retracted version whose tag names another commit than the
proxy serves, such as a tag that was moved, or deleted and pushed again,
after the proxy fetched it, and a pinned release whose tag is not the
pinned tag object, whatever commit it names: the tag then names content
that nobody withdrew. Restore the pinned tag object for a pinned release,
or the tag the proxy fetched for one that is not pinned, or delete the
tag, which leaves a version without a tag. A bad tag whose version the
proxy does not record a commit for is not acknowledged either, since
nothing records which commit was withdrawn:

- if the proxy has not fetched the version, delete the tag, or, if the tag
  names the commit to withdraw, fetch the version through the proxy, which
  records the commit;
- if the proxy has fetched it but its `.info` has no `Origin`, delete the
  tag: the proxy goes on serving that `.info`, so fetching the version again
  records nothing.

Any version that `go.mod` does not retract still fails the audit. A range
such as `retract [v0.2.0, v0.2.3]` acknowledges none of its versions, so
retract each version the audit names on a line of its own. Never pin a bad
tag to silence the audit: a pin records a tag object that was verified.

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
