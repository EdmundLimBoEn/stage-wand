#!/usr/bin/env bash
# Build StageWandMac.app as a universal binary and wrap it in a DMG.
#
# Local / PR use (ad-hoc signed, not notarized; version from the VERSION file):
#   scripts/mac-release.sh
#
# Release use (what .github/workflows/mac-release.yml does):
#   VERSION=0.2.0 BUILD_NUMBER=7 \
#   SIGN_IDENTITY="Developer ID Application: <name> (DUU8J39BA7)" KEYCHAIN=/path/to.keychain-db \
#   NOTARIZE=1 ASC_KEY_PATH=/path/AuthKey_XXXX.p8 ASC_KEY_ID=XXXX ASC_ISSUER_ID=<uuid> \
#   scripts/mac-release.sh
#
# With NOTARIZE=1 the app is notarized and stapled, then the DMG is built,
# signed, notarized and stapled, and both are checked with stapler and spctl.
# Any missing input or rejected submission exits non-zero: nothing unsigned
# is produced on the release path.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
version="${VERSION:-$(tr -d '[:space:]' < "${root}/VERSION")}"
build_number="${BUILD_NUMBER:-1}"
identity="${SIGN_IDENTITY:--}"
notarize="${NOTARIZE:-0}"
out_dir="${OUT_DIR:-${root}/dist}"
keychain="${KEYCHAIN:-}"

fail() {
  echo "::error::$*" >&2
  exit 1
}

if [ "${notarize}" = "1" ]; then
  [ "${identity}" != "-" ] || fail "NOTARIZE=1 needs SIGN_IDENTITY set to a Developer ID Application identity."
  for name in ASC_KEY_PATH ASC_KEY_ID ASC_ISSUER_ID; do
    [ -n "${!name:-}" ] || fail "NOTARIZE=1 needs ${name}."
  done
  [ -f "${ASC_KEY_PATH}" ] || fail "ASC_KEY_PATH does not point at a file."
fi

work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

sign() {
  local args=(--force --sign "${identity}")
  if [ "${identity}" = "-" ]; then
    args+=(--timestamp=none)
  else
    args+=(--timestamp)
  fi
  if [ -n "${keychain}" ]; then
    args+=(--keychain "${keychain}")
  fi
  codesign "${args[@]}" "$@"
}

notary_submit() {
  local file="$1" result id status
  echo "Submitting $(basename "${file}") to the notary service..."
  result="$(xcrun notarytool submit "${file}" \
    --key "${ASC_KEY_PATH}" --key-id "${ASC_KEY_ID}" --issuer "${ASC_ISSUER_ID}" \
    --wait --timeout 30m --output-format json)" || true
  id="$(printf '%s' "${result}" | python3 -c 'import json,sys
try: print(json.load(sys.stdin).get("id", ""))
except Exception: print("")')"
  status="$(printf '%s' "${result}" | python3 -c 'import json,sys
try: print(json.load(sys.stdin).get("status", ""))
except Exception: print("")')"
  echo "notarytool: file=$(basename "${file}") id=${id:-none} status=${status:-unknown}"
  if [ "${status}" != "Accepted" ]; then
    if [ -n "${id}" ]; then
      xcrun notarytool log "${id}" \
        --key "${ASC_KEY_PATH}" --key-id "${ASC_KEY_ID}" --issuer "${ASC_ISSUER_ID}" || true
    fi
    fail "Notarization of $(basename "${file}") was not accepted (status: ${status:-unknown})."
  fi
}

echo "Building StageWandMac ${version} (${build_number}), universal..."
swift_args=(--package-path "${root}/mac" -c release --arch arm64 --arch x86_64)
swift build "${swift_args[@]}"
bin_dir="$(swift build "${swift_args[@]}" --show-bin-path)"
lipo -archs "${bin_dir}/StageWandMac"

app="${work}/StageWandMac.app"
mkdir -p "${app}/Contents/MacOS"
cp "${bin_dir}/StageWandMac" "${app}/Contents/MacOS/StageWandMac"
cp "${root}/mac/Info.plist" "${app}/Contents/Info.plist"
/usr/libexec/PlistBuddy \
  -c "Set :CFBundleShortVersionString ${version}" \
  -c "Set :CFBundleVersion ${build_number}" \
  "${app}/Contents/Info.plist"
plutil -lint "${app}/Contents/Info.plist"
xattr -cr "${app}"

sign --options runtime --entitlements "${root}/mac/StageWandMac.entitlements" "${app}"
codesign --verify --strict --deep --verbose=2 "${app}"
codesign --display --verbose=2 "${app}" 2>&1 | grep -E '^(Identifier|Authority|TeamIdentifier|Timestamp|Runtime Version|CodeDirectory)' || true

if [ "${notarize}" = "1" ]; then
  ditto -c -k --keepParent "${app}" "${work}/StageWandMac.zip"
  notary_submit "${work}/StageWandMac.zip"
  xcrun stapler staple "${app}"
  xcrun stapler validate "${app}"
  spctl --assess --type execute -vv "${app}"
fi

mkdir -p "${out_dir}"
dmg="${out_dir}/StageWand-${version}.dmg"
staging="${work}/dmg"
mkdir -p "${staging}"
ditto "${app}" "${staging}/StageWandMac.app"
ln -s /Applications "${staging}/Applications"
rm -f "${dmg}"
hdiutil create -volname "Stage Wand ${version}" -srcfolder "${staging}" \
  -fs HFS+ -format UDZO -ov "${dmg}"

if [ "${identity}" != "-" ]; then
  sign "${dmg}"
  codesign --verify --strict --verbose=2 "${dmg}"
fi

if [ "${notarize}" = "1" ]; then
  notary_submit "${dmg}"
  xcrun stapler staple "${dmg}"
  xcrun stapler validate "${dmg}"
  spctl --assess --type open --context context:primary-signature -vv "${dmg}"
fi

(cd "${out_dir}" && shasum -a 256 "$(basename "${dmg}")" > "$(basename "${dmg}").sha256")
echo "DMG_PATH=${dmg}"
if [ -n "${GITHUB_ENV:-}" ]; then
  echo "DMG_PATH=${dmg}" >> "${GITHUB_ENV}"
fi
