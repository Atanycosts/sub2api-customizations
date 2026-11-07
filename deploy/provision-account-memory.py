#!/usr/bin/env python3
"""Provision product accounts into independent Respire Runtime libraries.

Run as root on the production host. No existing library, key or binding is
replaced. The JSON manifest is published only after all requested users pass
Runtime identity and active-model checks; Sub2API admits additions on refresh.
"""

import argparse
import fcntl
import grp
import hashlib
import json
import os
from pathlib import Path
import pwd
import shutil
import subprocess
import sys
import time
import urllib.request
import uuid

import yaml


RELEASE = Path('/opt/respire-runtime/releases/1.0.20')
MANIFEST = Path('/etc/sub2api/memory-bindings.json')
PROFILE = 'bge-small-zh-v1.5'
MODEL_FILES = {
    'tokenizer.json': (
        '48cea5d44424912a6fd1ea647bf4fe50b55ab8b1e5879c3275f80e339e8fae26',
        439125,
    ),
    'onnx/model_quantized.onnx': (
        '15b717c382bcb518ba457b93ea6850ede7f4f1cd8937454aa06972366cd19bcc',
        24010842,
    ),
}
MODEL_URL = ('https://huggingface.co/Xenova/bge-small-zh-v1.5/resolve/'
             '75c43b069aac4d136ba6bc1122f995fedcfd2781/')


def run(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, text=True, **kwargs)


def rpc(port, method, args):
    body = json.dumps({'v': 1, 'id': str(uuid.uuid4()), 'method': method,
                       'args': args, 'queue_wait_ms': 1000}).encode()
    req = urllib.request.Request(f'http://127.0.0.1:{port}/api/rpc', data=body,
                                 headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=30) as response:
        data = json.load(response)
    if not data.get('ok') or data.get('exit') != 0:
        raise RuntimeError(f'Runtime {method} failed: {data.get("code", "rpc_error")}')
    return data['envelope']


def health(port):
    with urllib.request.urlopen(f'http://127.0.0.1:{port}/api/health', timeout=5) as response:
        return json.load(response)


def verify_model_file(path, digest, size):
    return (path.is_file() and path.stat().st_size == size
            and hashlib.sha256(path.read_bytes()).hexdigest() == digest)


def prepare_models():
    root = Path('/opt/respire-runtime/models') / PROFILE
    for name, (digest, size) in MODEL_FILES.items():
        target = root / name
        if verify_model_file(target, digest, size):
            continue
        target.parent.mkdir(parents=True, exist_ok=True)
        temporary = target.with_name(target.name + '.download')
        with urllib.request.urlopen(MODEL_URL + name, timeout=60) as response:
            with temporary.open('wb') as output:
                shutil.copyfileobj(response, output)
        if not verify_model_file(temporary, digest, size):
            raise RuntimeError(f'Model checksum failed: {name}')
        temporary.chmod(0o644)
        temporary.replace(target)
    return root


def runtime_env(home, library, port):
    env = {k: v for k, v in os.environ.items() if not k.startswith(('RSRS_', 'ONEMEMORY_'))}
    env.update(HOME=str(home), RSRS_DATA_DIR=str(library),
               RSRS_BIN_DIR=str(library / 'bin'), RSRS_RPC_PORT=str(port),
               RSRS_NO_AUTOSYNC='1', RSRS_UPDATE_CHECK='0')
    return env


