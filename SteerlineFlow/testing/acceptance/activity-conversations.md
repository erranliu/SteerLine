# 活动中心统一对话阅读验收

2026-11-12，基于 `400e15dc` 和本次工作区改动。六类活动通过同一入口读取原生对话或已记录交互。会话选择、历史分页和正文读取均以原业务记录的绑定为依据，选择活动和读取摘要时不访问原生历史。未部署。

## 真实环境检测

只读核对保留的环境检测记录及其第 1 轮绑定。原记录包含 Session、generation 和已核实的最终回复，适配器为 `codex-app-server`。

使用原安装构建、当前服务实例及上述完整绑定，只读执行 `native-conversation-history --limit 25`。Runner 返回 `unsupported_capability`，原执行适配器未公开原生对话历史。同一 Run 的公开 `history` 也返回 `unsupported_capability`。公开 `events --limit 100` 返回 23 条且无后续页，事件类型为流程状态及结果事件。此次没有通过公开接口取得最终回复之前的聊天正文。面板可以展示已记录交互中的最终回复。

## 自动验证

| 命令 | 结果 | 证据 |
| --- | --- | --- |
| `go run ./cmd/test-feedback -level=L1` | 通过，随后修改的阅读组件由最终定向测试覆盖 | 本地验收证据目录 |
| `go run ./cmd/test-feedback -level=L2` | 通过 | 本地验收证据目录 |
| `go run ./cmd/test-feedback -- ./internal/app ./internal/product/query ./internal/storage/sqlite ./internal/runnerclient -run '^(TestActivityConversation\|TestActivityReading\|TestRefactoring\|TestNativeSession)' -count=1` | 通过，覆盖六类注册来源、原服务与会话绑定、目标隔离、交互分页和 SQLite 原文引用 | 本地验收证据目录 |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.storage` | 通过 | 本地验收证据目录 |
| `npm --prefix web test -- src/features/activities` | 44 项通过，覆盖按需读取、检测轮次选择、读取失败后使用已记录交互，以及主分支检查结果 | Vitest 输出 |
| `npm --prefix web run build` | 通过 | TypeScript 和 Vite 输出 |

SQLite 争用修复后定向复验活动读取、来源绑定、SQLite 只读池和重构正文，通过。原报告位于 本地验收证据目录。

首次 `runtime.async-activities` 在同时打开对话和已记录交互时遇到 SQLite `SQLITE_BUSY`。已记录交互的只读请求在原超时内对短暂锁争用有限重试，持续不可读时返回可重试的 503。首次报告位于 本地验收证据目录。

复跑 `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.async-activities` 时，活动抽屉与独立阅读页两条 Edge 用例通过，包含 1440、768、320 像素视口以及按需分页和正文检查。活动读取证据为 `scenarios/runtime.async-activities/browser/activity-reading-proof.json`。整场 L3 仍失败，发起反馈子用例等待两次候选提交时实际只观察到一次。浏览器记录中没有第二次候选提交请求。复跑报告和上述相对证据均位于 本地验收证据目录。

历史 Attempt 的原生内容取决于 Runner 当前保留的可读取绑定。重构的本地交互索引随原业务记录写入，已有未索引的记录没有进行回填。

## 本地 Codex app-server 接入复验

2026-11-12，在独立 SteerRunner worktree `codex/codex-native-conversation-history`（基线 `97b78b39`）扩展了同一个公开历史命令。活动读取继续核验原 Runner 状态目录和原会话身份，向兼容升级后的当前 Runner 使用完整服务绑定。定向回归验证同目录不同构建可读，其他 Runner 状态目录被拒绝。

使用新 Runner CLI 和隔离的只读服务分派器，从 SteerLine 公共活动读取路由查询上述同一 Run 和 Attempt。返回 2 条助手消息，第一条为 138 字节的原过程消息，第二条与 184 字节已核实最终回复完全相同。复验过程没有启动业务执行或替换安装。日志见 Runner worktree 的 `.evidence/native-conversation-original-run/original-run-activity.log`。

本轮活动中心 44 项前端定向测试、生产构建及固定 L1/L2 均通过。
