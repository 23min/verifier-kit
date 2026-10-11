#!/usr/bin/env bash
# devcontainer-kit — runs INSIDE the container once, after it is created (postCreateCommand).
# Identical in every kit repo; repo-specific steps go in .devcontainer/project/post-create.sh.
# Idempotent: every step is safe to re-run.
#
# The image carries the latest of every tool. This script wires git, then applies the repo's
# own version pins (each language's native version file) and installs its dependencies.
set -euo pipefail

cd "$(dirname "$0")/.."
# shellcheck source=devkit.conf
. .devcontainer/devkit.conf

step() { current_step=$*; printf '\n==> %s\n' "$*"; }
warn() { printf '    ! %s\n' "$*" >&2; }
uses() { case " $DEVKIT_LANGUAGES " in *" $1 "*) return 0 ;; esac; return 1; }

current_step=setup
# VS Code runs no postStartCommand after this script fails, and post-start is what installs
# Codex and aiwf and runs aiwf update. Say so where the failure is reported, with the recovery.
on_exit() {
  local rc=$?
  [ "$rc" -eq 0 ] && return
  {
    printf '\n!! post-create failed (exit %s) during: %s\n' "$rc" "$current_step"
    if [ "$DEVKIT_AIWF" = 1 ]; then
      printf '!! post-start will not run, so Codex and aiwf are not installed yet.\n'
    else
      printf '!! post-start will not run, so Codex is not installed yet.\n'
    fi
    printf '!! Fix the cause, then run: bash .devcontainer/post-create.sh && bash .devcontainer/post-start.sh\n'
  } >&2
}
trap on_exit EXIT

echo "==> ${DEVKIT_SLUG} devcontainer post-create"

# --- git --------------------------------------------------------------------------------------
# Probe paths can leave these pointing at host paths; the repo's own .git must be authoritative.
unset GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR

# Identity comes from the host's global git config (via .host.env). A repo that sets its own
# user.email in .git/config keeps it — repo config wins on host and container alike.
step "git identity"
if [ -n "${GIT_USER_NAME:-}" ]; then git config --global user.name "$GIT_USER_NAME"; fi
if [ -n "${GIT_USER_EMAIL:-}" ]; then git config --global user.email "$GIT_USER_EMAIL"; fi
echo "    $(git config --get user.name || echo '?') <$(git config --get user.email || echo '?')>"

