# Plan：Orca 单终端短提示词传输

依据[Spec](orca-terminal-transport.spec.md)直接实施，不进入 roadmap。最新主干 `03df9d14` 保留短提示词、候选快照与结构化发布 URL 流程。

1. 删除 supervised 实现/注册/专属夹具，删除 HTTP 回调、文件回传和辅助终端原型。
2. 共用 worker 以能力声明添加有界紧凑结果约定；Codex 保持原生结果接口。Orca 下层不改业务提示词，不解析阶段含义。
3. 一个原终端；PrepareSession 只读保存每轮游标与输入摘要，StartTurn 原样发送一次。CLI 固定环境路由，accepted / started 分开；未知效果只读对账。
4. 按原 stream 游标读取、限定来源和身份，解析完整紧凑帧并等待原终端 idle。游标/正文/冲突状态通过既有资源封套持久化；保留 worker_declared 来源，缺口不伪造完成。
5. 更新配置、单终端 viewer、测试入口/夹具与当前文档。本机/远端仅做相同 revision 核验，Git/PR/交付继续上层。

## 测试计划

用户于 2026-10-10 明确要求“不用验证，直接实现”；本次不执行下面的自动验证或新探针，也不把已撤回原型的通过记录用于新方案。待后续要求验证时执行最小入口并将结果登记[专属验收](../acceptance/orca-terminal-transport.md)。没有默认 full 或真实发布验收。

| 级别/场景 | 自动项 | 环境、命令、时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| 单元 compact-frame | OTT-A5 | 临时内存；`go test ./internal/execution/workerreturn -count=1`，后续授权验证时 | 回显/错 nonce/不完整/重复字段/冲突/超限均不产生完成；日志登记 acceptance |
| 单元原语与生命周期 | OTT-A6/A7 | 假 CLI、隔离状态；`go test ./internal/execution/adapters/protocol/orca ./internal/execution/adapters/orcatransport -count=1`，后续授权验证时 | 单一资源、实际短输入、原游标恢复、身份变更和裁剪拒绝、原终端停止；日志登记 acceptance |
| 配置/共用正文 | OTT-A8 | 既有临时夹具；`go test ./internal/app ./internal/console -run 'Terminal|Orca|CurrentExample' -count=1` 和 `go test ./internal/execution -run 'Declared' -count=1`，后续授权验证时 | 旧路径和回调字段拒绝，新原生 agent 和声明结果可接线；日志登记 acceptance |
| 受控完整阶段 | OTT-A7/A8 | 既有隔离 SteerlineTest 夹具；`go test ./tests/integration -run 'WorkerStageOrca(Terminal|Environment)RoundTrip' -count=1`，后续授权验证时 | 两目标相同阶段/同终端/原命令、结构化发布 URL 与恢复不重发；日志登记 acceptance |
| 真机单终端 | OTT-A9 | 新 `scripts/new-debug-release.ps1` 独立包与专属 SteerlineTest；命令/场景待后续验证安排时固化 | 本机/远端短投递、完整帧、连续回合和清理均实测；证据位于包 evidence。当前不运行、不宣称稳定通过 |

人工项：可在后续窗口查看原 Orca 启动设置、会话 viewer 和待核实状态，不阻塞无关工作；不代替任何自动失败或能力缺口。
