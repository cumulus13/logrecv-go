#!/usr/bin/env bash
# Usage: build-release.sh <version> <outdir> [targets-file]
# Builds static, stripped binaries for every target in the targets file and
# packages them as .tar.gz (zip for windows) into <outdir>.
set -euo pipefail

version="${1:?usage: build-release.sh <version> <outdir> [targets-file]}"
out="${2:?usage: build-release.sh <version> <outdir> [targets-file]}"
targets="${3:-.github/targets.txt}"

export CGO_ENABLED=0
mkdir -p "$out"
out="$(cd "$out" && pwd)"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

count=0
while read -r goos goarch extra label; do
  [[ -z "${goos:-}" || "$goos" == \#* ]] && continue
  name="logrecv_${version}_${goos}_${label}"
  bin="logrecv"
  [[ "$goos" == "windows" ]] && bin="logrecv.exe"

  dir="$stage/$name"
  mkdir -p "$dir"

  env_args=("GOOS=$goos" "GOARCH=$goarch")
  [[ "$extra" != "-" ]] && env_args+=("$extra")

  echo "==> $name"
  env "${env_args[@]}" go build -trimpath \
    -ldflags "-s -w -X main.version=${version}" \
    -o "$dir/$bin" .

  for f in README.md LICENSE; do
    [[ -f "$f" ]] && cp "$f" "$dir/"
  done

  if [[ "$goos" == "windows" ]]; then
    (cd "$stage" && zip -qr "$out/$name.zip" "$name")
  else
    tar -C "$stage" -czf "$out/$name.tar.gz" "$name"
  fi
  rm -rf "$dir"
  count=$((count + 1))
done < "$targets"

echo "built $count archives in $out"
