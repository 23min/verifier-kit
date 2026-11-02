#!/usr/bin/env bash
# Installs verifier-kit into the project in the current folder, or updates it: `verify` at the
# project's root, and the toolkit in .verifier-kit/ beside it. Run it in the project's root:
#
#   curl -fsSL https://raw.githubusercontent.com/23min/verifier-kit/main/install.sh | bash
#   curl -fsSL .../install.sh | bash -s -- v0.2.0    # a release or a commit, not the latest release
#   bash install.sh --from DIR                        # from a checkout of the toolkit, offline
#
# It needs Bash, curl, tar and git, and downloads only verifier-kit, from GitHub. It changes
# nothing but `verify` and .verifier-kit/, and refuses when either is already there and isn't one
# it installed. The release or commit it installed is in .verifier-kit/VERSION. The whole script
# is main, run on its last line, so a download cut short runs nothing.
set -euo pipefail

repo=https://github.com/23min/verifier-kit
archive=https://codeload.github.com/23min/verifier-kit/tar.gz
kit=.verifier-kit
usage="usage: install.sh [vX.Y.Z | COMMIT | --from DIR]"
tmp='' stage='' done=''

fail() {
  printf 'install: %s\n' "$1" >&2
  exit "${2:-1}"
}

# cleanup puts the old .verifier-kit/ back unless the install finished, and removes what the
# install made for itself.
cleanup() {
  if [ -z "$done" ] && [ -n "$stage" ] && [ -d "$stage/old" ]; then
    rm -rf "$kit"
    mv "$stage/old" "$kit"
  fi
  rm -rf ${tmp:+"$tmp"} ${stage:+"$stage"}
}

main() {
  unset CDPATH
  local from='' want='' src version commit tags refs id f was
  case "$#:${1:-}" in
    0:) ;;
    2:--from) from=$2 ;;
    1:*) want=$1 ;;
    *) fail "$usage" 2 ;;
  esac
  if [ "$#" = 2 ] && [ -z "$from" ]; then
    fail "$usage" 2
  fi
  if [ -n "$want" ] && ! [[ $want =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ || $want =~ ^[0-9a-f]{7,40}$ ]]; then
    fail "$usage" 2
  fi

  if [ ! -f "$kit/VERSION" ]; then
    [ ! -e "$kit" ] || fail "$kit/ is here already, and verifier-kit didn't install it; nothing changed"
    [ ! -e verify ] || fail "verify is here already, and verifier-kit didn't install it; nothing changed"
  fi
  [ ! -e verify ] || [ -f verify ] || fail "verify is here, and isn't a file; nothing changed"

  trap cleanup EXIT
  trap 'exit 130' INT TERM
  tmp=$(mktemp -d)
  if [ -n "$from" ]; then
    [ -f "$from/verify" ] && [ -d "$from/tools" ] || fail "$from is not a checkout of verifier-kit"
    src=$(cd "$from" && pwd -P)
    version="checkout $src"
  else
    for f in curl tar gzip git; do
      command -v "$f" >/dev/null || fail "needs $f"
    done
    if [ -z "$want" ]; then
      tags=$(git ls-remote --tags --refs "$repo" 'v*') || fail "cannot list the releases at $repo"
      want=$(printf '%s\n' "$tags" | sed -n 's|.*refs/tags/v\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)$|\1|p' |
        sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1)
      [ -n "$want" ] || fail "verifier-kit has no release yet; name a commit"
      want=v$want
    fi
    if [[ $want == v* ]]; then
      refs=$(git ls-remote "$repo" "refs/tags/$want" "refs/tags/$want^{}") || fail "cannot list the releases at $repo"
      commit=$(printf '%s\n' "$refs" | tail -n 1 | cut -f1) # the commit a tag points to comes last
      [ -n "$commit" ] || fail "verifier-kit has no release $want"
    else
      commit=$want
    fi
    curl -fsSL -o "$tmp/kit.tar.gz" "$archive/$commit" || fail "cannot download verifier-kit at $commit"
    gzip -dc "$tmp/kit.tar.gz" >"$tmp/kit.tar" || fail "cannot unpack verifier-kit at $commit"
    mkdir "$tmp/kit"
    tar -xf "$tmp/kit.tar" -C "$tmp/kit" || fail "cannot unpack verifier-kit at $commit"
    id=$(git get-tar-commit-id <"$tmp/kit.tar") || id=$commit # the full commit, from the archive
    if [[ $want == v* ]]; then
      version="$want $id"
    else
      version=$id
    fi
    src=$(printf '%s\n' "$tmp"/kit/*)
    [ -f "$src/verify" ] && [ -d "$src/tools" ] || fail "verifier-kit at $commit has no verify and tools/"
  fi

  # Made beside the project's files, so that the moves below are renames.
  stage=$(mktemp -d "./$kit.XXXXXX")
  mkdir "$stage/kit"
  # tools/ without its tests, their inputs, its built programs and Python's caches
  (cd "$src/tools" && find . \( -name testdata -o -name bin -o -name __pycache__ \) -prune -o -type f ! -name '*_test.go' -print) |
    while IFS= read -r f; do
      mkdir -p "$stage/kit/$(dirname "$f")"
      cp -p "$src/tools/$f" "$stage/kit/$f"
    done
  printf '%s\n' "$version" >"$stage/kit/VERSION"
  printf 'bin/\n__pycache__/\n' >"$stage/kit/.gitignore"
  cp -p "$src/verify" "$stage/verify"

  was=''
  [ ! -f "$kit/VERSION" ] || was=$(cat "$kit/VERSION")
  [ ! -e "$kit" ] || mv "$kit" "$stage/old"
  mv "$stage/kit" "$kit"
  mv "$stage/verify" verify
  done=1
  if [ -z "$was" ]; then
    printf 'installed verifier-kit %s: ./verify, and %s/\n' "$version" "$kit"
  elif [ "$was" = "$version" ]; then
    printf 'verifier-kit %s is installed already; installed it again\n' "$version"
  else
    printf 'updated verifier-kit from %s to %s\n' "$was" "$version"
  fi
}

main "$@"
