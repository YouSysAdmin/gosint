#!/bin/sh
# Gosint, Copyright (c) 2026 YouSysAdmin
#
# Builds the desktop app for THIS machine and packs it into dist/:
# a zipped gosint.app on macOS, a tar.gz and a .deb on Linux, a zip on
# Windows (run from Git Bash, MinGW gcc on PATH, go-winres installed). The
# window links the system web view through cgo, so there is no cross
# build, each platform is packed on its own.
#
# Expects the console in web/dist (task web). VERSION names the
# artifacts, 0.0.0-devel without one.
set -eu

version=${VERSION:-0.0.0-devel}
version=${version#v}
os=$(go env GOOS)
arch=$(go env GOARCH)
ldflags="-s -w -X github.com/yousysadmin/gosint/pkg.Version=${version}"
exe=bin/gosint-desktop

mkdir -p bin dist

# webview_go asks for webkit2gtk-4.0, the shim points it at 4.1.
if [ "$os" = linux ] && ! pkg-config --exists webkit2gtk-4.0; then
  PKG_CONFIG_PATH="$(pwd)/packaging/linux/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
  export PKG_CONFIG_PATH
fi

# Windows: the icon, version and a manifest asking for per monitor DPI
# (a blurry window otherwise) go in as a resource beside main.go, and
# windowsgui keeps a console window from opening with the app.
if [ "$os" = windows ]; then
  numeric=${version%%-*}
  go-winres simply --arch "$arch" --out cmd/gosint-desktop/rsrc --manifest gui \
    --icon packaging/windows/icon.png --product-name gosint --file-description "gosint desktop" \
    --original-filename gosint-desktop.exe --product-version "$numeric" --file-version "$numeric"
  trap 'rm -f cmd/gosint-desktop/rsrc_windows_*.syso' EXIT
  ldflags="$ldflags -H windowsgui"
  exe=bin/gosint-desktop.exe
fi

CGO_ENABLED=1 go build -trimpath -ldflags "$ldflags" -o "$exe" ./cmd/gosint-desktop

case "$os" in
darwin)
  app=dist/gosint.app
  rm -rf "$app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  cp bin/gosint-desktop "$app/Contents/MacOS/"
  sed "s/@VERSION@/${version}/g" packaging/macos/Info.plist >"$app/Contents/Info.plist"

  set_dir=$(mktemp -d)/gosint.iconset
  mkdir -p "$set_dir"
  for size in 16 32 128 256 512; do
    rsvg-convert -w "$size" -h "$size" packaging/icon.svg -o "$set_dir/icon_${size}x${size}.png"
    rsvg-convert -w $((size * 2)) -h $((size * 2)) packaging/icon.svg -o "$set_dir/icon_${size}x${size}@2x.png"
  done
  iconutil -c icns "$set_dir" -o "$app/Contents/Resources/gosint.icns"
  rm -rf "$(dirname "$set_dir")"

  # Ad hoc signature, Apple Silicon runs nothing unsigned.
  codesign --force --deep --sign - "$app"

  out="dist/gosint-desktop_v${version}_darwin_${arch}.zip"
  rm -f "$out"
  ditto -c -k --keepParent "$app" "$out"
  ;;
linux)
  stage=$(mktemp -d)/gosint-desktop
  mkdir -p "$stage"
  cp bin/gosint-desktop packaging/linux/gosint.desktop LICENSE README.md "$stage/"
  cp packaging/icon.svg "$stage/gosint.svg"
  tar -C "$(dirname "$stage")" -czf "dist/gosint-desktop_v${version}_linux_${arch}.tar.gz" gosint-desktop
  rm -rf "$(dirname "$stage")"

  VERSION=$version ARCH=$arch nfpm package -f packaging/linux/nfpm.yaml -p deb \
    -t "dist/gosint-desktop_v${version}_linux_${arch}.deb"
  ;;
windows)
  stage=$(mktemp -d)/gosint-desktop
  mkdir -p "$stage"
  cp "$exe" LICENSE README.md "$stage/"
  out="dist/gosint-desktop_v${version}_windows_${arch}.zip"
  rm -f "$out"
  powershell -NoProfile -Command "Compress-Archive -Path '$(cygpath -w "$stage")' -DestinationPath '$(cygpath -w "$out")'"
  rm -rf "$(dirname "$stage")"
  ;;
*)
  echo "desktop-package: $os is not packed" >&2
  exit 1
  ;;
esac

ls -l dist/gosint-desktop_* dist/gosint.app 2>/dev/null || true
