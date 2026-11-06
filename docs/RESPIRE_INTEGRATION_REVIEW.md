# Respire 服务端记忆实现审阅与验证

日期：2026-11-06（Asia/Shanghai）。本文件记录 macOS 工作区、PostgreSQL 16 临时独立实例和最终源码的实现审阅；真实生产发布验收另见 [docs-local/respire-deployment-20261106/FINAL-DEPLOYMENT-STATUS.md](../docs-local/respire-deployment-20261106/FINAL-DEPLOYMENT-STATUS.md)。实施配置和下一阶段见 [RESPIRE_INTEGRATION_PLAN.md](RESPIRE_INTEGRATION_PLAN.md)。

## 本次交付范围

已实现按产品用户隔离的服务端 Responses HTTP 文本记忆：固定版 Respire 适配、检索注入、最终响应采集、持久队列、来源日记写入核对及用户停用。源码默认关闭，生产通过显式 user 绑定灰度启用；worker 只在应用完整初始化成功后启动。

多人开发后由独立验证者检查边界并补充反例；最终结果以协调后的测试为准。前端首页及 web embed 文件由其他进程推进，本次未修改或验收这些改动。未提交 Git commit，未更改线上服务，也未接入个人记忆库。

## 边界与证据

| 边界 | 已实现的不变量 | 验证证据 | 未证明的范围 |
| --- | --- | --- | --- |
| 身份 | 只取鉴权产品 user_id；跨 Key/模型不另建租户；不同用户不回退共享库 | handler/auth unit、真实数据库双用户测试 | 生产宿主进程/凭据/端口隔离 |
| 请求接入 | 纯文本 eligibility；检索注入一次；failover 共用快照；原始正文用于已有缓存身份 | 真实 handler + Forward + 两上游 HTTP fixture | 在线模型前缀命中或成本收益 |
| 检索 | 超时有界；前后代次核验；错误继续正常转发；日记 locator/版本/Unicode 范围保留 | provider contract、handler unit | 真 Core 检索质量、索引延迟和规模性能 |
| 参考注入 | 独立 user 数据消息；包装开销计入预算；instructions 与原始用户文字保留 | Unicode/tokenizer 回归、出站正文检查 | 任意 tokenizer 精确计数、模型完全抵御错误参考 |
| 响应采集 | 观察下游响应 writer 接受的最终响应字节；完整纯文本终态才归档；工具、断流、错误、过量、歧义 JSON 均跳过 | JSON/SSE hostile fixtures、Write/WriteString 和跨协议转换测试 | 网络最终送达；真实模型/代理网络全部行为 |
| 幂等和租约 | 唯一用户事件+摘要；随机租约 token；外部提交前落盘；旧租约不可完成 | PostgreSQL component、同 worker 重新领取和并发测试 | 客户端独立重发的逻辑去重、引擎 exactly-once |
| 未知写入 | 先查 runtime.result 并核对来源读回；未确认不自动重写；有限核对 | PostgreSQL+适配器 HTTP fixture 的提交后丢回复测试 | Runtime 重启/64 条逐出后的自动恢复保证 |
| 来源归档 | 用户陈述和未验证模型回答分开；trivial 来源日记；回执与读回匹配 | adapter contract、PostgreSQL 组合测试 | LLM 提炼、important 主记忆维护或来源内容真实性 |
| 停用 | 永久阻断新检索/入队；增加代次；清空排队原文；旧任务不可提交完成状态 | PostgreSQL 阻断并发、RPC 中停用、禁用/删除用户测试 | 已提交 Runtime 写入的物理取消；已转发上下文追回；全副本删除 |
| 原文保留 | 完成/失败/取消擦除；其余 7 天，Claim 时维护；每用户未解决上限 1000 | PostgreSQL 清理、失效租约恢复顺序、容量测试 | worker 停机时准时擦除、永久回执容量清理 |
| 生命周期 | 构造不启动；完整应用初始化后启动；退出取消并等待；Noop 不得伪完成 | Wire/入口顺序源码核对、nil cleanup 测试、worker RunOnce component、追加真实二进制启停及归档烟测 | 真实 Runtime 服务生命周期、生产重启和迁移演练 |

## 验证中修正的问题

