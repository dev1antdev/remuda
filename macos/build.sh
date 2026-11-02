#!/bin/sh
# build.sh: the Mac app as bin/remuda.app — `swift build` in release, the bundle laid out, the icon made from the
# page's own, an ad-hoc signature so the system runs it from here. Run from anywhere: `sh macos/build.sh` or `make mac`.
set -e
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)
version=$(git -C "$root" describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
[ -n "$version" ] || version=dev
build=$(git -C "$root" rev-list --count HEAD 2>/dev/null || echo 1)
app=$root/bin/remuda.app
source=$root/web/public/icons/icon-512.png
if [ ! -f "$source" ]; then
  echo "the icon is missing: $source (the page's own icon); nothing was built or removed"
  exit 1
fi

cd "$here"
log=$(mktemp)
if ! swift build -c release >"$log" 2>&1; then
  grep -v '^\[' "$log"
  rm -f "$log"
  echo "swift build failed"
  exit 1
fi
grep -v '^\[' "$log" || true
rm -f "$log"
test -x .build/release/Remuda || { echo "swift build failed"; exit 1; }

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp .build/release/Remuda "$app/Contents/MacOS/remuda"
sed -e "s/@VERSION@/$version/" -e "s/@BUILD@/$build/" Info.plist > "$app/Contents/Info.plist"
printf 'APPL????' > "$app/Contents/PkgInfo"

iconset=$(mktemp -d)/remuda.iconset
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$source" --out "$iconset/icon_${size}x${size}.png" >/dev/null
done
for size in 16 32 128 256 512; do
  double=$((size * 2))
  sips -z "$double" "$double" "$source" --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/remuda.icns"
rm -rf "$(dirname "$iconset")"

if ! signed=$(codesign --force --sign - "$app" 2>&1); then
  printf '%s\n' "$signed"
  rm -rf "$app"
  echo "codesign failed; the unsigned bundle was removed"
  exit 1
fi
printf '%s\n' "$signed" | grep -v 'replacing existing signature' || true
plutil -lint "$app/Contents/Info.plist" >/dev/null
echo "$app ($version)"
