#!/usr/bin/env python3
"""Idempotently provision apps, dev/uat/prod channels and public certificates."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
CHANNELS = ("dev", "uat", "prod")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise RuntimeError("Unexpected HTTP redirect; check --url (credentials are never forwarded)")


def save(path, value):
    temporary = path.with_suffix(".tmp")
    with temporary.open("w") as stream:
        json.dump(value, stream, indent=2)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://localhost:3000")
    parser.add_argument("--admin-email", required=True)
    parser.add_argument("--password-file", type=Path, default=ROOT / "deploy/ota/.secrets/admin_password")
    parser.add_argument("--apps", type=Path, default=ROOT / "deploy/ota/apps.example.json")
    parser.add_argument("--state", type=Path, default=ROOT / "deploy/ota/state")
    args = parser.parse_args()
    args.url = args.url.rstrip("/")
    parsed = urllib.parse.urlsplit(args.url)
    if parsed.username or parsed.password or parsed.query or parsed.fragment or not parsed.hostname:
        parser.error("URL must contain an origin and optional path, without credentials/query/fragment")
    if parsed.scheme != "https" and not (parsed.scheme == "http" and parsed.hostname in ("localhost", "127.0.0.1", "::1")):
        parser.error("HTTPS is required except on localhost")
    apps = json.loads(args.apps.read_text())["apps"]
    slugs, names = set(), set()
    for app in apps:
        if not re.fullmatch(r"[a-z][a-z0-9-]{0,47}", app["slug"]):
            parser.error("app slug must be 1-48 lowercase letters/digits/hyphens, starting with a letter")
        if not isinstance(app["name"], str) or not app["name"].strip():
            parser.error("each app requires a display name")
        if app["slug"] in slugs or app["name"] in names:
            parser.error("app slugs and display names must be unique")
        slugs.add(app["slug"])
        names.add(app["name"])
    if not apps:
        parser.error("apps must not be empty")
    args.state.mkdir(parents=True, exist_ok=True)
    # Serialize local runs: POST /api/apps has no idempotency-key primitive.
    lock = (args.state / ".bootstrap.lock").open("w")
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    state_file = args.state / "apps.json"
    state = json.loads(state_file.read_text()) if state_file.exists() else {"baseUrl": args.url, "apps": {}}
    if state["baseUrl"] != args.url:
        parser.error("state belongs to another server; use a separate --state directory")
    opener = urllib.request.build_opener(NoRedirect())
    token = None

    def request(method, route, data=None, form=False, raw=False):
        headers = {"Accept": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        body = None
        if data is not None:
            body = (urllib.parse.urlencode(data) if form else json.dumps(data)).encode()
            headers["Content-Type"] = "application/x-www-form-urlencoded" if form else "application/json"
        try:
            with opener.open(urllib.request.Request(args.url + route, data=body, headers=headers, method=method), timeout=30) as response:
                result = response.read(4 * 1024 * 1024)
        except urllib.error.HTTPError as error:
            # Never dump credential-bearing request bodies or server responses.
            raise RuntimeError(f"{method} {route} returned HTTP {error.code}; inspect server logs") from None
        return result if raw else (json.loads(result) if result else None)

    request("GET", "/ready", raw=True)
    token = request("POST", "/auth/login", {"email": args.admin_email, "password": args.password_file.read_text().strip()}, form=True)["token"]
    known = request("GET", "/api/apps") or []
    for desired in apps:
        slug, name = desired["slug"], desired["name"]
        existing = state["apps"].get(slug)
        if existing:
            app_id = existing["id"]
            if not any(app["id"] == app_id for app in known):
                raise RuntimeError(f"{slug}: saved app ID is missing from server; restore it, do not silently replace installed-client identity")
        else:
            matches = [app for app in known if app["name"] == name]
            if len(matches) > 1:
                raise RuntimeError(f"{slug}: ambiguous display name; use unique app names")
            app_id = matches[0]["id"] if matches else request("POST", "/api/apps", {"name": name, "keysConfig": {"mode": "database"}})["appId"]
            state["apps"][slug] = {"id": app_id, "name": name}
            save(state_file, state)
        base = "/api/apps/" + urllib.parse.quote(app_id, safe="")
        branches = {branch["branchName"] for branch in request("GET", base + "/branches") or []}
        channels = {channel["releaseChannelName"]: channel for channel in request("GET", base + "/channels") or []}
        for channel in CHANNELS:
            if channel not in branches:
                request("POST", base + "/branches", {"branchName": channel})
            if channel not in channels:
                request("POST", base + "/channels", {"channelName": channel, "branchName": channel})
            elif (channels[channel].get("branchSurfing") or {}).get("enabled"):
                raise RuntimeError(f"{slug}/{channel}: branch surfing is enabled; disable it before applying this deployment policy")
            # Existing mappings/rollouts can be intentional promotions: never reset them.
        cert = request("GET", base + "/certificate", raw=True)
        cert_path = args.state / f"{slug}-certificate.pem"
        if cert_path.exists() and cert_path.read_bytes() != cert:
            raise RuntimeError(f"{slug}: signing certificate changed; review client trust before replacing it")
        cert_path.write_bytes(cert)
        state["apps"][slug].update({"channels": list(CHANNELS), "certificate": cert_path.name})
        save(state_file, state)
        print(f"{slug}: {app_id}; dev, uat, prod ready; certificate {cert_path}")
    print("No existing channel mappings, certificates, app IDs or release history were replaced.")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, RuntimeError) as error:
        sys.exit(str(error))
