#!/usr/bin/env bash
# 生产发布脚本（唯一入口）
#
# 用法:
#   ./deploy/publish-production.sh              # 构建并发布
#   VERSION=0.2.20 ./deploy/publish-production.sh
#   DRY_RUN=1 ./deploy/publish-production.sh    # 只构建，不连服务器
#   SKIP_BUILD=1 ./deploy/publish-production.sh # 复用已构建的产物
#
# 依赖: node, pnpm 9, go 1.27+, rsync, ssh/scp（BatchMode 免密）
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER="${SERVER:-root@103.236.85.237}"
PUBLIC_BASE="${PUBLIC_BASE:-http://103.236.85.237:7788}"
VERSION="${VERSION:-0.2.19-$(date +%Y%m%d-%H%M)}"
PNPM_VERSION="${PNPM_VERSION:-9.15.9}"
LABEL="${LABEL:-$(printf '%s' "$VERSION" | tr '/: ' '---')}"
OUT="$REPO_ROOT/docs-local/build/sub2api-$LABEL"
DRY_RUN="${DRY_RUN:-0}"
SKIP_BUILD="${SKIP_BUILD:-0}"

SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10)

log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarn:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

cd "$REPO_ROOT"

# ---------- 1. 预检 ----------
log "1/7 预检"
for bin in node go rsync; do command -v "$bin" >/dev/null || die "缺少 $bin"; done

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

  log "3/7 同步前端产物到 embed 目录"
  # --exclude .keep: go:embed 需要该占位文件，dist 其余内容整体替换
  rsync -a --delete --exclude '.keep' frontend/dist/ backend/internal/web/dist/
  [ -f backend/internal/web/dist/index.html ] || die "dist 同步失败"
else
  log "2-3/7 跳过构建（SKIP_BUILD=1）"
fi

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

if [ "$DRY_RUN" = "1" ]; then
  log "DRY_RUN=1，跳过发布"
  exit 0
fi

# ---------- 4. 上传 ----------
log "5/7 上传到 $SERVER"
scp "${SSH_OPTS[@]}" "$OUT" "$SERVER:/tmp/sub2api-$LABEL"

# ---------- 5. 切换 ----------
log "6/7 安装 release 并原子切换符号链接"
PREV="$(ssh "${SSH_OPTS[@]}" "$SERVER" 'readlink -f /opt/sub2api/sub2api')"
ssh "${SSH_OPTS[@]}" "$SERVER" bash -s -- "$LABEL" "$SHA" <<'REMOTE'
set -euo pipefail
LABEL="$1"; SHA="$2"
RELEASE="/opt/sub2api/releases/$LABEL"
install -d -o sub2api -g sub2api -m 755 "$RELEASE"
install -o sub2api -g sub2api -m 755 "/tmp/sub2api-$LABEL" "$RELEASE/sub2api"
ACTUAL="$(sha256sum "$RELEASE/sub2api" | awk '{print $1}')"
[ "$ACTUAL" = "$SHA" ] || { echo "远端哈希不一致: $ACTUAL"; exit 1; }
ln -sfn "$RELEASE/sub2api" /opt/sub2api/.sub2api.new
mv -Tf /opt/sub2api/.sub2api.new /opt/sub2api/sub2api
systemctl restart sub2api
for i in $(seq 1 15); do
  systemctl is-active --quiet sub2api && break
  sleep 1
done
systemctl is-active sub2api
systemctl show sub2api -p MainPID -p NRestarts --value | paste -sd' ' -
rm -f "/tmp/sub2api-$LABEL"
REMOTE

# ---------- 6. 验收 ----------
log "7/7 公网验收 $PUBLIC_BASE"
HEALTH="$(curl -fsS --max-time 8 "$PUBLIC_BASE/health" || true)"
[ "$HEALTH" = '{"status":"ok"}' ] || die "健康检查失败: ${HEALTH:-<empty>}"
log "health ok"

HOME_HTML="$(curl -fsS --max-time 8 "$PUBLIC_BASE/home")"
ENTRY="$(printf '%s' "$HOME_HTML" | grep -o '/assets/index-[A-Za-z0-9_-]*\.js' | head -1)"
[ -n "$ENTRY" ] || die "首页未找到入口 JS"
CODE="$(curl -fsS -o /dev/null -w '%{http_code}' --max-time 8 "$PUBLIC_BASE$ENTRY")"
[ "$CODE" = "200" ] || die "入口 $ENTRY 返回 $CODE"
CACHE="$(curl -fsS -D - -o /dev/null --max-time 8 "$PUBLIC_BASE/home" | tr -d '\r' | awk -F': ' 'tolower($1)=="cache-control"{print $2}')"
log "入口 $ENTRY (200)，cache-control: ${CACHE:-none}"

log "完成"
printf '\n  版本    %s\n  release /opt/sub2api/releases/%s\n  sha256  %s\n  回滚    ssh %s "ln -sfn %s /opt/sub2api/.sub2api.new && mv -Tf /opt/sub2api/.sub2api.new /opt/sub2api/sub2api && systemctl restart sub2api"\n\n' \
  "$VERSION" "$LABEL" "$SHA" "$SERVER" "$PREV"
