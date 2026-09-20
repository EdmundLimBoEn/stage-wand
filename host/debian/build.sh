#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
pkgver=$(awk -F= '/^pkgver=/ {print $2; exit}' "$root/host/arch/PKGBUILD")
pkgrel=$(awk -F= '/^pkgrel=/ {print $2; exit}' "$root/host/arch/PKGBUILD")
version="${pkgver}-${pkgrel}"
arch=$(dpkg --print-architecture)
outdir="${1:-$root/host/debian}"
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

stage="$workdir/pkg"
mkdir -p "$stage/DEBIAN" "$workdir/build" "$workdir/go" "$workdir/cache"

export GOPATH="$workdir/go"
export GOCACHE="$workdir/cache"
export CGO_ENABLED=1
cd "$root/host"
go mod download -modcacherw
GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw -buildvcs=false" \
  go build -ldflags "-linkmode=external" -o "$workdir/build/stagewand-host" ./cmd/stagewand-host

cmake -S "$root/host/desktop" -B "$workdir/desktop-build" -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=/usr
cmake --build "$workdir/desktop-build"
ctest --test-dir "$workdir/desktop-build" --output-on-failure
GOFLAGS="-mod=readonly -modcacherw -buildvcs=false" go test ./internal/... ./cmd/...

DESTDIR="$stage" cmake --install "$workdir/desktop-build"
install -Dm755 "$workdir/build/stagewand-host" "$stage/usr/bin/stagewand-host"
install -Dm644 "$root/host/udev/70-stagewand-uinput.rules" "$stage/usr/lib/udev/rules.d/70-stagewand-uinput.rules"
install -Dm644 "$root/host/modules-load.d/uinput.conf" "$stage/usr/lib/modules-load.d/stagewand-uinput.conf"

sed -e "s/@VERSION@/${version}/" -e "s/@ARCH@/${arch}/" \
  "$root/host/debian/control" > "$stage/DEBIAN/control"
install -Dm755 "$root/host/debian/postinst" "$stage/DEBIAN/postinst"

mkdir -p "$outdir"
deb="$outdir/stagewand-host_${version}_${arch}.deb"
dpkg-deb --root-owner-group --build "$stage" "$deb"
cp -f "$deb" "$outdir/stagewand-host_${arch}.deb"
echo "built $deb"
