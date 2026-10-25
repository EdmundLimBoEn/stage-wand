#!/usr/bin/env python3
"""Report or repair what TestFlight serves to external testers.

Usage:
  asc-testflight.py status                     print the newest builds, their groups and beta review state
  asc-testflight.py ensure VERSION BUILD GROUP wait for that build to be VALID, set export compliance if
                                               missing, add it to GROUP and submit it for Beta App Review

Inputs (environment): ASC_KEY_ID, ASC_ISSUER_ID, APP_STORE_CONNECT_API_KEY_PATH, ASC_APP_ID.
Writes a Markdown table to $GITHUB_STEP_SUMMARY when set. Stdlib + openssl only.
Read-only in `status` mode.
"""
import os, sys, time, urllib.parse

import importlib.util
_spec = importlib.util.spec_from_file_location("ascprofile", os.path.join(os.path.dirname(os.path.abspath(__file__)), "asc-appstore-profile.py"))
ascprofile = importlib.util.module_from_spec(_spec); _spec.loader.exec_module(ascprofile)
call = ascprofile.call

APP = os.environ.get("ASC_APP_ID", "")


def builds(limit=10):
    query = urllib.parse.urlencode({"filter[app]": APP, "sort": "-uploadedDate", "limit": limit,
                                    "include": "preReleaseVersion,buildBetaDetail,betaAppReviewSubmission"})
    response = call("GET", "/v1/builds?" + query)
    membership = {}
    for group in call("GET", f"/v1/apps/{APP}/betaGroups?limit=200")["data"]:
        for member in call("GET", f"/v1/betaGroups/{group['id']}/builds?limit=200&fields[builds]=version")["data"]:
            membership.setdefault(member["id"], []).append(group["attributes"]["name"])
    included = {(i["type"], i["id"]): i["attributes"] for i in response.get("included", [])}
    rows = []
    for build in response["data"]:
        rel = {k: (v.get("data") or {}) for k, v in build["relationships"].items()}
        get = lambda kind, key: included.get((kind, rel.get(key, {}).get("id")), {})
        groups = membership.get(build["id"], [])
        rows.append({"id": build["id"], "marketing": get("preReleaseVersions", "preReleaseVersion").get("version"),
                     **build["attributes"], "version_number": build["attributes"]["version"], "groups": groups,
                     "external": get("buildBetaDetails", "buildBetaDetail").get("externalBuildState"),
                     "internal": get("buildBetaDetails", "buildBetaDetail").get("internalBuildState"),
                     "review": get("betaAppReviewSubmissions", "betaAppReviewSubmission").get("betaReviewState")})
    return rows


def status():
    lines = ["### TestFlight state", "", "| Version | Build | Uploaded | Processing | Expired | Encryption | Internal | External | Beta review | Groups |",
             "|---|---|---|---|---|---|---|---|---|---|"]
    for r in builds():
        lines.append(f"| {r['marketing']} | {r['version_number']} | {r['uploadedDate']} | {r['processingState']} | {r['expired']} | "
                     f"{r['usesNonExemptEncryption']} | {r['internal']} | {r['external']} | {r['review']} | {', '.join(r['groups'])} |")
    lines += ["", "| Group | Internal | Public link | Link |", "|---|---|---|---|"]
    for g in call("GET", f"/v1/apps/{APP}/betaGroups")["data"]:
        a = g["attributes"]
        lines.append(f"| {a['name']} | {a['isInternalGroup']} | {a.get('publicLinkEnabled')} | {a.get('publicLink') or ''} |")
    text = "\n".join(lines)
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as summary:
            summary.write(text + "\n")


def ensure(version, number, group_name):
    deadline = time.monotonic() + 45 * 60
    state = "NOT_FOUND"
    while True:
        if time.monotonic() >= deadline:
            sys.exit(f"::error::Build {version} ({number}) still {state} after 45 minutes.")
        query = urllib.parse.urlencode({"filter[app]": APP, "filter[version]": number,
                                        "filter[preReleaseVersion.version]": version, "limit": 1})
        try:
            found = call("GET", "/v1/builds?" + query, deadline=deadline)["data"]
        except TimeoutError:
            sys.exit(f"::error::Timed out waiting for build {version} ({number}) in App Store Connect.")
        state = found[0]["attributes"]["processingState"] if found else "NOT_FOUND"
        print(f"Build {version} ({number}): {state}")
        if state == "VALID":
            break
        if state in ("FAILED", "INVALID"):
            sys.exit(f"::error::Build {version} ({number}) is {state} in App Store Connect.")
        if time.monotonic() >= deadline:
            sys.exit(f"::error::Build {version} ({number}) still {state} after 45 minutes.")
        time.sleep(max(0, min(30, deadline - time.monotonic())))
    build = found[0]
    if build["attributes"].get("usesNonExemptEncryption") is None:
        call("PATCH", f"/v1/builds/{build['id']}", {"data": {"type": "builds", "id": build["id"],
                                                           "attributes": {"usesNonExemptEncryption": False}}})
        print("Set usesNonExemptEncryption=false")
    groups = [g for g in call("GET", f"/v1/apps/{APP}/betaGroups?limit=200")["data"] if g["attributes"]["name"] == group_name]
    if not groups:
        sys.exit(f"::error::Beta group {group_name!r} not found.")
    group_id = groups[0]["id"]
    current = [g["id"] for g in call("GET", f"/v1/betaGroups/{group_id}/builds?limit=200&fields[builds]=version")["data"]]
    if build["id"] not in current:
        call("POST", f"/v1/betaGroups/{group_id}/relationships/builds", {"data": [{"type": "builds", "id": build["id"]}]})
        print(f"Added build to {group_name}")
    submission = call("GET", f"/v1/builds/{build['id']}/betaAppReviewSubmission").get("data")
    if submission:
        if submission["attributes"]["betaReviewState"] == "REJECTED":
            sys.exit(f"::error::Beta App Review for build {version} ({number}) is REJECTED. "
                     "Open App Store Connect to review and resolve the rejection before retrying external distribution.")
        print(f"Beta App Review already submitted: {submission['attributes']['betaReviewState']}")
    else:
        result = call("POST", "/v1/betaAppReviewSubmissions",
                      {"data": {"type": "betaAppReviewSubmissions",
                                "relationships": {"build": {"data": {"type": "builds", "id": build["id"]}}}}})
        print(f"Submitted for Beta App Review: {result['data']['attributes']['betaReviewState']}")


if __name__ == "__main__":
    if not APP:
        sys.exit("::error::ASC_APP_ID is not set.")
    if sys.argv[1:2] == ["status"]:
        status()
    elif sys.argv[1:2] == ["ensure"] and len(sys.argv) == 5:
        ensure(*sys.argv[2:])
    else:
        sys.exit(__doc__)
