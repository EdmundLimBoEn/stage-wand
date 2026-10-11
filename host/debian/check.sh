#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
bash -n "$root/host/debian/build.sh" "$root/host/debian/smoke.sh"
sh -n "$root/host/debian/postinst" "$root/host/debian/postrm"
cd "$root/host"
go test -buildvcs=false .
echo "PASS: Debian packaging check"