- 队列领取增加随机租约 token，防止同一 worker 重领后旧执行仍完成；等待行锁后的租约判断使用真实时钟。
- 提交前持久化 `submitted`；提交后崩溃、超时或 provider panic 进入核对，未知结果不重放。Noop provider 不允许把任务处理为成功。
- 先恢复过期 processing/submitted，再执行原文 TTL，避免旧任务恢复后绕过 7 天限制。
- provider 主记忆不替代显式日记检索；两类结果交错保留，日记 fragment 元数据不降为不完整短 ID。
- JSON 重复键、转义键和 Unicode 大小写折叠别名可能被不同解析器解释为不同请求/响应。记忆参与层增加递归歧义检查；覆盖 `\u017f` 等 simple-fold 反例，正常转发不改写。
- SSE 在 completed 前收到 `[DONE]`、终态后继续出现 JSON、canceled/cancelled、incomplete、error、工具输出或写入失败均不归档。
- 控制接口仍需有效用户和密钥鉴权，但查询/停用不要求模型余额、订阅、计费额度或未到期；避免用户因费用状态无法停用记忆。
- 全路由测试发现 suspend POST 未登记提示词审计分类，已补为无模型提示词的管理操作；保留原有审计覆盖检查。

## 本次验证记录

完整 unit 覆盖配置、service、handler、middleware、routes、cmd/server。首次完整运行中其他包通过，routes 发现上述新管理接口分类遗漏；修正后完整 routes 包通过。service 与 handler 完整包分别耗时约 231 秒和 48 秒。Redis fail-close/fail-open 用例中的连接拒绝日志是故障注入，不能单独当成测试失败。

```sh
cd backend
go test -p 2 -tags unit ./internal/config ./internal/service ./internal/handler \
  ./internal/server/middleware ./internal/server/routes ./cmd/server \
  -count=1 -timeout 240s
go test -p 2 -tags unit ./internal/server/routes -count=1 -timeout 90s
```

真实 PostgreSQL component 覆盖 13 个队列场景和 2 个组合场景：从入队、适配器归档回执、show 读回、完成清原文到再次召回；以及已提交但丢失 HTTP 回复后只核对、不重复写。Runtime HTTP 为固定稳定协议 fixture，并非真 Respire Core。独立数据库验证者的 15 项全部通过；最终来源冻结后加 `-race` 再运行 15 项，全部通过（2.788 秒）。

```sh
SUB2API_MEMORY_TEST_DSN='<专用临时 PostgreSQL DSN>' \
go test -p 2 -race -tags memorycomponent ./internal/repository \
  -run '^(TestMemoryArchivePostgres|TestMemoryRoundtripPostgres)' \
  -count=1 -timeout 90s -v
```

定向并发检测覆盖记忆及已有兼容缓存/终态回归，配置、service、handler、middleware 和 cmd/server 五个包全部通过（各约 1.4–2.6 秒）：

```sh
go test -p 2 -race -tags unit ./internal/config ./internal/service \
  ./internal/handler ./internal/server/middleware ./cmd/server \
  -run '^(TestMemory|TestRespireMemory|TestEstimateMemoryTokens|TestProvideCleanup|TestDeriveCompatPromptCacheKey|TestDeriveAnthropicCompatPromptCacheKey|TestOpenAIStreamingPassthroughResponseIncomplete)' \
  -count=1 -timeout 240s
go build -p 2 -o '<临时目录>/sub2api-memory' ./cmd/server
```

服务端构建命令退出码为 0。Wire 已重新生成；`go.sum` 增加 Wire 工具依赖 `google/subcommands` 的校验和。34 个本轮 Go 文件的 `gofmt` 检查及 `git diff --check` 通过；其他进程的文件不重排。验证后已停止并删除本任务独占的 PostgreSQL 临时集群，诊断日志留在忽略的 `docs-local/respire-verification/`。

独立验证者在核对最终源码、文档和上述结果后，批准当前按配置启用的 Responses HTTP 文本 MVP，未发现范围内遗留的明确 P1/P2 问题。此批准仅覆盖本地实现与列明的测试，不扩展到以下待验收项目。

## 实际部署后的持续事项

