#!/usr/bin/env bash
# 生产发布脚本（唯一入口）
#
# 用法:
#   ./deploy/publish-production.sh              # 构建并发布
#   VERSION=0.2.20 ./deploy/publish-production.sh
#   DRY_RUN=1 ./deploy/publish-production.sh    # 只构建，不连服务器
#   SKIP_BUILD=1 ./deploy/publish-production.sh # 复用已构建的产物
#   ARTIFACT_DIR=/path/to/ci-artifact ./deploy/publish-production.sh # 发布 CI 的原始产物
#
# 依赖: node, pnpm 9, go 1.27+, python3, ssh/scp（BatchMode 免密）
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER="${SERVER:-root@103.236.85.237}"
PUBLIC_BASE="${PUBLIC_BASE:-http://103.236.85.237:7788}"
VERSION="${VERSION:-0.2.21-gateway-$(date +%Y%m%d-%H%M)}"
PNPM_VERSION="${PNPM_VERSION:-9.15.9}"
LABEL="${LABEL:-$(printf '%s' "$VERSION" | tr '/: ' '---')}"
OUT="$REPO_ROOT/docs-local/build/sub2api-$LABEL"
DIST="$REPO_ROOT/backend/internal/web/dist"
RESOURCES="$REPO_ROOT/backend/resources"
DRY_RUN="${DRY_RUN:-0}"
SKIP_BUILD="${SKIP_BUILD:-0}"
ARTIFACT_DIR="${ARTIFACT_DIR:-}"

SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes)

log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarn:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

cd "$REPO_ROOT"

# ---------- 1. 预检 ----------
log "1/7 预检"
if [ -n "$ARTIFACT_DIR" ]; then
  command -v python3 >/dev/null || die "缺少 python3"
  OUT="$(cd "$ARTIFACT_DIR" && pwd)/sub2api"
  RESOURCES="$(dirname "$OUT")/resources"
  [ -f "$OUT.manifest.json" ] || die "缺少 CI 产物清单"
  VERSION="$(python3 - "$OUT" "$OUT.manifest.json" "$(git rev-parse HEAD)" <<'PY'
import hashlib, json, pathlib, sys
binary, manifest_path, commit = sys.argv[1:]
m = json.loads(pathlib.Path(manifest_path).read_text())
assert m['commit'] == commit, 'CI artifact commit differs from current checkout'
assert hashlib.sha256(pathlib.Path(binary).read_bytes()).hexdigest() == m['binary_sha256'], 'CI artifact hash mismatch'
assert m['version'] and m['entry'] in m['assets'], 'incomplete CI artifact manifest'
resources = pathlib.Path(binary).parent / 'resources'
assert m['resources'], 'missing runtime resources manifest'
assert {str(p.relative_to(resources)) for p in resources.rglob('*') if p.is_file()} == set(m['resources']), 'runtime resources differ from manifest'
for name, expected in m['resources'].items():
    assert hashlib.sha256((resources / name).read_bytes()).hexdigest() == expected, 'runtime resource hash mismatch'
print(m['version'])
PY
)"
  LABEL="${LABEL_OVERRIDE:-$VERSION}"
  SHA="$(python3 - "$OUT.manifest.json" <<'PY'
import json, sys
print(json.load(open(sys.argv[1]))['binary_sha256'])
PY
)"
  [ -z "$(git status --porcelain)" ] || die "CI 发布要求工作区无未提交改动"
  log "使用 CI 原始产物 $VERSION / $SHA"
else
for bin in node go python3; do command -v "$bin" >/dev/null || die "缺少 $bin"; done

[ -f frontend/pnpm-lock.yaml ] || die "缺少 frontend/pnpm-lock.yaml"

# pnpm 12 会丢弃 overrides，必须拦住
grep -q '^overrides:' frontend/pnpm-lock.yaml \
  || die "lockfile 缺少 overrides 段（曾被 pnpm 12 重写），先恢复正确 lock 再发布"
