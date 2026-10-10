# Plan：Orca 单终端短提示词传输

依据[Spec](orca-terminal-transport.spec.md)直接实施，不进入 roadmap。已 rebase 主干 `872dba44`，保留短提示词、候选快照与结构化发布 URL 流程。

1. 删除 supervised 实现/注册/专属夹具，删除 HTTP 回调、文件回传和辅助终端原型。
2. 共用 worker 以能力声明添加有界紧凑结果约定；Codex 保持原生结果接口。Orca 下层不改业务提示词，不解析阶段含义。
3. 一个激活的原终端；PrepareSession 只读保存每轮来源、游标与输入摘要，StartTurn 原样发送一次。两处均检查公共 screen draft；CLI 固定环境路由，accepted / started 分开；未知效果只读对账，不观察尚未尝试投递的输入。
4. 按固定来源读取、限定身份；stream 要求连续游标，screen 要求完整紧凑帧可见。解析 base64 帧并等待原终端 idle。游标/正文/冲突状态通过既有资源封套持久化；保留 worker_declared 来源，缺口不伪造完成。启动菜单误报 idle 等原生缺口据实登记。
5. 更新配置、单终端 viewer、测试入口/夹具与当前文档。本机/远端仅做相同 revision 核验，Git/PR/交付继续上层。

## 测试计划

用户最新要求提 PR 并自行验证本机/远端真实流程，取代先前“不用验证”的安排。执行下列最小入口，并将结果登记[专属验收](../acceptance/orca-terminal-transport.md)。不跑 full；真实 Git/PR 仅使用独立包与 SteerlineTest 本次专属分支，主机准备问题不转记为产品通过。

| 级别/场景 | 自动项 | 环境、命令、时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| 单元 compact-frame | OTT-A5 | 临时内存；`go test ./internal/execution/workerreturn -count=1`，修改后 | 回显/错 nonce/不完整/重复字段/冲突/超限均不产生完成；软换行不改变业务正文；日志登记 acceptance |
| 单元原语与生命周期 | OTT-A6/A7 | 假 CLI、隔离状态；`go test ./internal/execution/adapters/protocol/orca ./internal/execution/adapters/orcatransport -count=1`，修改后 | 单一资源、原游标恢复、身份/来源/缺口拒绝、原终端停止；非空 draft 拒绝且零 send；日志登记 acceptance |
| 配置/共用正文 | OTT-A8 | 既有临时夹具；`go test ./internal/app ./internal/console -run 'Terminal|Orca|CurrentExample' -count=1` 和 `go test ./internal/execution -run 'Declared' -count=1`，接线修改后 | 旧路径和回调字段拒绝，新原生 agent 和声明结果可接线；日志登记 acceptance |
| 受控完整阶段 | OTT-A7/A8 | 既有隔离夹具；`go test ./tests/integration -run 'WorkerStageOrca(Terminal|Environment)RoundTrip' -count=1`，最终实现后 | 两目标相同三阶段/同终端/原命令、结构化发布 URL 与重启不重发；日志登记 acceptance |
| debug 包隔离 | OTT-A9 | `go test ./tests/e2e/internal/driver -run '^TestDebugReleaseRuntime$' -count=1`，包支持修改后 | 生成、冻结方案、限定测试仓库/分支、重置范围；不将 simulate 当成真实 Orca 通过 |
| 真机单终端 | OTT-A9 | `scripts/new-debug-release.ps1 -Orca -OrcaExecutable <固定CLI>`；远端增加 `-OrcaEnvironment <固定environment>`；prepare → 推送专属 base → 原生 repo add 或 project setup-clone → debug run → resume_dispatch/start_task → snapshot → 专属 PR 合并 → snapshot，代码与原生主机准备后 | 三阶段同一 runtime/terminal/incarnation、一次投递；中心核实候选和 PR 后交付并释放后继。全部命令、失败及清理回执保存包 evidence，结果登记 acceptance |

人工项：可在后续窗口查看原 Orca 启动设置、会话 viewer 和待核实状态，不阻塞无关工作；不代替任何自动失败或能力缺口。