1. **扩大用户范围前的验收。** 当前只验收了 user 1 的独立服务端 Runtime/库、普通客户端、跨 Key/模型召回、跨用户拒绝和重启持久化。新增用户必须独立准备 Runtime、库、端口、凭据和资源预算，不能共享个人库。
2. **部署权限和许可。** 稳定 CLI v1.0.20 固定启动库，缺少原子多租户 admission；必须保持库不切换并建立宿主隔离。Respire Noncommercial License 1.0 与 Core 条款的商业使用许可覆盖范围仍需由部署方落实。
3. **完整产品能力。** 当前来源日记可用于后续检索，但不是自动提炼后的长期事实库；没有产品级物理删除、本地同步、自动供应全站用户 Runtime 或全协议接入。
4. **真实收益。** 尚未完成在线成本实验。`ForceCacheBilling` 会改写用量分类，下一阶段须先保留原始 upstream usage，再合计注入、归档、存储及 Runtime 开销评估收益。

上述限制不影响本次开发与本地验证结论，也不能被 unit、race 或数据库 fixture 的通过替代为上线完成。

## 追加独立复验：共享数据库的 worker 范围

用户要求另外启动执行者和独立验证者后，本轮重新读取当前源码并实际运行测试，没有沿用先前批准作为结论。独立审查发现并修复两个问题：

- **P2，worker 消费未绑定用户事件。** 原 Claim 全局领取共享数据库中的任务，本实例 provider 只认识自身绑定用户，可能将其他实例负责的事件标成失败并清除原文。现在 Claim 明确接收用户范围，领取、失效租约恢复和 TTL 维护都仅处理本实例 bindings 的用户；空范围不操作，非法范围拒绝。worker 独立复制并去重配置，调用者事后修改切片不能扩大范围。
- **P3，低归档超时配置无法启动。** 合法的 100 ms 归档超时原先可能配出 250 ms RPC 等待，导致 provider 拒绝构造。现在等待上限取检索超时、归档超时和 250 ms 的最小值。

完整六包 unit 在修复前全部通过（总约 240 秒）。修复后冻结源码重新执行五包定向 `-race`，61 个顶层用例通过；真实 PostgreSQL 16 的 component `-race` 从 15 项增加到 18 项，全部通过（包内执行 2.427 秒）。新增用例验证两组不同用户绑定的 worker 共享队列、空/非法 scope 不领取、外部修改配置切片不扩权。完整包的旧结果与修复后的定向结果分别报告，不能混为同一次全量运行。

修复后服务端二进制构建成功，36 个本轮 Go 文件格式和差异检查通过，执行者及独立验证者核对冻结源码哈希未变。

实际启动构建产物，使用独占临时 PostgreSQL 16、Redis 及 loopback 上游/Respire 模拟服务，17 项 HTTP 冒烟检查全部通过（2.785 秒）：

- 启动、健康响应、完整 301 项迁移记录（含 migration 250）及重启后的服务就绪。
- 已绑定用户的 active 状态、未绑定用户的 configured=false、无鉴权请求 401。
- 普通 Responses HTTP 完成，返回事件 ID；真实后台 worker 写来源日记并核对读回，事件进入 completed，队列原文擦除。
- 另一用户读取事件为 404；同用户第二把 Key、另一模型的出站请求实际包含前一轮来源及 fragment 定位。
- 永久停用返回 physical_deletion_confirmed=false；此后普通模型转发继续，记忆 provider 不再被调用。

两次服务端 SIGTERM 退出码均为 0。这补充了真实应用组装、worker 启停和请求链路证据；运行模式为 simple，standard 计费完整流程不在这轮二进制烟测内。Respire 和模型仍是本地合成模拟服务，不能代替真实 Core/LLM 验收。共享数据库的不同 worker 范围由前述 component 测试验证，单进程冒烟测试不代替多机部署隔离验证。

执行者已核验并停止本轮应用、Redis、PostgreSQL，删除含临时配置及合成数据的私有目录，清理回执退出码为 0。独立验证者直接核对执行日志、冒烟脚本/观察结果及冻结源码哈希后再次批准当前限定范围，无遗留明确 P1/P2。

本轮新日志与独立报告位于忽略的 `docs-local/respire-verification/rerun-20261106/`。当前维护只覆盖仍有 worker 负责的用户；移除 binding 或关闭 worker 后，不会自动继续执行该用户的 7 天原文清理，需部署维护或通过停用清理。
