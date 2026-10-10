#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
for tool in go cmake ctest dpkg dpkg-deb dpkg-shlibdeps strip; do
  command -v "$tool" >/dev/null || { echo "Missing build tool: $tool" >&2; exit 1; }
done
pkgver=$(awk -F= '/^pkgver=/ {print $2; exit}' "$root/host/arch/PKGBUILD")
pkgrel=$(awk -F= '/^pkgrel=/ {print $2; exit}' "$root/host/arch/PKGBUILD")
version="${pkgver}-${pkgrel}"
if git -C "$root" rev-parse --verify HEAD >/dev/null 2>&1; then
  version+="+git$(git -C "$root" show -s --format=%ct HEAD).$(git -C "$root" rev-parse --short=12 HEAD)"
  export SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$root" show -s --format=%ct HEAD)}"
fi
version="${STAGEWAND_PACKAGE_VERSION:-$version}"
dpkg --validate-version "$version"
arch=$(dpkg --print-architecture)
outdir=$(realpath -m "${1:-$root/host/debian}")
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

stage="$workdir/pkg"
mkdir -p "$stage/DEBIAN" "$workdir/build"

export CGO_ENABLED=1
cd "$root/host"
go mod download
GOFLAGS="-buildmode=pie -trimpath -mod=readonly -buildvcs=false" \
  go build -ldflags "-linkmode=external" -o "$workdir/build/stagewand-host" ./cmd/stagewand-host

cmake -S "$root/host/desktop" -B "$workdir/desktop-build" -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=/usr
cmake --build "$workdir/desktop-build" --parallel "${CMAKE_BUILD_PARALLEL_LEVEL:-2}"
ctest --test-dir "$workdir/desktop-build" --output-on-failure
GOFLAGS="-mod=readonly -buildvcs=false" go test ./internal/... ./cmd/... .

DESTDIR="$stage" cmake --install "$workdir/desktop-build"
install -Dm755 "$workdir/build/stagewand-host" "$stage/usr/bin/stagewand-host"
install -Dm644 "$root/host/udev/70-stagewand-uinput.rules" "$stage/usr/lib/udev/rules.d/70-stagewand-uinput.rules"
install -Dm644 "$root/host/modules-load.d/uinput.conf" "$stage/usr/lib/modules-load.d/stagewand-uinput.conf"
strip --strip-unneeded "$stage/usr/bin/stagewand" "$stage/usr/bin/stagewand-host"

# Use the linked symbols, including glibc and Qt, rather than claiming an older ABI.
mkdir -p "$workdir/debian"
printf 'Source: stagewand\n\nPackage: stagewand-host\nArchitecture: any\n' > "$workdir/debian/control"
shlibs=$(cd "$workdir" && dpkg-shlibdeps -O -e"$stage/usr/bin/stagewand" -e"$stage/usr/bin/stagewand-host")
shlibs=${shlibs#shlibs:Depends=}
[[ -n "$shlibs" && "$shlibs" != *$'\n'* ]] || { echo "Cannot determine runtime dependencies" >&2; exit 1; }
installed_size=$(du -sk "$stage/usr" | cut -f1)
sed -e "s/@VERSION@/${version}/" -e "s/@ARCH@/${arch}/" \
  -e "s/@SHLIBS@/${shlibs}/" -e "s/@INSTALLED_SIZE@/${installed_size}/" \
  "$root/host/debian/control" > "$stage/DEBIAN/control"
install -Dm755 "$root/host/debian/postinst" "$stage/DEBIAN/postinst"
install -Dm755 "$root/host/debian/postrm" "$stage/DEBIAN/postrm"
(cd "$stage" && find usr -type f -print0 | LC_ALL=C sort -z | xargs -0 md5sum > DEBIAN/md5sums)

mkdir -p "$outdir"
deb="$outdir/stagewand-host_${version}_${arch}.deb"
dpkg-deb --root-owner-group --uniform-compression -Zxz --build "$stage" "$deb"
cp -f "$deb" "$outdir/stagewand-host_${arch}.deb"
echo "built $deb"
