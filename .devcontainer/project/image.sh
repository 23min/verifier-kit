#!/usr/bin/env bash
# Project hook — runs during the image build, as the last layer, as vscode (use sudo for root).
# Owned by this repo: `copier update` never changes this file.
#
# Use for image-level installs this repo needs beyond the apt_packages answer, e.g. a CLI from
# a release tarball. Keep it deterministic; anything that should track "latest" belongs in
# post-start.sh instead. Changing this file rebuilds only this layer.
set -euo pipefail
