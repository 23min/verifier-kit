#!/usr/bin/env bash
# devcontainer-kit — runs on the HOST (macOS) via initializeCommand, before every build/start.
# Identical in every kit repo; repo-specific host steps go in .devcontainer/project/initialize.sh.
# Written for macOS's bash 3.2.
#
#   1. Create mount sources before Docker does (a missing bind source is created root-owned).
#   2. Point the project-named /tmp/.<slug>-* links at them. The mounts use these links because
#      $HOME is always right in this host shell, and the <slug> prefix keeps two open repos from
#      re-pointing each other's links.
#   3. Write .host.env: the host's global git identity, passed into the container.
#   4. Once per ISO week: re-pull the upstream images and bump .refresh, so the image's tool
#      layers rebuild with the latest versions on the next build.
#   5. Personal guidance: rebuild ~/.guidance's AGENTS.md and point the container-only Codex
#      home at it.
set -euo pipefail

cd "$(dirname "$0")/.."
# shellcheck source=devkit.conf
. .devcontainer/devkit.conf

slug=$DEVKIT_SLUG
link() { ln -sfn "$1" "/tmp/.${slug}-$2"; }

# --- 1 + 2. mount sources and links ------------------------------------------------------------
# ~/.claude-linux/plugins: container-only plugin index (anthropics/claude-code#31388 — the index
# stores absolute paths, so macOS and Linux need separate ones). ~/.codex-linux: Codex state and
# its standalone install, shared by all containers and kept apart from the Mac's ~/.codex.
# ~/.guidance: personal assistant rules, read-write so a rule can be changed from any container.
mkdir -p "$HOME/.claude" "$HOME/.claude-linux/plugins" "$HOME/.config/gh" "$HOME/.codex-linux" \
  "$HOME/.guidance"
chmod 700 "$HOME/.codex-linux"
link "$HOME/.claude" claude
link "$HOME/.claude-linux/plugins" claude-plugins
link "$HOME/.config/gh" gh
link "$HOME/.codex-linux" codex
link "$HOME/.guidance" guidance

if [ "$DEVKIT_DATA_DIR" = 1 ]; then
  mkdir -p "$HOME/ProjectData/${slug}-dev"
  link "$HOME/ProjectData/${slug}-dev" data
fi

# Other projects' data, mounted read-only. Never created here: a missing folder means the
# producing project hasn't run (or the path is wrong), and should fail loudly.
for path in $DEVKIT_READONLY_DATA; do
  if [ ! -d "$HOME/ProjectData/$path" ]; then
    echo "devcontainer: read-only data $HOME/ProjectData/$path not found (listed in devkit.conf)." >&2
    exit 1
  fi
  link "$HOME/ProjectData/$path" "ro-${path//\//--}"
done

parent=$(cd .. && pwd)
for sibling in $DEVKIT_SIBLINGS; do
  if [ ! -d "$parent/$sibling" ]; then
    echo "devcontainer: sibling repo $parent/$sibling not found (listed in devkit.conf)." >&2
    exit 1
  fi
  link "$parent/$sibling" "sibling-$sibling"
done

# --- 3. host git identity ---------------------------------------------------------------------
# docker --env-file format: KEY=value, taken literally (no quotes).
{
  printf 'GIT_USER_NAME=%s\n' "$(git config --global --get user.name || true)"
  printf 'GIT_USER_EMAIL=%s\n' "$(git config --global --get user.email || true)"
} > .devcontainer/.host.env

# --- 4. weekly refresh ------------------------------------------------------------------------
week=$(date +%G-W%V)
if [ "$(cat .devcontainer/.refresh 2>/dev/null || true)" != "$week" ]; then
  echo "devcontainer: weekly refresh ($week) — pulling upstream images"
  for image in $DEVKIT_PULL_IMAGES; do
    docker pull --quiet "$image" >/dev/null \
      || echo "devcontainer: could not pull $image; building with the cached copy" >&2
  done
  printf '%s\n' "$week" > .devcontainer/.refresh
fi

# --- 5. personal guidance ---------------------------------------------------------------------
# ~/.guidance is a checkout of 23min/guidance. Claude reaches it through the shared ~/.claude, so
# only the container-only Codex home needs a link, and it is made only once a build has produced
# ~/.guidance/AGENTS.md, so the AGENTS.md there keeps working until then. A missing checkout or a
# failing build never blocks a start.
if [ -x "$HOME/.guidance/build" ]; then
  "$HOME/.guidance/build" \
    || echo "devcontainer: $HOME/.guidance/build failed; its previous output, if any, stays in use" >&2
else
  echo "devcontainer: no personal guidance in $HOME/.guidance. Set it up with:" \
    "git clone https://github.com/23min/guidance.git $HOME/.guidance && $HOME/.guidance/install" >&2
fi
codex_rules=$HOME/.codex-linux/AGENTS.md
if [ -e "$HOME/.guidance/AGENTS.md" ]; then
  if [ -L "$codex_rules" ] || [ ! -e "$codex_rules" ]; then
    ln -sfn ../.guidance/AGENTS.md "$codex_rules"
  else
    echo "devcontainer: $codex_rules exists and is not a link, so Codex in containers" \
      "doesn't read $HOME/.guidance; move it aside to use it" >&2
  fi
fi

# --- project hook -----------------------------------------------------------------------------
if [ -f .devcontainer/project/initialize.sh ]; then
  bash .devcontainer/project/initialize.sh
fi