def provision(user_id, model_root):
    name = f'sub2api-memory{user_id}'
    home = Path('/var/lib') / name
    library = home / 'respire'
    port = 15169 + user_id
    if port > 65535:
        raise RuntimeError('Dedicated Runtime port range exhausted')
    try:
        account = pwd.getpwnam(name)
    except KeyError:
        run(['useradd', '--system', '--create-home', '--home-dir', str(home),
             '--shell', '/usr/sbin/nologin', name])
        account = pwd.getpwnam(name)
    if Path(account.pw_dir) != home:
        raise RuntimeError(f'Unexpected service home for product user {user_id}')
    home.chmod(0o700)
    library.mkdir(mode=0o700, exist_ok=True)
    os.chown(library, account.pw_uid, account.pw_gid)
    env = runtime_env(home, library, port)
    receipt = home / 'initial-keygen-receipt.json'
    secret = Path('/etc') / f'{name}.env'
    if not receipt.exists():
        if secret.exists() or (library / 'session.json').exists() or (library / 'rsrs.db').exists():
            raise RuntimeError(f'Existing untracked library for user {user_id}; manual review required')
        with open(receipt, 'x', opener=lambda p, f: os.open(p, f, 0o600)) as output:
            result = subprocess.run(['runuser', '-u', name, '--', str(RELEASE / 'rsrs'),
                                     '--json', 'keygen'], env=env, stdout=output,
                                    stderr=subprocess.PIPE, text=True, timeout=45)
        data = json.loads(receipt.read_text())
        if result.returncode or data.get('status') != 'ok' or not data.get('summary', {}).get('super'):
            raise RuntimeError(f'Key initialization failed for user {user_id}; inspect protected receipt')
        # Recovery output stays root-only; never print or reuse another user's key.
    if not secret.exists():
        code = json.loads(receipt.read_text())['summary']['super']
        if '\n' in code or '\r' in code:
            raise RuntimeError('Invalid generated unlock material')
        with open(secret, 'x', opener=lambda p, f: os.open(p, f, 0o600)) as output:
            output.write('RSRS_SUPER=' + code + '\n')

    # Key initialization may have bootstrapped a host Runtime. Hand lifecycle
    # ownership to systemd before starting the supervised dedicated instance.
    unit = Path('/etc/systemd/system') / f'{name}.service'
    if not unit.exists():
        run(['runuser', '-u', name, '--', str(RELEASE / 'rsrs'),
             '--json', 'runtime', 'stop'], env=env, timeout=40)

    bins = library / 'bin'
    bins.mkdir(exist_ok=True)
    os.chown(bins, account.pw_uid, account.pw_gid)
    binary = bins / 'rsrs'
    if not binary.exists():
        binary.symlink_to(RELEASE / 'rsrs')
    models = library / 'models'
    models.mkdir(exist_ok=True)
    os.chown(models, account.pw_uid, account.pw_gid)
    model_link = models / PROFILE
    if not model_link.exists():
        model_link.symlink_to(model_root)
    if not unit.exists():
        unit.write_text(f'''[Unit]
Description=Respire dedicated memory Runtime for Sub2API user {user_id}
After=network.target
[Service]
Type=simple
User={name}
Group={name}
WorkingDirectory={home}
Environment=HOME={home}
Environment=RSRS_DATA_DIR={library}
Environment=RSRS_BIN_DIR={bins}
Environment=RSRS_RPC_PORT={port}
Environment=RSRS_NO_AUTOSYNC=1
Environment=RSRS_UPDATE_CHECK=0
EnvironmentFile={secret}
ExecStart={binary} runtime run
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
KillMode=control-group
UMask=0077
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths={home}
MemoryMax=384M
CPUQuota=100%
TasksMax=64
[Install]
WantedBy=multi-user.target
''')
        run(['systemctl', 'daemon-reload'])
    run(['systemctl', 'enable', '--now', name])
    deadline = time.monotonic() + 60
    while True:
        try:
            state = health(port)
            if state.get('store_ready'):
                break
        except OSError:
            pass
        if time.monotonic() >= deadline:
            raise RuntimeError(f'Runtime startup timed out for user {user_id}')
        time.sleep(1)
    if state.get('bin') != '1.0.20' or state.get('data_dir') != str(library):
        raise RuntimeError(f'Runtime identity mismatch for user {user_id}')
    catalog = rpc(port, 'model.catalog', [])['summary']
    active = catalog.get('active')
    if not active:
        rpc(port, 'model.switch', [json.dumps({'model': PROFILE, 'backend': 'onnx', 'engine': 'cpu'})])
        deadline = time.monotonic() + 180
        while True:
            progress = rpc(port, 'model.progress', [])['summary']
            state = progress.get('switch', {}).get('state')
            if state == 'ready':
                break
            if state == 'failed' or time.monotonic() >= deadline:
                raise RuntimeError(f'Model preparation failed for user {user_id}; inspect model.progress')
            time.sleep(1)
        active = rpc(port, 'model.catalog', [])['summary']['active']
    if active.get('model') != PROFILE:
        raise RuntimeError(f'Unexpected existing model for user {user_id}')
    return {'user_id': user_id, 'base_url': f'http://127.0.0.1:{port}', 'data_dir': str(library)}


def publish(bindings):
    MANIFEST.parent.mkdir(mode=0o750, exist_ok=True)
    group = grp.getgrnam('sub2api').gr_gid
    os.chown(MANIFEST.parent, 0, group)
    temporary = MANIFEST.with_suffix('.tmp')
    temporary.write_text(json.dumps(sorted(bindings, key=lambda b: b['user_id']), indent=2) + '\n')
    os.chown(temporary, 0, group)
    temporary.chmod(0o640)
    temporary.replace(MANIFEST)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--user-id', type=int, help='Provision only this existing active product user')
    parser.add_argument('--config', type=Path, default=Path('/app/data/config.yaml'))
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise RuntimeError('Run this deployment helper as root')
    with Path('/run/sub2api-memory-provision.lock').open('w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        config = yaml.safe_load(args.config.read_text())
        static = config.get('memory', {}).get('bindings', [])
        static_ids = {b['user_id'] for b in static}
        bindings = json.loads(MANIFEST.read_text()) if MANIFEST.exists() else []
        admitted = {b['user_id'] for b in bindings} | static_ids
        output = run(['runuser', '-u', 'postgres', '--', '/www/server/pgsql/bin/psql',
                      '-d', 'sub2api', '-At', '-v', 'ON_ERROR_STOP=1', '-c',
                      "SELECT u.id FROM users u LEFT JOIN memory_archive_tenants t ON t.user_id=u.id "
                      "WHERE u.deleted_at IS NULL AND u.status='active' AND NOT COALESCE(t.blocked,FALSE) ORDER BY u.id"])
        users = [int(line) for line in output.stdout.splitlines()]
        if args.user_id is not None:
            if args.user_id not in users:
                raise RuntimeError('Requested user is not active or has suspended memory')
            users = [args.user_id]
        added = []
        pending = [user_id for user_id in users if user_id not in admitted]
        if pending:
            model_root = prepare_models()
            for user_id in pending:
                available_kib = int(next(line.split()[1] for line in Path('/proc/meminfo').read_text().splitlines()
                                         if line.startswith('MemAvailable:')))
                if available_kib < 256 * 1024:
                    raise RuntimeError('Insufficient memory to prepare another dedicated Runtime; increase host capacity')
                bindings.append(provision(user_id, model_root))
                # Each publication contains only fully prepared independent tenants.
                publish(bindings)
                added.append(user_id)
        elif not MANIFEST.exists():
            publish(bindings)
        print(json.dumps({'configured_user_ids': sorted(admitted | set(added)), 'added': added}))


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, subprocess.SubprocessError, OSError, ValueError) as error:
        # CLI/HTTP bodies may contain recovery material. Only bounded operator
        # errors or the exception class reach the service journal.
        print(str(error) if isinstance(error, RuntimeError) else type(error).__name__, file=sys.stderr)
        sys.exit(1)
