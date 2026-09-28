#!/usr/bin/env python3
"""Read-only checks against provisioned apps and every channel/platform pair."""
import argparse
import json
from pathlib import Path
import sys
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--state", type=Path, default=Path(__file__).resolve().parents[1] / "deploy/ota/state/apps.json")
    parser.add_argument("--runtime", default="ota-smoke-no-published-update")
    args = parser.parse_args()
    state = json.loads(args.state.read_text())

    def get(route, headers=None):
        try:
            with urllib.request.urlopen(urllib.request.Request(state["baseUrl"] + route, headers=headers or {}), timeout=10) as response:
                return response.status, response.headers, response.read(2 * 1024 * 1024)
        except urllib.error.HTTPError as error:
            return error.code, error.headers, error.read(4096)

    for endpoint in ("/hc", "/ready"):
        assert get(endpoint)[0] == 200, endpoint + " is unhealthy"
    # An absent tenant must never resolve to whichever app happened to be first.
    assert get("/api/apps")[0] in (401, 403), "management API accepted anonymous request"
    common = {"expo-protocol-version": "1", "expo-platform": "ios", "expo-runtime-version": args.runtime, "expo-channel-name": "prod"}
    assert get("/manifest", common)[0] == 400, "manifest accepted a missing app ID"
    assert get("/manifest", dict(common, **{"expo-app-id": "unknown-smoke-app"}))[0] in (400, 404), "unknown app did not fail closed"
    for slug, app in state["apps"].items():
        for channel in ("dev", "uat", "prod"):
            for platform in ("ios", "android"):
                headers = dict(common, **{"expo-app-id": app["id"], "expo-channel-name": channel, "expo-platform": platform})
                code, response_headers, body = get("/manifest", headers)
                assert code in (200, 204), f"{slug}/{channel}/{platform}: HTTP {code}"
                assert "no-store" in response_headers.get("Cache-Control", ""), "manifest must not enter a shared cache"
                if code == 200:
                    assert "multipart/mixed" in response_headers.get("Content-Type", ""), "unexpected protocol response"
                print(f"PASS {slug}/{channel}/{platform}: HTTP {code}")
    print("PASS health, anonymous management rejection, tenant selection and all channel/platform checks")


if __name__ == "__main__":
    try:
        main()
    except (AssertionError, OSError, ValueError, KeyError) as error:
        sys.exit(str(error))
