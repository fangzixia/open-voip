#!/usr/bin/env bash
# Build the single native media engine and retain auditable dependency records.
set -euo pipefail

test "$(uname -s)" = Linux || { echo 'Linux/WSL is required' >&2; exit 1; }
switch_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$switch_root"
output_dir=${1:-"$switch_root/build/linux"}
mkdir -p -- "$output_dir/licenses"
output_dir=$(cd -- "$output_dir" && pwd)
export CGO_ENABLED=1
pkg-config --exists soxr spandsp

go build -trimpath -o "$output_dir/open-switch" ./cmd/open-switch
go version -m "$output_dir/open-switch" > "$output_dir/build-info.txt"
go list -m -json all > "$output_dir/go-modules.json"
{
  date -u +'%Y-%m-%dT%H:%M:%SZ'
  uname -a
  go version
  printf 'soxr='
  pkg-config --modversion soxr
  printf 'spandsp='
  pkg-config --modversion spandsp
  pkg-config --cflags --libs soxr spandsp
  ldd "$output_dir/open-switch"
} > "$output_dir/native-build.txt"

# Keep each module's original license/notice, including the controlled SDK.
while IFS='|' read -r module_name module_dir; do
  test -n "$module_dir" || continue
  license_dir="$output_dir/licenses/${module_name//\//_}"
  mkdir -p -- "$license_dir"
  find "$module_dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \) \
    -exec cp -- '{}' "$license_dir/" \;
done < <(go list -m -f '{{.Path}}|{{.Dir}}' all)

# Ubuntu/Debian native packages provide their complete upstream declarations.
for package in libsoxr-dev libspandsp-dev; do
  copyright_file="/usr/share/doc/$package/copyright"
  test -f "$copyright_file" || { echo "Missing native license: $copyright_file" >&2; exit 1; }
  cp -- "$copyright_file" "$output_dir/licenses/$package.copyright"
done
cp -- third_party/media-sdk/OPEN_VOIP_PATCHES.md "$output_dir/media-sdk-patches.md"
sha256sum "$output_dir/open-switch" > "$output_dir/SHA256SUMS"
printf 'Built %s\n' "$output_dir/open-switch"
