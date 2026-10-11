#!/usr/bin/env bash
# devcontainer-kit — runs INSIDE the container at every start (postStartCommand).
# Identical in every kit repo; repo-specific steps go in .devcontainer/project/post-start.sh.
#
# Refreshes the assistant tooling that should always be current: Claude Code, Codex, aiwf.
# Nothing here may block a start: failures (e.g. offline) are reported and skipped.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
# shellcheck source=devkit.conf
. .devcontainer/devkit.conf

step() { printf '\n==> %s\n' "$*"; }
warn() { printf '    ! %s\n' "$*" >&2; }

echo "==> ${DEVKIT_SLUG} devcontainer post-start"

# --- git --------------------------------------------------------------------------------------
# Docker Desktop's VirtioFS can briefly report the workspace as root-owned after an idle spell,
# and git then refuses the repo as "dubious ownership". The container holds only our own repos.
step "git: trust /workspaces/*"
git config --global --get-all safe.directory 2>/dev/null | grep -qxF '/workspaces/*' \
  || git config --global --add safe.directory '/workspaces/*' \
  || warn "could not set git safe.directory"

# --- Claude Code ------------------------------------------------------------------------------
step "Claude Code: update"
claude update >/dev/null 2>&1 || warn "claude update failed; keeping $(claude --version 2>/dev/null || echo 'the installed version')"

# --- Codex ------------------------------------------------------------------------------------
# The standalone installer keeps Codex under $CODEX_HOME (~/.codex = the host's ~/.codex-linux,
# shared by all containers) and links ~/.local/bin/codex. Reinstall only when not current.
step "Codex: install/update"
latest=$(curl -fsS --max-time 10 https://api.github.com/repos/openai/codex/releases/latest 2>/dev/null \
  | jq -r '.tag_name // empty' | sed 's/^rust-v//; s/^v//')
current=$(codex --version 2>/dev/null | awk '{ print $NF }')
if [ -n "$current" ] && { [ -z "$latest" ] || [ "$current" = "$latest" ]; }; then
  echo "    codex $current${latest:+ (latest)}"
elif curl -fsSL https://chatgpt.com/codex/install.sh \
    | CODEX_NON_INTERACTIVE=1 sh -s -- --release "${latest:-latest}" >/dev/null; then
  echo "    codex $(codex --version 2>/dev/null | awk '{ print $NF }')"
else
  warn "Codex install failed; it is retried at the next start"
fi

# --- aiwf -------------------------------------------------------------------------------------
if [ "$DEVKIT_AIWF" = 1 ]; then
  # aiwf publishes no release binaries yet, so it is built from source at the latest tag.
  step "aiwf: install @latest"
  go install github.com/23min/aiwf/cmd/aiwf@latest || warn "go install failed; keeping the installed aiwf"
  if ! command -v aiwf >/dev/null 2>&1; then
    warn "aiwf is not on PATH"
  elif [ -f aiwf.yaml ]; then
    # Initialised repos stay current: refreshes skills, hooks and selected guidance packs
    # (may change tracked files — review and commit them). stdin from /dev/null: lifecycle
    # commands can run on a pseudo-terminal nobody watches, where an undecided hook or guidance
    # prompt would hang while holding the repo lock (aiwf G-0708). Undecided choices stay
    # undecided; `aiwf doctor` lists them.
    step "aiwf update"
    aiwf update </dev/null || warn "aiwf update failed"
  else
    echo "    aiwf $(aiwf version 2>/dev/null) — repo not initialised (no aiwf.yaml); run 'aiwf init' deliberately"
  fi
fi

# --- project hook -----------------------------------------------------------------------------
if [ -f .devcontainer/project/post-start.sh ]; then
  step "project post-start"
  bash .devcontainer/project/post-start.sh || warn "project post-start failed"
fi