for pin in 'js-cookie: 3.0.7' 'decode-uri-component@<0.5.1'; do
  grep -q "$pin" frontend/pnpm-lock.yaml || die "lockfile 缺少安全覆盖: $pin"
done

PNPM=(pnpm)
if ! pnpm --version 2>/dev/null | grep -q '^9\.'; then
  command -v corepack >/dev/null || die "需要 pnpm 9（当前 $(pnpm --version 2>/dev/null || echo none)），且未找到 corepack"
  PNPM=(corepack "pnpm@$PNPM_VERSION")
  warn "当前 pnpm 不是 9.x，改用 ${PNPM[*]}"
fi

if [ -n "$(git status --porcelain)" ]; then
  warn "工作区有未提交改动，发布产物将无法对应任何 commit"
fi
log "版本 $VERSION / 标签 $LABEL"

# ---------- 2. 前端 ----------
if [ "$SKIP_BUILD" != "1" ]; then
  log "2/7 前端依赖与构建（frozen lockfile）"
  ( cd frontend && "${PNPM[@]}" install --frozen-lockfile && "${PNPM[@]}" run build )

  log "3/7 检查前端产物（Vite 直接输出到 embed 目录）"
else
  log "2-3/7 跳过构建（SKIP_BUILD=1）"
fi
[ -f "$DIST/index.html" ] || die "缺少 embed index.html"
ENTRY="$(python3 - "$DIST/index.html" <<'PY'
import pathlib, re, sys
html = pathlib.Path(sys.argv[1]).read_text()
match = re.search(r'/assets/index-[A-Za-z0-9_-]+\.js', html)
if not match:
    raise SystemExit('本地首页缺少入口 JS')
print(match.group())
PY
)"
[ -f "$DIST$ENTRY" ] || die "缺少本地入口 $ENTRY"

# ---------- 3. 后端 ----------
log "4/7 编译 linux/amd64（-tags embed）"
mkdir -p "$(dirname "$OUT")"
( cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -tags embed -trimpath \
    -ldflags "-s -w -X main.Version=$VERSION" \
    -o "$OUT" ./cmd/server )
