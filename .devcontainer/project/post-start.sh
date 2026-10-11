#!/usr/bin/env bash
# Project hook — runs INSIDE the container after the kit's post-start.sh (every start).
# Owned by this repo: `copier update` never changes this file.
#
# Use for things that must be true on every start, e.g. a safety check on a data path or
# bringing up a VPN. A failure here is reported but does not stop the container.
set -euo pipefail
