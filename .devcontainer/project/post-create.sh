#!/usr/bin/env bash
# Project hook — runs INSIDE the container after the kit's post-create.sh (once per container).
# Owned by this repo: `copier update` never changes this file.
#
# Use for one-time project setup: git hooks, extra dependency installs, seed data. Keep it
# idempotent — it runs again after every rebuild.
set -euo pipefail

# oban_job's Hex packages, installed once, online, so that its commands run offline (#110).
elixir examples/oban_job/src/deps.exs