SHA="$(shasum -a 256 "$OUT" | awk '{print $1}')"
SIZE="$(stat -f%z "$OUT" 2>/dev/null || stat -c%s "$OUT")"
log "二进制 $OUT"
log "sha256 $SHA ($SIZE bytes)"
python3 - "$OUT" "$DIST" "$ENTRY" "$VERSION" "$RESOURCES" <<'PY'
import hashlib, json, pathlib, subprocess, sys
binary, dist, entry, version, resources = sys.argv[1:]
root = pathlib.Path(dist)
resources = pathlib.Path(resources)
manifest = {
    'version': version,
    'commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
    'source_dirty': bool(subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip()),
    'entry': entry,
    'binary_sha256': hashlib.sha256(pathlib.Path(binary).read_bytes()).hexdigest(),
    'assets': {'/' + str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest()
               for p in root.rglob('*') if p.is_file() and p.suffix in {'.js', '.css', '.svg', '.png', '.webp', '.woff2'}},
    'resources': {str(p.relative_to(resources)): hashlib.sha256(p.read_bytes()).hexdigest()
                  for p in resources.rglob('*') if p.is_file()},
}
pathlib.Path(binary + '.manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
PY
fi

[[ "$LABEL" =~ ^[A-Za-z0-9._-]+$ ]] || die "release 标签仅允许字母、数字、点、横线和下划线"

if [ "$DRY_RUN" = "1" ]; then
  log "DRY_RUN=1，跳过发布"
  exit 0
fi

# Each release keeps a consistent database snapshot and both effective/fallback
# configurations. Startup applies embedded migrations after the pointer switch.
log "发布前备份数据库与配置"
ssh "${SSH_OPTS[@]}" "$SERVER" python3 - "$LABEL" < "$REPO_ROOT/deploy/backup-production.py"

# ---------- 4. 上传 ----------
log "5/7 上传到 $SERVER"
scp "${SSH_OPTS[@]}" "$OUT" "$SERVER:/tmp/sub2api-$LABEL"
scp "${SSH_OPTS[@]}" "$OUT.manifest.json" "$SERVER:/tmp/sub2api-$LABEL.manifest.json"
scp "${SSH_OPTS[@]}" -r "$RESOURCES" "$SERVER:/tmp/sub2api-$LABEL.resources"

# ---------- 5. 切换 ----------
log "6/7 安装 release 并原子切换符号链接"
PREV="$(ssh "${SSH_OPTS[@]}" "$SERVER" 'readlink -f /opt/sub2api/sub2api')"
PREV_RESOURCES="$(ssh "${SSH_OPTS[@]}" "$SERVER" 'if [ -e /opt/sub2api/resources ]; then readlink -f /opt/sub2api/resources; fi')"
ssh "${SSH_OPTS[@]}" "$SERVER" bash -s -- "$LABEL" "$SHA" <<'REMOTE'
set -euo pipefail
LABEL="$1"; SHA="$2"
RELEASE="/opt/sub2api/releases/$LABEL"
[ ! -e "$RELEASE/sub2api" ] || { echo "release 已存在: $RELEASE"; exit 1; }
install -d -o sub2api -g sub2api -m 755 "$RELEASE"
install -o sub2api -g sub2api -m 755 "/tmp/sub2api-$LABEL" "$RELEASE/sub2api"
install -o sub2api -g sub2api -m 644 "/tmp/sub2api-$LABEL.manifest.json" "$RELEASE/release-manifest.json"
install -d -o sub2api -g sub2api -m 755 "$RELEASE/resources"
cp -R "/tmp/sub2api-$LABEL.resources/." "$RELEASE/resources/"
chown -R sub2api:sub2api "$RELEASE/resources"
ACTUAL="$(sha256sum "$RELEASE/sub2api" | awk '{print $1}')"
[ "$ACTUAL" = "$SHA" ] || { echo "远端哈希不一致: $ACTUAL"; exit 1; }
python3 - "$RELEASE" <<'PY'
import hashlib, json, pathlib, sys
root = pathlib.Path(sys.argv[1])
manifest = json.loads((root / 'release-manifest.json').read_text())
resources = root / 'resources'
assert {str(p.relative_to(resources)) for p in resources.rglob('*') if p.is_file()} == set(manifest['resources']), 'runtime resources differ from manifest'
for name, expected in manifest['resources'].items():
    assert hashlib.sha256((root / 'resources' / name).read_bytes()).hexdigest() == expected, name
PY
[ ! -e /opt/sub2api/resources ] || [ -L /opt/sub2api/resources ] || { echo "resources 必须不存在或为发布符号链接"; exit 1; }
PREVIOUS_BINARY="$(readlink -f /opt/sub2api/sub2api)"
PREVIOUS_RESOURCES=""
if [ -e /opt/sub2api/resources ]; then PREVIOUS_RESOURCES="$(readlink -f /opt/sub2api/resources)"; fi
restore_release_on_error() {
  trap - ERR
  ln -sfn "$PREVIOUS_BINARY" /opt/sub2api/.sub2api.new
  mv -Tf /opt/sub2api/.sub2api.new /opt/sub2api/sub2api
  if [ -n "$PREVIOUS_RESOURCES" ]; then
    ln -sfn "$PREVIOUS_RESOURCES" /opt/sub2api/.resources.new
    mv -Tf /opt/sub2api/.resources.new /opt/sub2api/resources
  else
    rm -f /opt/sub2api/resources
  fi
  systemctl restart sub2api || true
}
trap restore_release_on_error ERR
ln -sfn "$RELEASE/resources" /opt/sub2api/.resources.new
mv -Tf /opt/sub2api/.resources.new /opt/sub2api/resources
ln -sfn "$RELEASE/sub2api" /opt/sub2api/.sub2api.new
mv -Tf /opt/sub2api/.sub2api.new /opt/sub2api/sub2api
systemctl restart sub2api
for i in $(seq 1 15); do
  systemctl is-active --quiet sub2api && break
  sleep 1
done
systemctl is-active sub2api
systemctl show sub2api -p MainPID -p NRestarts --value | paste -sd' ' -
trap - ERR
rm -f "/tmp/sub2api-$LABEL" "/tmp/sub2api-$LABEL.manifest.json"
rm -rf "/tmp/sub2api-$LABEL.resources"
REMOTE

# ---------- 6. 验收 ----------
log "7/7 公网验收 $PUBLIC_BASE"
python3 - "$PUBLIC_BASE" "$OUT.manifest.json" <<'PY'
import hashlib, json, pathlib, re, sys, time, urllib.request
base, manifest_path = sys.argv[1:]
base = base.rstrip('/')
manifest = json.loads(pathlib.Path(manifest_path).read_text())
entry = manifest['entry']
expected = manifest['assets'][entry]
for attempt in range(15):
    try:
        with urllib.request.urlopen(base + '/health', timeout=8) as response:
            assert json.load(response)['status'] == 'ok'
        break
    except (OSError, AssertionError):
        if attempt == 14:
            raise
        time.sleep(1)
nonces = []
for _ in range(2):
    with urllib.request.urlopen(base + '/home', timeout=8) as response:
        html = response.read().decode()
        assert entry in html, '线上入口与本次构建不同'
        assert 'no-store' in {v.strip().lower() for v in response.headers.get('Cache-Control', '').split(',')}, 'HTML 缓存策略不正确'
        assert response.headers.get('ETag') is None, 'HTML 不应带 ETag'
        nonce = re.search(r"'nonce-([^']+)'", response.headers.get('Content-Security-Policy', ''))
        inline = re.search(r'<script[^>]*nonce="([^"]+)"[^>]*>\s*window\.__APP_CONFIG__', html)
        assert nonce and inline and nonce[1] == inline[1], 'CSP 与配置脚本 nonce 不匹配'
        nonces.append(nonce[1])
assert nonces[0] != nonces[1], 'CSP nonce 未逐次刷新'
with urllib.request.urlopen(base + entry, timeout=15) as response:
    actual = hashlib.sha256(response.read()).hexdigest()
    asset_cache = response.headers.get('Cache-Control', '')
    assert {'public', 'max-age=31536000', 'immutable'} <= {part.strip().lower() for part in asset_cache.split(',')}, '入口静态资源缓存策略不正确'
assert actual == expected, '线上入口内容与本次构建不同'
print(json.dumps({'health': 'ok', 'entry': entry, 'entry_sha256': actual,
                  'cache_control': 'no-store', 'asset_cache_control': asset_cache,
                  'csp_nonce_verified': True}))
PY

log "完成"
ROLLBACK_RESOURCES=""
if [ -n "$PREV_RESOURCES" ]; then
  ROLLBACK_RESOURCES="ln -sfn $PREV_RESOURCES /opt/sub2api/.resources.new && mv -Tf /opt/sub2api/.resources.new /opt/sub2api/resources && "
else
  log "上一个 release 无资源目录；二进制回滚时保留本次已核实的资源"
fi
printf '\n  版本    %s\n  release /opt/sub2api/releases/%s\n  sha256  %s\n  回滚    ssh %s "%sln -sfn %s /opt/sub2api/.sub2api.new && mv -Tf /opt/sub2api/.sub2api.new /opt/sub2api/sub2api && systemctl restart sub2api"\n\n' \
  "$VERSION" "$LABEL" "$SHA" "$SERVER" "$ROLLBACK_RESOURCES" "$PREV"
