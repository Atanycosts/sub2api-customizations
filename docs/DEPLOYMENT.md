# 部署

本文件是本仓库**部署事实的唯一来源**。改发布方式、验收标准或回滚流程，只改这里和 `deploy/publish-production.sh`。

- `deploy/README.md` 描述的是上游提供的通用安装方式（Docker Compose / Apple container / 一键二进制安装），与本项目生产发布是并行的另一条路径，不作为本项目的操作依据。
- `docs-local/` 下的历次部署记录（`ui-deployment-*`、`logo-deployment-*`）是发布当时的证据归档，不是操作指南，也不入库。

## 1. 生产现状

2026-11-08（Asia/Shanghai）核实：

| 项 | 值 |
|---|---|
| 地址 | `http://103.236.85.237:7788` |
| 服务 | `sub2api.service`（systemd，独立用户 `sub2api`） |
| 生效二进制 | `/opt/sub2api/sub2api` → `/opt/sub2api/releases/<label>/sub2api` |
| 当前 release | `memory-all-users-20261107`，sha256 `eeb0f8cb957bd185a5a48ce6a36004e8cd9a03f693f076189f4dd29300e93119`；入口 `/assets/index-D-gpDLrb.js` |
| 来源提交 | `4bc89767b4b1a33b9643a1e264c458f5ffd0ef02`（PR #4；上游基线 `6b4b37db4`） |
| 页面版本 | `v0.2.20-ui-memory`（版本号不带时间；release 目录保留独立标识） |
| 依赖 | 同机 PostgreSQL（开机自动执行迁移）+ Redis DB 0 |
| 配置 | 生效 `/app/data/config.yaml`；回退 `/opt/sub2api/config.yaml` |
| 回滚点 | `/opt/sub2api/releases/upstream-0.2.20-20261107/sub2api`（`v0.2.20-ui-memory`）及 `/opt/sub2api/backups/memory-all-users-20261107/` 中的生效配置副本 |

本次发布已核对 187 个静态资源、公开版本及全部 8 个活跃账号的记忆状态（configured/active）。只在生效 YAML 增加 `memory.bindings_file`，回退配置字节不变；301 次迁移、8 个用户、18 个上游账号保持。维护期间用户 8 新增了一把普通 API Key（id 16，2026-11-07 23:51:51 +08:00），总数由 15 到 16，部署未创建密钥。Respire 同步与用户 1 Runtime 的进程、启动时间和重启计数均未变化。该更新没有新增数据库迁移。

用户 2–8 已使用共享编码与独立库，新账号准备 timer 已启用。测量时新增 7 个账号进程及共享编码器合计约 125 MB，原用户 1 的 M3 服务仍约 1.5 GB。真实用户 7 的 HTTP Responses 合成请求完成、队列 completed、来源日记读回与共享语义检索通过；其他账号查询同一事件返回 404，测试日记已 tombstone，队列原文已清除。另一次用户 4 所属分组请求返回上游 503，因此该请求未作为完成归档的证据。没有据此声称已验证回答质量或成本收益。

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

1. 预检：node / go / python3 存在；**lockfile 必须含 `overrides` 段**（pnpm 12 会静默丢弃，见 §7）
2. `pnpm install --frozen-lockfile` + `pnpm run build`（pnpm 非 9.x 时自动切 `corepack pnpm@9.15.9`）
3. 检查 `backend/internal/web/dist/` 的首页及入口 JS；Vite 直接输出到该目录
4. `CGO_ENABLED=0 GOOS=linux go build -tags embed -trimpath -ldflags "-X main.Version=<version>"`
5. 生成包含二进制及静态资源 sha256 的 `release-manifest.json`，上传到 `/tmp`，远端校验后落到新的 `releases/<label>/`（不覆盖已有 release）
6. `ln -sfn` + `mv -Tf` 原子切换 `/opt/sub2api/sub2api`，重启服务并等待 active
7. 公网验收：`/health`、首页入口 JS 与本次清单的名称及 sha256 一致、哈希入口长期缓存、HTML 无缓存、CSP 配置脚本 nonce 一致且每次刷新

脚本不打印任何密钥，不修改 `config.yaml`，不动数据库。

## 3. 验收标准

发布脚本已内置前四项；涉及后端改动时补后三项。

