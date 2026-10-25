#!/usr/bin/env python3
"""Find or create the App Store provisioning profile for the iOS remote and install it.

Used by ios-testflight.yml so the archive is signed manually with the imported
Apple Distribution certificate. Automatic signing on a fresh runner would
otherwise create a new Apple Development certificate on every run.

Inputs (environment): ASC_KEY_ID, ASC_ISSUER_ID, APP_STORE_CONNECT_API_KEY_PATH,
DISTRIBUTION_CERT_PEM (path to the imported certificate in PEM form).
Usage: asc-appstore-profile.py BUNDLE_ID
Writes PROFILE_NAME and PROFILE_UUID to $GITHUB_ENV when set. Stdlib + openssl only.
"""
import base64, contextlib, json, os, pathlib, plistlib, signal, subprocess, sys, time, urllib.error, urllib.parse, urllib.request

API = "https://api.appstoreconnect.apple.com"


def b64url(data):
    return base64.urlsafe_b64encode(data).rstrip(b"=")


def der_to_raw(der):
    # ECDSA-Sig-Value ::= SEQUENCE { r INTEGER, s INTEGER }
    def read_len(buf, i):
        n = buf[i]; i += 1
        if n & 0x80:
            count = n & 0x7F
            n = int.from_bytes(buf[i:i + count], "big"); i += count
        return n, i
    assert der[0] == 0x30
    _, i = read_len(der, 1)
    parts = []
    for _ in range(2):
        assert der[i] == 0x02
        n, i = read_len(der, i + 1)
        parts.append(int.from_bytes(der[i:i + n], "big").to_bytes(32, "big")); i += n
    return parts[0] + parts[1]


def token():
    header = b64url(json.dumps({"alg": "ES256", "kid": os.environ["ASC_KEY_ID"], "typ": "JWT"}).encode())
    now = int(time.time())
    payload = b64url(json.dumps({"iss": os.environ["ASC_ISSUER_ID"], "iat": now, "exp": now + 900,
                                 "aud": "appstoreconnect-v1"}).encode())
    signing_input = header + b"." + payload
    der = subprocess.run(["openssl", "dgst", "-sha256", "-sign", os.environ["APP_STORE_CONNECT_API_KEY_PATH"]],
                         input=signing_input, capture_output=True, check=True).stdout
    return (signing_input + b"." + b64url(der_to_raw(der))).decode()


@contextlib.contextmanager
def response_deadline(timeout):
    # These CLI helpers run on the main thread of Unix CI runners. Socket
    # timeouts alone cannot stop a response that keeps trickling bytes.
    def expired(signum, frame):
        raise TimeoutError("App Store Connect response deadline expired")

    previous_handler = signal.signal(signal.SIGALRM, expired)
    started = time.monotonic()
    previous_timer = signal.setitimer(signal.ITIMER_REAL, timeout)
    try:
        yield
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, previous_handler)
        delay, interval = previous_timer
        if delay:
            delay = max(0.000001, delay - (time.monotonic() - started))
        signal.setitimer(signal.ITIMER_REAL, delay, interval)


def call(method, path, body=None, *, deadline=None):
    # Retry reads only: a timed-out write may already have been applied by Apple.
    attempts = 3 if method == "GET" else 1
    for attempt in range(attempts):
        if deadline is not None and time.monotonic() >= deadline:
            raise TimeoutError("App Store Connect polling deadline expired")
        request = urllib.request.Request(API + path, method=method,
                                         data=json.dumps(body).encode() if body is not None else None,
                                         headers={"Authorization": "Bearer " + token(), "Content-Type": "application/json"})
        timeout = 30 if deadline is None else min(30, deadline - time.monotonic())
        if timeout <= 0:
            raise TimeoutError("App Store Connect polling deadline expired")
        try:
            with response_deadline(timeout):
                with urllib.request.urlopen(request, timeout=timeout) as response:
                    result = json.loads(response.read() or b"{}")
            if deadline is not None and time.monotonic() >= deadline:
                raise TimeoutError("App Store Connect polling deadline expired")
            return result
        except urllib.error.HTTPError as error:
            transient = error.code in (429, 500, 502, 503, 504)
            # Do not wait for an error body before enforcing the retry budget.
            error.close()
            if not transient or attempt == attempts - 1:
                sys.exit(f"::error::App Store Connect API {method} {path.split('?')[0]} failed: {error.code}")
        except (urllib.error.URLError, TimeoutError, ConnectionError):
            if attempt == attempts - 1:
                raise
        delay = 2 ** attempt
        if deadline is not None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("App Store Connect polling deadline expired")
            delay = min(delay, remaining)
        time.sleep(delay)


