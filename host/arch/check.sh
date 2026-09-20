#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
fail() { echo "FAIL: $*" >&2; exit 1; }

udev="$root/host/udev/70-stagewand-uinput.rules"
[[ -f "$udev" ]] || fail "missing $udev"
[[ ! -e "$root/host/udev/99-stagewand-uinput.rules" ]] || fail "99- udev rule is too late for uaccess"
grep -q 'TAG+="uaccess"' "$udev" || fail "udev rule must set TAG+=uaccess"
grep -q 'SUBSYSTEM=="misc"' "$udev" || fail "udev rule must match SUBSYSTEM misc"
if grep -q 'GROUP="input"' "$udev"; then
  fail "udev rule must not use GROUP=input"
fi

mod="$root/host/modules-load.d/uinput.conf"
[[ -f "$mod" ]] || fail "missing $mod"
grep -qx 'uinput' "$mod" || fail "modules-load.d must contain uinput"

readme="$root/README.md"
grep -q 'pacman -S --needed go git' "$readme" || fail "README must show Arch pacman install"
grep -q 'pacman -S --needed bluez bluez-utils' "$readme" || fail "README must show Arch bluez install"
if grep -q 'usermod -aG input' "$readme"; then
  fail "README must not tell users to join the input group"
fi
grep -q 'uaccess' "$readme" || fail "README must mention uaccess"
grep -q 'apt install golang-go git' "$readme" || fail "README must show the Debian install line"

pkgbuild="$root/host/arch/PKGBUILD"
grep -q "makedepends=('go' 'gcc' 'cmake')" "$pkgbuild" || fail "PKGBUILD must use extra/go and core/gcc"
grep -q -- '-buildvcs=false' "$pkgbuild" || fail "PKGBUILD must set -buildvcs=false"
for dep in ydotool xdotool libei; do
  if grep -q "$dep" "$pkgbuild"; then
    fail "PKGBUILD must not depend on $dep"
  fi
done

if command -v pacman >/dev/null 2>&1; then
  pacman -Si go git gcc >/dev/null || fail "go, git, and gcc must be in the pacman sync database"
  repo_go=$(pacman -Si go | awk -F': ' '/^Repository/ {print $2; exit}')
  case "$repo_go" in
    extra|core) ;;
    *) fail "go must come from extra or core, got ${repo_go:-empty}" ;;
  esac
  echo "pacman go $(pacman -Si go | awk -F': ' '/^Version/ {print $2; exit}') from $repo_go"
fi

command -v go >/dev/null || fail "go is required"
go version

cd "$root/host"
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false"
go test ./internal/... ./cmd/... .
go build -o stagewand-host ./cmd/stagewand-host
./stagewand-host --diagnose
./stagewand-host --selftest --dry-run

echo "PASS: Arch host check"
