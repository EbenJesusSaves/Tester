#!/usr/bin/env python3
"""Exercise real publishing and signed downloads using two temporary apps.

Only supports a disposable localhost server with local storage. This writes
synthetic (non-executable) fixture updates and deletes only the apps it creates.
It is a protocol test, not a substitute for installing a native release build.
"""
import argparse
import base64
from email import policy
from email.parser import BytesParser
import gzip
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import uuid


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise RuntimeError("Fixture server must not redirect credential-bearing requests")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:3000")
    parser.add_argument("--admin-email", required=True)
    parser.add_argument("--password-file", type=Path, default=Path(__file__).resolve().parents[1] / "deploy/ota/.secrets/admin_password")
    parser.add_argument("--disposable-server", action="store_true", required=True)
    args = parser.parse_args()
    args.url = args.url.rstrip("/")
    origin = urllib.parse.urlsplit(args.url)
    if origin.scheme not in ("http", "https") or origin.hostname not in ("localhost", "127.0.0.1", "::1") or origin.username or origin.password or origin.query or origin.fragment:
        parser.error("this fixture test is restricted to a disposable localhost server")
    subprocess.run(["openssl", "version"], check=True, capture_output=True)
    opener = urllib.request.build_opener(NoRedirect())

    def request(method, path, *, token=None, data=None, headers=None, expected=200):
        url = path if path.startswith(("http://", "https://")) else args.url + path
        if urllib.parse.urlsplit(url).netloc != origin.netloc:
            raise RuntimeError("This test requires local storage; refusing to forward credentials to another origin")
        hdr = dict(headers or {})
        if token:
            hdr["Authorization"] = "Bearer " + token
        if data is not None and not isinstance(data, bytes):
            hdr["Content-Type"] = "application/json"
            data = json.dumps(data).encode()
        try:
            response = opener.open(urllib.request.Request(url, data=data, headers=hdr, method=method), timeout=20)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            body = response.read(4 * 1024 * 1024)
            if response.code not in ((expected,) if isinstance(expected, int) else expected):
                raise RuntimeError(f"{method} {urllib.parse.urlsplit(url).path}: HTTP {response.code}, expected {expected}")
            return body, response.headers

    def api(method, path, token=None, data=None, expected=200):
        body, _ = request(method, path, token=token, data=data, expected=expected)
        return json.loads(body) if body else None

    credentials = urllib.parse.urlencode({"email": args.admin_email, "password": args.password_file.read_text().strip()}).encode()
    body, _ = request("POST", "/auth/login", data=credentials, headers={"Content-Type": "application/x-www-form-urlencoded"})
    admin = json.loads(body)["token"]
    created = []
    namespace = uuid.uuid4().hex
    runtime = "ota-e2e-" + namespace
    fixtures = []
    try:
        with tempfile.TemporaryDirectory(prefix="ota-e2e-") as temporary:
            directory = Path(temporary)
            for index in range(2):
                app = api("POST", "/api/apps", admin, {"name": f"OTA disposable E2E {namespace} {index}", "keysConfig": {"mode": "database"}}, 201)["appId"]
                created.append(app)
                base = "/api/apps/" + app
                token = api("POST", base + "/apiKeys", admin, {"name": "temporary-e2e-publisher"}, 201)["apiKey"]
                cert, _ = request("GET", base + "/certificate", token=admin)
                certificate = directory / f"{index}.pem"
                certificate.write_bytes(cert)
                public_key = directory / f"{index}.pub.pem"
                public_key.write_bytes(subprocess.run(["openssl", "x509", "-in", str(certificate), "-pubkey", "-noout"], check=True, capture_output=True).stdout)
                fixtures.append({"app": app, "token": token, "public_key": public_key})
                for channel in ("dev", "uat", "prod"):
                    api("POST", base + "/branches", admin, {"branchName": channel})
                    api("POST", base + "/channels", admin, {"channelName": channel, "branchName": channel})
                    for platform in ("ios", "android"):
                        marker = f"{app}/{channel}/{platform}"
                        files = {
                            "bundle.js": (f"// non-executable OTA protocol fixture: {marker}\n".encode(), "launch", "js"),
                            "asset.txt": (("fixture asset " + marker).encode(), "asset", "txt"),
                            "expoConfig.json": (json.dumps({"name": "Disposable OTA fixture", "slug": "ota-fixture", "runtimeVersion": runtime}).encode(), "config", "json"),
                            "metadata.json": (json.dumps({"version": 0, "bundler": "metro", "fileMetadata": {platform: {"bundle": "bundle.js", "assets": [{"path": "asset.txt", "ext": "txt"}]}}}).encode(), "config", "json"),
                        }
                        manifest_files = [{"path": path, "hash": base64.urlsafe_b64encode(hashlib.sha256(value[0]).digest()).decode().rstrip("="), "key": hashlib.md5(value[0]).hexdigest(), "ext": value[2], "role": value[1]} for path, value in files.items()]
                        query = urllib.parse.urlencode({"runtimeVersion": runtime, "platform": platform, "commitHash": namespace})
                        upload = api("POST", f"/{app}/requestUploadUrl/{channel}?{query}", token, {"files": manifest_files, "message": "Disposable E2E fixture"})
                        for item in upload["uploadRequests"]:
                            content = files[item["originalFileName"]][0]
                            boundary = "ota-e2e-" + uuid.uuid4().hex
                            multipart = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="fixture"\r\nContent-Type: application/octet-stream\r\n\r\n'.encode() + content + f"\r\n--{boundary}--\r\n".encode())
                            upload_headers = dict(item.get("headers") or {})
                            if "local-upload-token" not in upload_headers:
                                raise RuntimeError("This fixture test only supports STORAGE_MODE=local")
                            upload_headers["Content-Type"] = "multipart/form-data; boundary=" + boundary
                            request("PUT", item["requestUploadUrl"], token=token, data=multipart, headers=upload_headers)
                        published = api("POST", f"/{app}/markUpdateAsUploaded/{channel}?{query}&updateId={upload['updateId']}", token)
                        headers = {"expo-app-id": app, "expo-channel-name": channel, "expo-platform": platform, "expo-runtime-version": runtime, "expo-protocol-version": "1", "expo-expect-signature": 'sig, keyid="main", alg="rsa-v1_5-sha256"'}
                        body, response_headers = request("GET", "/manifest", headers=headers)
                        assert "no-store" in response_headers.get("Cache-Control", ""), "manifest entered shared cache"
                        message = BytesParser(policy=policy.default).parsebytes(("Content-Type: " + response_headers["Content-Type"] + "\r\n\r\n").encode() + body)
                        part = next(part for part in message.iter_parts() if part.get_param("name", header="content-disposition") == "manifest")
                        payload = part.get_payload(decode=True)
                        signature = re.search(r'sig="([^"]+)"', str(part["expo-signature"]))
                        assert signature, "manifest not signed"
                        signature_path = directory / "signature.bin"
                        signature_path.write_bytes(base64.b64decode(signature.group(1)))
                        verify = ["openssl", "dgst", "-sha256", "-verify", str(public_key), "-signature", str(signature_path)]
                        subprocess.run(verify, input=payload, check=True, capture_output=True)
                        assert subprocess.run(verify, input=payload + b"tampered", capture_output=True).returncode != 0, "tampered manifest accepted"
                        manifest = json.loads(payload)
                        assert manifest["id"] == published["updateUUID"], "wrong app/channel/platform release selected"
                        for asset in [manifest["launchAsset"], *manifest["assets"]]:
                            downloaded, asset_headers = request("GET", asset["url"], headers={"expo-app-id": app, "expo-channel-name": channel})
                            if asset_headers.get("Content-Encoding") == "gzip":
                                downloaded = gzip.decompress(downloaded)
                            assert base64.urlsafe_b64encode(hashlib.sha256(downloaded).digest()).decode().rstrip("=") == asset["hash"], "asset hash mismatch"
                            assert marker.encode() in downloaded, "cross-app/channel/platform asset bytes"
                        fixture = fixtures[-1]
                        fixture["last_asset_url"] = manifest["launchAsset"]["url"]
                        print(f"PASS signed manifest, tamper rejection and asset hashes: app{index}/{channel}/{platform}")
            # Tenant integrity: another app's token cannot publish to this app;
            # an existing tenant cannot read the other app's app-scoped CAS object.
            a, b = fixtures
            request("POST", f"/{a['app']}/requestUploadUrl/dev?runtimeVersion={runtime}&platform=ios", token=b["token"], data={"files": []}, expected=(401, 403))
            request("GET", a["last_asset_url"], headers={"expo-app-id": b["app"], "expo-channel-name": "prod"}, expected=404)
            print("PASS cross-app publisher rejection and asset isolation")
    finally:
        failures = []
        for app in created:
            try:
                request("DELETE", "/api/apps/" + app, token=admin, expected=204)
            except Exception:
                failures.append(app)
        if failures:
            raise RuntimeError("Clean up these disposable app IDs manually: " + ", ".join(failures))
    print("PASS 12 published releases; temporary apps removed. No mobile binary was installed.")


if __name__ == "__main__":
    try:
        main()
    except (AssertionError, OSError, RuntimeError, ValueError, KeyError, subprocess.SubprocessError, StopIteration) as error:
        sys.exit(str(error))
