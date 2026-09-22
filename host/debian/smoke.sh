#!/usr/bin/env bash
set -euo pipefail

if [[ $EUID -ne 0 || ( ! -f /.dockerenv && ! -f /run/.containerenv ) ]]; then
  echo "Run this install/upgrade/remove test as root inside a disposable container." >&2
  exit 1
fi
root=$(cd "$(dirname "$0")/../.." && pwd)
deb=$(realpath "${1:?Usage: smoke.sh path/to/stagewand-host.deb}")
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends "$deb" dbus-daemon python3 util-linux weston xvfb xauth

for path in /usr/bin/stagewand /usr/bin/stagewand-host \
  /usr/share/applications/systems.edmundlim.StageWand.desktop \
  /usr/share/icons/hicolor/scalable/apps/stagewand.svg \
  /usr/lib/udev/rules.d/70-stagewand-uinput.rules \
  /usr/lib/modules-load.d/stagewand-uinput.conf; do
  test -f "$path"
done
dpkg-query -W -f='${Status}\n' stagewand-host | grep -qx 'install ok installed'
dpkg --verify stagewand-host
dpkg-query -W bluez qt6-qpa-plugins qt6-wayland hicolor-icon-theme
stagewand-host --diagnose
stagewand-host --selftest --dry-run
for platform in offscreen wayland; do
  runuser --user nobody -- env STAGEWAND_DRY_RUN=1 \
    dbus-run-session -- python3 "$root/host/debian/smoke-desktop.py" "$platform"
done
runuser --user nobody -- env STAGEWAND_DRY_RUN=1 \
  xvfb-run -a dbus-run-session -- python3 "$root/host/debian/smoke-desktop.py" xcb

# Exercise the maintainer scripts with dpkg's real upgrade and removal arguments.
apt-get install -y --no-install-recommends --reinstall "$deb"
stagewand-host --selftest --dry-run
apt-get remove -y stagewand-host
test ! -e /usr/bin/stagewand
test ! -e /usr/bin/stagewand-host
test ! -e /usr/lib/udev/rules.d/70-stagewand-uinput.rules
test ! -e /usr/lib/modules-load.d/stagewand-uinput.conf
apt-get purge -y stagewand-host
test ! -e /var/lib/dpkg/info/stagewand-host.postinst
test ! -e /var/lib/dpkg/info/stagewand-host.postrm
echo "PASS: package install, desktop lifecycle, upgrade, remove and purge"
