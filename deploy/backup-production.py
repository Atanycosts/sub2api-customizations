#!/usr/bin/env python3
"""Keep a release-local PostgreSQL snapshot; credentials never enter argv/logs."""

import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
from datetime import datetime, timezone

import yaml


def main():
    label = sys.argv[1]
    if not re.fullmatch(r"[A-Za-z0-9._-]+", label):
        raise ValueError("invalid release label")
    root = Path("/opt/sub2api/backups") / label
    root.mkdir(mode=0o700, parents=True, exist_ok=False)
    effective = Path("/app/data/config.yaml")
    cfg = yaml.safe_load(effective.read_text())["database"]
    env = os.environ.copy()
    env["PGPASSWORD"] = str(cfg["password"])
    env["PGOPTIONS"] = "-c statement_timeout=120000 -c lock_timeout=5000"
    pg_dir = Path("/usr/local/pgsql/src/bin/pg_dump")
    dump = root / "database.dump"
    result = subprocess.run(
        [str(pg_dir / "pg_dump"), "-h", str(cfg.get("host", "127.0.0.1")),
         "-p", str(cfg.get("port", 5432)), "-U", str(cfg["user"]),
         "-d", str(cfg["dbname"]), "-Fc", "--no-owner", "--no-acl", "-f", str(dump)],
        env=env, capture_output=True, timeout=180,
    )
    if result.returncode:
        raise RuntimeError("database backup failed; raw diagnostics withheld")
    dump.chmod(0o600)
    subprocess.run([str(pg_dir / "pg_restore"), "--list", str(dump)],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    files = {}
    for source, name in [(effective, "effective-config.yaml"),
                         (Path("/opt/sub2api/config.yaml"), "fallback-config.yaml")]:
        if source.exists():
            target = root / name
            shutil.copyfile(source, target)
            target.chmod(0o600)
            files[name] = hashlib.sha256(target.read_bytes()).hexdigest()
    metadata = {
        "created_at": datetime.now(timezone.utc).isoformat(),
        "previous_binary": str(Path("/opt/sub2api/sub2api").resolve()),
        "previous_resources": str(Path("/opt/sub2api/resources").resolve()) if Path("/opt/sub2api/resources").exists() else None,
        "database_bytes": dump.stat().st_size,
        "database_sha256": hashlib.sha256(dump.read_bytes()).hexdigest(),
        "config_sha256": files,
    }
    (root / "backup.json").write_text(json.dumps(metadata, indent=2) + "\n")
    (root / "backup.json").chmod(0o600)
    print(json.dumps({"backup": str(root), "database_bytes": metadata["database_bytes"],
                      "database_sha256": metadata["database_sha256"]}))


if __name__ == "__main__":
    main()