# .git/config is shared with the host, so an absolute hooksPath only resolves on the side that
# wrote it. Report it rather than change a file the host also uses.
hooks_path=$(git config --get core.hooksPath || true)
case "$hooks_path" in
  /*) [ -d "$hooks_path" ] || warn "core.hooksPath=$hooks_path does not exist here; git hooks will not run in the container" ;;
esac

step "GitHub CLI as git credential helper"
# `gh auth token` reads the stored token; `gh auth status` would also check it against GitHub
# over the network, and one network blip at container creation would then leave git without a
# credential helper for the container's whole life (post-create never re-runs).
if gh auth token >/dev/null 2>&1; then
  gh auth setup-git
else
  warn "gh is not authenticated — run 'gh auth login' (the state is shared with the host), then 'gh auth setup-git'"
fi

# --- languages: pins + dependencies -----------------------------------------------------------
if uses python; then
  # uv reads .python-version itself; the venv lives outside the repo (UV_PROJECT_ENVIRONMENT).
  if [ -f pyproject.toml ]; then
    step "Python: uv sync"
    uv sync
  else
    step "Python: no pyproject.toml yet — creating an empty venv so the interpreter path resolves"
    uv venv --allow-existing "$UV_PROJECT_ENVIRONMENT"
  fi
fi

if uses node; then
  # nvm (from the node feature) reads .nvmrc itself.
  set +u
  # shellcheck source=/dev/null
  . /usr/local/share/nvm/nvm.sh
  if [ -f .nvmrc ]; then
    step "Node: installing the version pinned in .nvmrc"
    nvm install
    nvm alias default "$(nvm version)"
    nvm use default >/dev/null
  fi
  set -u
  if [ -f package-lock.json ]; then
    step "Node: npm ci"
    npm ci
  elif [ -f package.json ]; then
    step "Node: npm install"
    npm install
  fi
fi

if uses go && [ -f go.mod ]; then
  # A `toolchain` line in go.mod is honoured by Go itself (GOTOOLCHAIN=auto).
  step "Go: go mod download"
  go mod download
fi

if uses rust; then
  if [ -f rust-toolchain.toml ] || [ -f rust-toolchain ]; then
    step "Rust: installing the toolchain pinned in rust-toolchain(.toml)"
    rustup toolchain install
  fi
  if [ -f Cargo.toml ]; then
    step "Rust: cargo fetch"
    cargo fetch
  fi
fi

if uses elixir; then
  # .tool-versions pins (asdf/mise format): `erlang 28.1`, `elixir 1.19.5-otp-28`.
  otp_pin=$(awk '$1 == "erlang" { print $2 }' .tool-versions 2>/dev/null || true)
  elixir_pin=$(awk '$1 == "elixir" { print $2 }' .tool-versions 2>/dev/null || true)
  elixir_pin=${elixir_pin%%-otp-*}
  if [ -n "$otp_pin" ] || [ -n "$elixir_pin" ]; then
    step "Elixir: installing elixir@${elixir_pin:-latest} otp@${otp_pin:-latest} from .tool-versions"
    curl -fsSLo /tmp/elixir-install.sh https://elixir-lang.org/install.sh
    out=$(sh /tmp/elixir-install.sh "elixir@${elixir_pin:-latest}" "otp@${otp_pin:-latest}")
    rm -f /tmp/elixir-install.sh
    # install.sh ends by printing the two PATH lines for what it installed; point the fixed
    # links (already on PATH) at those directories.
    otp_dir=$(printf '%s\n' "$out" | sed -n 's|.*\(installs/otp/[^/]*\)/bin.*|\1|p' | tail -1)
    elixir_dir=$(printf '%s\n' "$out" | sed -n 's|.*\(installs/elixir/[^/]*\)/bin.*|\1|p' | tail -1)
    ln -sfn "$HOME/.elixir-install/$otp_dir" "$HOME/.elixir-install/otp"
    ln -sfn "$HOME/.elixir-install/$elixir_dir" "$HOME/.elixir-install/elixir"
  fi
  mix local.hex --force --if-missing
  mix local.rebar --force --if-missing
  if [ -f mix.exs ]; then
    step "Elixir: mix deps.get"
    mix deps.get
  fi
fi

if uses dotnet && [ -f global.json ]; then
  step ".NET: installing the SDK pinned in global.json"
  curl -fsSLo /tmp/dotnet-install.sh https://dot.net/v1/dotnet-install.sh
  bash /tmp/dotnet-install.sh --jsonfile global.json --install-dir "$HOME/.dotnet"
  rm -f /tmp/dotnet-install.sh
fi

# --- project hook -----------------------------------------------------------------------------
if [ -f .devcontainer/project/post-create.sh ]; then
  step "project post-create"
  bash .devcontainer/project/post-create.sh
fi

# --- versions ---------------------------------------------------------------------------------
step "tool versions"
v() { printf '    %-8s %s\n' "$1" "$("${@:2}" 2>&1 | head -1 || echo -)"; }
v git git --version
v gh gh --version
v uv uv --version
v claude claude --version
if uses python; then v python "$UV_PROJECT_ENVIRONMENT/bin/python" --version; v ruff ruff --version; fi
if uses node; then v node node --version; fi
if uses go || [ "$DEVKIT_AIWF" = 1 ]; then v go go version; fi
if uses rust; then v rustc rustc --version; fi
if uses elixir; then v elixir elixir --short-version; v erlang erl -noshell -eval 'io:put_chars(erlang:system_info(otp_release)), halt().'; fi
if uses dotnet; then v dotnet dotnet --version; fi

echo
echo "==> ${DEVKIT_SLUG} devcontainer ready (post-start installs/refreshes Codex and aiwf)."
