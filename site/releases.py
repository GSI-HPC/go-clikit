# SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
# SPDX-License-Identifier: Apache-2.0
#
# Writes the releases of the kit, newest first, as JSON for the page of the
# releases: the signed tags, whose messages are the release notes
# (doc/release.md). Nothing in the tree names a version (doc/decisions.md,
# decisions 4 and 48), so make site and the Pages workflow write
# data/releases.json from the tags as they build.

import json
import subprocess
import sys

FIELDS = ["tag", "date", "title", "notes"]
FORMAT = "%00".join(["%(refname:short)", "%(creatordate:short)", "%(contents:subject)", "%(contents:body)"]) + "%00%00"

out = subprocess.run(
    ["git", "for-each-ref", "--sort=-v:refname", "--format=" + FORMAT, "refs/tags/v*"],
    capture_output=True, text=True, check=True,
).stdout
releases = []
for record in out.split("\0\0\n"):
    if record.strip():
        releases.append(dict(zip(FIELDS, (part.strip() for part in record.split("\0")))))
json.dump(releases, sys.stdout, indent=1)
sys.stdout.write("\n")
