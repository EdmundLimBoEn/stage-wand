#!/usr/bin/env bash
# Single source of truth for the Stage Wand app version: the VERSION file.
#
#   scripts/version.sh            print the version
#   scripts/version.sh check      fail if mac/Info.plist or ios/project.yml disagree with VERSION
#   scripts/version.sh set 0.2.0  write the version to VERSION, mac/Info.plist and ios/project.yml
#
# Release workflows read VERSION. The copies in mac/Info.plist
# (CFBundleShortVersionString) and ios/project.yml (MARKETING_VERSION) exist so
# local `make -C mac app` and Xcode builds carry the same version; CI keeps
# them in sync with `check`.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
version_file="${root}/VERSION"
plist="${root}/mac/Info.plist"
project="${root}/ios/project.yml"

valid() {
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

current() {
  tr -d '[:space:]' < "${version_file}"
}

mac_version() {
  python3 -c 'import plistlib,sys; print(plistlib.load(open(sys.argv[1],"rb"))["CFBundleShortVersionString"])' "${plist}"
}

ios_version() {
  sed -nE 's/^[[:space:]]*MARKETING_VERSION:[[:space:]]*"?([^"[:space:]]+)"?.*/\1/p' "${project}" | head -n1
}

case "${1:-print}" in
  print)
    current
    ;;
  check)
    version="$(current)"
    valid "${version}" || { echo "::error file=VERSION::VERSION '${version}' must be MAJOR.MINOR.PATCH"; exit 1; }
    status=0
    if [ "$(mac_version)" != "${version}" ]; then
      echo "::error file=mac/Info.plist::CFBundleShortVersionString is $(mac_version), VERSION is ${version}. Run scripts/version.sh set ${version}"
      status=1
    fi
    if [ "$(ios_version)" != "${version}" ]; then
      echo "::error file=ios/project.yml::MARKETING_VERSION is $(ios_version), VERSION is ${version}. Run scripts/version.sh set ${version}"
      status=1
    fi
    [ "${status}" -eq 0 ] && echo "Version ${version} is consistent (VERSION, mac/Info.plist, ios/project.yml)"
    exit "${status}"
    ;;
  set)
    version="${2:-}"
    valid "${version}" || { echo "usage: scripts/version.sh set MAJOR.MINOR.PATCH" >&2; exit 2; }
    printf '%s\n' "${version}" > "${version_file}"
    python3 - "${plist}" "${version}" <<'PY'
import pathlib, re, sys
path, version = pathlib.Path(sys.argv[1]), sys.argv[2]
text = path.read_text()
text, count = re.subn(r"(<key>CFBundleShortVersionString</key>\s*<string>)[^<]*(</string>)",
                      lambda m: m.group(1) + version + m.group(2), text)
if count != 1:
    sys.exit("CFBundleShortVersionString not found exactly once in mac/Info.plist")
path.write_text(text)
PY
    python3 - "${project}" "${version}" <<'PY'
import pathlib, re, sys
path, version = pathlib.Path(sys.argv[1]), sys.argv[2]
text, count = re.subn(r'(MARKETING_VERSION:\s*)"?[^"\s]+"?', lambda m: m.group(1) + '"' + version + '"', path.read_text())
if count != 1:
    sys.exit("MARKETING_VERSION not found exactly once in ios/project.yml")
path.write_text(text)
PY
    "$0" check
    ;;
  *)
    echo "usage: scripts/version.sh [print|check|set MAJOR.MINOR.PATCH]" >&2
    exit 2
    ;;
esac
