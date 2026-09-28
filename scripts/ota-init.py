#!/usr/bin/env python3
"""Generate local-only deployment secrets once; never print their values."""
import argparse
import base64
import os
from pathlib import Path
import re
import secrets


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--admin-email", required=True)
    parser.add_argument("--directory", type=Path, default=Path(__file__).resolve().parents[1] / "deploy/ota")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9_.+%-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}", args.admin_email):
        parser.error("use a plain email address")
    directory = args.directory.resolve()
    directory.mkdir(parents=True, exist_ok=True)
    secret_dir = directory / ".secrets"
    if secret_dir.is_symlink():
        parser.error("secret directory must not be a symlink")
    secret_dir.mkdir(mode=0o700, exist_ok=True)
    secret_dir.chmod(0o700)
    values = {
        "jwt_secret": secrets.token_hex(48),
        "master_key": base64.b64encode(secrets.token_bytes(32)).decode(),
        "postgres_password": secrets.token_hex(32),
        "redis_password": secrets.token_hex(32),
        "admin_password": secrets.token_urlsafe(32) + "Aa1!",
    }
    for name, value in values.items():
        target = secret_dir / name
        if target.is_symlink():
            parser.error(f"{target} must not be a symlink")
        if target.exists():
            if not target.read_text().strip():
                parser.error(f"{target} is empty; restore the original secret")
            continue
        # Docker Compose mounts host files without honoring uid/gid overrides.
        # 0444 makes them readable to non-root containers; the host parent is 0700.
        fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as stream:
            stream.write(value + "\n")
        target.chmod(0o444)
    redis_config = secret_dir / "redis_config"
    if not redis_config.exists():
        password = (secret_dir / "redis_password").read_text().strip()
        if not re.fullmatch(r"[0-9a-f]{64}", password):
            parser.error("Redis password is not the expected generated hex value")
        redis_config.write_text(
            "bind 0.0.0.0\nprotected-mode yes\nport 6379\n"
            f"requirepass {password}\n"
            'save ""\nappendonly no\nmaxmemory 256mb\nmaxmemory-policy noeviction\n'
        )
        redis_config.chmod(0o444)
    env_file = directory / ".env"
    if not env_file.exists():
        env_file.write_text(f"OTA_ADMIN_EMAIL={args.admin_email}\nOTA_BASE_URL=http://localhost:3000\nOTA_PORT=3000\n")
        env_file.chmod(0o600)
    print(f"Local configuration ready in {directory}; existing secrets preserved.")
    print("Keep .secrets off Git and back up the master key separately from PostgreSQL.")


if __name__ == "__main__":
    main()
