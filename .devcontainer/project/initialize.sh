#!/usr/bin/env bash
# Project hook — runs on the HOST after the kit's initialize.sh (every build/start).
# Owned by this repo: `copier update` never changes this file.
#
# Use for host-side preparation the kit doesn't cover, e.g. checking that a key file exists
# before a mount needs it, and failing loudly if it doesn't. macOS bash 3.2.
set -euo pipefail
