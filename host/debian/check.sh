#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
fail() { echo "FAIL: $*" >&2; exit 1; }

control="$root/host/debian/control"
[[ -f "$control" ]] || fail "missing $control"
grep -q 'Package: stagewand-host' "$control" || fail "debian control must name stagewand-host"
grep -q 'libqt6widgets6' "$control" || fail "debian control must depend on Qt 6 widgets"
grep -q 'libqt6core6t64' "$control" || fail "debian control must accept Ubuntu t64 Qt packages"
for dep in ydotool xdotool libei; do
  if grep -q "$dep" "$control"; then
    fail "debian control must not depend on $dep"
  fi
done

postinst="$root/host/debian/postinst"
[[ -x "$postinst" ]] || fail "debian postinst must be executable"
grep -q 'udevadm' "$postinst" || fail "debian postinst must reload udev"
grep -q 'uaccess' "$postinst" || fail "debian postinst must mention uaccess"

readme="$root/README.md"
grep -q 'stagewand-host_amd64.deb' "$readme" || fail "README must show the Debian/Ubuntu .deb download"
grep -q 'stagewand-host-x86_64.pkg.tar.zst' "$readme" || fail "README must show the Arch package download"
grep -q 'apt install ./stagewand-host' "$readme" || fail "README must show apt install of the .deb"
grep -q 'pacman -U ./stagewand-host' "$readme" || fail "README must show pacman -U of the release package"

echo "PASS: Debian packaging check"