| 检查 | 通过条件 |
|---|---|
| 健康 | `GET /health` = `{"status":"ok"}` |
| 入口 | 首页 `index-*.js` 返回 200，且与本次 `release-manifest.json` 同名、同 sha256 |
| 缓存 | HTML `Cache-Control: no-store`，无 `ETag`；哈希入口包含 `public, max-age=31536000, immutable` |
| CSP | 响应头 `nonce-X` 与内联 `__APP_CONFIG__` 的 `nonce` 相同，连续两次请求不同 |
| 静态资源 | 抽样或全量与 `release-manifest.json` 的 sha256 一致 |
| 服务 | `ActiveState=active`，`NRestarts` 不增长 |
| 数据 | 迁移数 / 用户数 / 上游账号数与发布前一致 |
| 配置 | 普通发布 `config.yaml` 字节不变；本次账号开通只允许 §8 的 `memory.bindings_file` 增量，并与备份逐项比较 |

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
- 记忆服务（Respire）同机独立进程：同步 `8787`、用户 1 M3 Runtime `15170`，用户 2–8 轻量 Runtime `15171`–`15177`；共享编码 `15168`。均监听回环，服务安装与网关二进制发布分开，详见 §8。
- lockfile 必须由 pnpm 9 维护；用 pnpm 12 执行任何安装都会重写并丢弃 `overrides`，发布脚本已加断言拦截。

## 8. 全站账号记忆的配套服务

网关仍通过本文件 §2 发布。账号准备和共享编码是独立 systemd 服务；模型准备完毕、绑定清单可由 `sub2api` 用户读取后，才在生效配置的 `memory` 段增加 `bindings_file: /etc/sub2api/memory-bindings.json`。静态用户 1 的库、密钥、M3 模型和同步保持原绑定。只增加绑定清单这一项配置，回退 YAML 保持原字节。

共享服务使用服务器 Python 3.12 独立 venv：`/opt/respire-runtime/embedding-venv`，依赖固定于 `deploy/memory-embeddings-requirements.txt`。安装脚本为 `/opt/respire-runtime/memory-embeddings.py`，unit 来源 `deploy/sub2api-memory-embeddings.service`；服务用户 `sub2api-embedding`，home `/var/lib/sub2api-embedding`，只监听 `127.0.0.1:15168`，不向公网开放。`GET /health` 返回模型身份；`POST /v1/embeddings` 为仅在内存中处理文本的 OpenAI 兼容编码接口，没有存储、记忆查询或日志正文。

模型目录 `/opt/respire-runtime/models/bge-small-zh-v1.5`，来源 `Xenova/bge-small-zh-v1.5` 的固定 revision `75c43b069aac4d136ba6bc1122f995fedcfd2781`。启动核对以下完整 SHA-256，不接受同名替换：

| 文件 | SHA-256 |
| --- | --- |
| `tokenizer.json` | `48cea5d44424912a6fd1ea647bf4fe50b55ab8b1e5879c3275f80e339e8fae26` |
| `onnx/model_quantized.onnx` | `15b717c382bcb518ba457b93ea6850ede7f4f1cd8937454aa06972366cd19bcc` |

账号供应脚本安装为 `/opt/sub2api/provision-account-memory.py`，依赖宿主 PyYAML 和现有 PostgreSQL CLI；unit/timer 来源 `deploy/sub2api-memory-provision.{service,timer}`。脚本只选取未删除、active 且未主动停用记忆的产品账号。新账号有独立 OS 用户 `sub2api-memory<ID>`、home `/var/lib/sub2api-memory<ID>`、root-only 密钥文件 `/etc/sub2api-memory<ID>.env` 和回环端口 `15169+ID`；端口超出 TCP 范围时明确失败。初始化密钥输出直接写 root-only 回执，不打印到日志。固定 Respire Runtime `1.0.20` 通过 `host_transport/openai_embeddings` 使用共享模型；每个账号不再单独加载模型权重。

```sh
systemctl enable --now sub2api-memory-embeddings
python3 /opt/sub2api/provision-account-memory.py
# 仅将此前准备的 small-model 管理库切换到共享编码，静态绑定不参与：
python3 /opt/sub2api/provision-account-memory.py --reconcile-bindings
python3 deploy/tests/memory-embeddings-smoke.py
systemctl enable --now sub2api-memory-provision.timer
```

新绑定只有通过版本、库目录和已激活模型检查后才写入 `/etc/sub2api/memory-bindings.json`（目录 root:sub2api 0750，文件 0640，原子替换）。供应每分钟运行一次，网关每 5 秒读取新增绑定。已加载绑定不因文件缩减而移除；停用仍由数据库 fence 控制，不能用编辑清单模拟删除或改绑。

本链路没有新增数据库迁移。验收需覆盖所有活跃账号状态、真实来源写入/读回、合成相关与无关查询、跨账号及并发隔离、共享编码故障、服务重启和静态用户 1/密文同步的进程不变。账号状态接口中的 active 只表示配置与停用状态，不是每次 Runtime 健康检查。回答效果、项目范围和成本收益需要单独实验，不能以归档成功或共享编码 smoke 测试代替。

回滚网关时切回上一 release 并恢复备份的生效 YAML；既有独立库和密钥保留。停用供应 timer 可阻止新增账号准备。不要删除账号库或覆盖密钥作为回滚手段。