def main():
    bundle_identifier = sys.argv[1]
    serial = subprocess.run(["openssl", "x509", "-noout", "-serial", "-in", os.environ["DISTRIBUTION_CERT_PEM"]],
                            capture_output=True, text=True, check=True).stdout.strip().split("=", 1)[1].upper().lstrip("0")

    certificates = call("GET", "/v1/certificates?limit=200&filter[certificateType]=DISTRIBUTION,IOS_DISTRIBUTION"
                               "&fields[certificates]=serialNumber,certificateType")["data"]
    matches = [c for c in certificates if c["attributes"]["serialNumber"].upper().lstrip("0") == serial]
    if not matches:
        sys.exit("::error::The imported distribution certificate is not an active certificate of this team (revoked or wrong p12?).")
    certificate_id = matches[0]["id"]

    query = urllib.parse.urlencode({"filter[identifier]": bundle_identifier, "fields[bundleIds]": "identifier"})
    bundles = [b for b in call("GET", "/v1/bundleIds?" + query)["data"] if b["attributes"]["identifier"] == bundle_identifier]
    if not bundles:
        sys.exit(f"::error::Bundle ID is not registered in the developer portal. See docs/testflight.md.")
    bundle_id = bundles[0]["id"]

    name = f"Stage Wand App Store CI {certificate_id}"
    query = urllib.parse.urlencode({"filter[name]": name, "filter[profileType]": "IOS_APP_STORE",
                                    "fields[profiles]": "name,profileState,profileContent,uuid"})
    existing = [p for p in call("GET", "/v1/profiles?" + query)["data"]
                if p["attributes"]["name"] == name and p["attributes"]["profileState"] == "ACTIVE"]
    if existing:
        profile = existing[0]
        print(f"Reusing profile {profile['id']} ({name})")
    else:
        profile = call("POST", "/v1/profiles", {"data": {
            "type": "profiles",
            "attributes": {"name": name, "profileType": "IOS_APP_STORE"},
            "relationships": {
                "bundleId": {"data": {"type": "bundleIds", "id": bundle_id}},
                "certificates": {"data": [{"type": "certificates", "id": certificate_id}]},
            }}})["data"]
        print(f"Created profile {profile['id']} ({name})")

    content = base64.b64decode(profile["attributes"]["profileContent"])
    uuid = profile["attributes"]["uuid"]
    directory = pathlib.Path.home() / "Library/MobileDevice/Provisioning Profiles"
    directory.mkdir(parents=True, exist_ok=True)
    (directory / f"{uuid}.mobileprovision").write_bytes(content)
    # Xcode 16+ also looks here.
    modern = pathlib.Path.home() / "Library/Developer/Xcode/UserData/Provisioning Profiles"
    modern.mkdir(parents=True, exist_ok=True)
    (modern / f"{uuid}.mobileprovision").write_bytes(content)

    if os.environ.get("GITHUB_ENV"):
        with open(os.environ["GITHUB_ENV"], "a") as env:
            env.write(f"PROFILE_NAME={name}\nPROFILE_UUID={uuid}\n")
    print(f"Installed {uuid}")


if __name__ == "__main__":
    main()
