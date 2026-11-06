# 部署

本文件是本仓库**部署事实的唯一来源**。改发布方式、验收标准或回滚流程，只改这里和 `deploy/publish-production.sh`。

- `deploy/README.md` 描述的是上游提供的通用安装方式（Docker Compose / Apple container / 一键二进制安装），与本项目生产发布是并行的另一条路径，不作为本项目的操作依据。
- `docs-local/` 下的历次部署记录（`ui-deployment-*`、`logo-deployment-*`）是发布当时的证据归档，不是操作指南，也不入库。

## 1. 生产现状

2026-11-07 核实：

| 项 | 值 |
|---|---|
| 地址 | `http://103.236.85.237:7788` |
| 服务 | `sub2api.service`（systemd，独立用户 `sub2api`） |
| 生效二进制 | `/opt/sub2api/sub2api` → `/opt/sub2api/releases/<label>/sub2api` |
| 当前 release | `logo-20261107-0120`，sha256 `1b01b69e…5cb753082` |
| 依赖 | 同机 PostgreSQL（开机自动执行迁移）+ Redis DB 0 |
| 配置 | 生效 `/app/data/config.yaml`；回退 `/opt/sub2api/config.yaml` |
| 回滚点 | `/opt/sub2api/releases/memory-ui-20261106-1100/sub2api` |

目录约定：

```
/opt/sub2api/
  sub2api              -> releases/<label>/sub2api   （唯一可变指针）
  releases/<label>/    # 每次发布一个目录，不覆盖历史
  backups/<label>/     # 配置副本与回滚元数据（root only）
  config.yaml          # 回退配置
/app/data/config.yaml  # 实际生效配置
```

## 2. 发布（唯一路径）

```sh
./deploy/publish-production.sh                 # 构建 + 发布 + 验收
VERSION=0.2.20 ./deploy/publish-production.sh  # 指定版本
DRY_RUN=1 ./deploy/publish-production.sh       # 只构建，不连服务器
```

脚本按顺序做七件事，任何一步失败即中止：

1. 预检：node / go / rsync 存在；**lockfile 必须含 `overrides` 段**（pnpm 12 会静默丢弃，见 §7）
2. `pnpm install --frozen-lockfile` + `pnpm run build`（pnpm 非 9.x 时自动切 `corepack pnpm@9.15.9`）
3. `rsync --delete --exclude .keep` 同步 `frontend/dist/` → `backend/internal/web/dist/`
4. `CGO_ENABLED=0 GOOS=linux go build -tags embed -trimpath -ldflags "-X main.Version=<version>"`
5. 上传到 `/tmp`，远端校验 sha256 后落到 `releases/<label>/sub2api`
6. `ln -sfn` + `mv -Tf` 原子切换 `/opt/sub2api/sub2api`，重启服务并等待 active
7. 公网验收：`/health`、首页入口 JS、`Cache-Control`

脚本不打印任何密钥，不修改 `config.yaml`，不动数据库。

## 3. 验收标准

发布脚本已内置前四项；涉及后端改动时补后三项。

| 检查 | 通过条件 |
|---|---|
| 健康 | `GET /health` = `{"status":"ok"}` |
| 入口 | 首页 `index-*.js` 返回 200，且与本地 `frontend/dist` 同名 |
| 缓存 | HTML `Cache-Control: no-store`，无 `ETag` |
| CSP | 响应头 `nonce-X` 与内联 `__APP_CONFIG__` 的 `nonce` 相同，连续两次请求不同 |
| 静态资源 | 抽样或全量与 `release-manifest.json` 的 sha256 一致 |
| 服务 | `ActiveState=active`，`NRestarts` 不增长 |
| 数据 | 迁移数 / 用户数 / 上游账号数与发布前一致 |
| 配置 | `config.yaml` 字节不变 |

## 4. 回滚

```sh
ssh root@103.236.85.237 '
  ln -sfn <上一个 release>/sub2api /opt/sub2api/.sub2api.new &&
  mv -Tf /opt/sub2api/.sub2api.new /opt/sub2api/sub2api &&
  systemctl restart sub2api &&
  curl -fsS http://127.0.0.1:7788/health'
```

纯前端/UI 回滚只需切符号链接；**若新版本执行过数据库迁移**，先确认旧二进制兼容当前 schema，再回滚，否则会启动失败。不要用 `cp` 覆盖正在运行的二进制。

## 5. 其他部署形态（不属于本项目维护范围）

| 形态 | 入口 | 状态 |
|---|---|---|
| Docker Compose | `deploy/docker-compose*.yml`、`deploy/docker-deploy.sh` | 上游提供，本项目未使用 |
| Apple container | `deploy/apple-container.sh` | 上游提供，本项目未使用 |
| 一键二进制安装 | `deploy/install.sh` | 上游提供；会直接改写 `/opt/sub2api`，与本项目的 `releases/` 布局冲突，禁止在生产使用 |
| GoReleaser 制品 | `.goreleaser.yaml`、`Dockerfile.goreleaser` | 上游发布流水线，本项目未使用 |
| **本项目生产** | `deploy/publish-production.sh` | **唯一使用** |

## 6. 为什么不是把重复文件删掉

`deploy/` 下确实存在多套并行形态（4 份 compose、3 个 Dockerfile、3 个 Makefile、5 个安装脚本）。但它们不是本项目写的：

- `.github/workflows/backend-ci.yml` 直接执行 `deploy/apple-container.sh` 与 `deploy/tests/*.sh`；
- 上游 `release.yml` 引用 `deploy/DOCKER.md`；
- 这些文件是上游资产，删除会在下次同步上游时产生冲突，也会让 CI 失效。

所以收敛方式是**指定真源 + 标注其余**，而不是物理删除：生产只认本文件与 `publish-production.sh`，其余保留为上游能力。

## 7. 已知限制

- 公网仅 HTTP，`7788` 无 TLS；证书签发受线路/备案限制（见 `respire-deploy` 记录）。
- 单机单实例，无蓝绿或灰度；发布期间有秒级中断。
- 迁移随进程启动自动执行，回滚前必须确认 schema 兼容。
- 记忆服务（Respire）同机独立进程，仅监听回环 `8787`，不在本发布链路内。
- lockfile 必须由 pnpm 9 维护；用 pnpm 12 执行任何安装都会重写并丢弃 `overrides`，发布脚本已加断言拦截。
