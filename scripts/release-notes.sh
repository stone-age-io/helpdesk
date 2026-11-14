#!/usr/bin/env bash
# Build the GitHub release body for a tag, and print it to stdout.
#
# goreleaser's own changelog is a list of commit subjects. That is the wrong
# document for this repo: the reasoning lives in CHANGELOG.md, entries there are
# paragraphs rather than one-liners, and the upgrade notes -- the part an
# operator actually needs before running `migrate up` -- exist nowhere in the
# commit log. So the release body is the changelog section for the tag, wrapped
# in the install prose that used to sit in .goreleaser.yaml as `release.header`
# and `release.footer`.
#
# Those two keys are gone from .goreleaser.yaml deliberately. goreleaser's
# behaviour when `--release-notes` is combined with a header/footer is not
# something this repo should have to know: assembling the whole body here means
# there is one definition of it, and it can be read and diffed before the tag is
# pushed. `changelog.disable: true` is set there for the same reason -- a body
# assembled two ways is a body that can disagree with itself.
#
# Usage:  scripts/release-notes.sh v0.5.0
#
# A FINAL tag requires its own `## [X.Y.Z]` section and fails without one, so a
# release cannot ship with an empty body because someone forgot to promote
# `## [Unreleased]`. A PRERELEASE tag (v0.5.1-rc1) reads `## [Unreleased]`
# instead, because that is where the content for an unreleased version is by
# definition -- an rc has no section of its own and never will.
set -euo pipefail

TAG="${1:-}"
if [ -z "$TAG" ]; then
  echo "usage: $0 <tag>   e.g. $0 v0.5.0" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHANGELOG="$ROOT/CHANGELOG.md"

VERSION="${TAG#v}"
case "$TAG" in
  *-*) HEADING='## [Unreleased]' ; KIND='prerelease' ;;
  *)   HEADING="## [$VERSION]"   ; KIND='release' ;;
esac

# Everything between this heading and the next `## ` heading, with surrounding
# blank lines trimmed. awk rather than sed: the section is hundreds of lines and
# contains `#` inside fenced code blocks, so the boundary has to be anchored to
# the start of a line and to the two-hash level exactly.
SECTION="$(
  awk -v want="$HEADING" '
    index($0, want) == 1 && substr($0, length(want) + 1, 1) ~ /^[ ]?$/ { grab = 1; next }
    grab && /^## / { exit }
    grab { print }
  ' "$CHANGELOG"
)"

# Trim leading and trailing blank lines.
SECTION="$(printf '%s\n' "$SECTION" | sed -e '/./,$!d' -e ':a' -e '/^\n*$/{$d;N;ba' -e '}')"

if [ -z "$(printf '%s' "$SECTION" | tr -d '[:space:]')" ]; then
  echo "release-notes: no content under '$HEADING' in CHANGELOG.md" >&2
  echo "release-notes: a $KIND tag needs that section filled in before tagging." >&2
  exit 1
fi

cat <<EOF
## Stone Age Helpdesk $TAG

One binary: **\`helpdesk\`**. One process: the REST API, the embedded Vue SPA
(staff app, requester portal and field shell), SQLite, and the NATS client that
turns machine-published events into tickets.

It is a standalone sibling of the control plane, not a feature of it. It holds
no platform credentials and joins to platform data by organization code.

Or run the container, which needs no toolchain at all:

\`\`\`
docker run -d -p 8090:8090 \
  -v helpdesk-data:/data \
  ghcr.io/stone-age-io/helpdesk:$TAG
\`\`\`

Nothing is required on first boot: it seeds a staff admin
(\`admin@helpdesk.local\`) and prints the password ONCE, so read it with
\`docker logs\` and change it after signing in. SMTP and the application URL
are PocketBase settings, set in the dashboard at \`/_\` afterwards.

---

$SECTION

---

Pre-1.0: treat a minor version as potentially breaking, and pin what you
deploy. CHANGELOG.md says which surfaces are worth pinning against.

**Full changelog**: https://github.com/stone-age-io/helpdesk/commits/$TAG
EOF
