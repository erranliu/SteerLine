# PR 4A Plan：长期双执行后端契约

依据：[Spec](pr-04a-execution-backends.spec.md)。依赖：PR 3 完整验收并合并。

## 实施顺序

1. **确认集成基线。** 从最新 origin/main 建本阶段 codex 分支；读取总览、Spec 02/03 和 PR 3 实现。记录权威存储、配置、runtime/控制协议版本及现有生命周期测试。若中心模型已落地，直接扩展其接口。列出 Service、Spawner、orcaexec、decision、retry、recovery 中现存宿主调用，作为迁移清单。
2. **定义契约与证据。** 增加不可变 AttemptSpec、ExecutionRef、Operation、Observation、类型化回执/错误和能力表；分开 backend/protocol/agent/target。记录状态转换、持久化顺序及各平台对照，先写双后端参数化契约测试。4B 的 ReadOutput 先保留原始来源与有界批次，不能猜 phase。
3. **持久化绑定与动作。** 实现版本化记录、历史映射与恢复索引，测试意图前后和平台响应前后的失败。SQLite 尚未合入时沿用持锁单写者；若升级 runtime，先完成新旧版本读者与迁移再接入副作用。没有平台请求查询能力时，明确保留 outcome_unknown。
4. **接入 local。** 包装现有工作区准备、Codex 启动、OS 身份、输出与停止；将进程句柄所有权和观察取消分开，保持 PR 3 Job/进程树与显式停服规则。Service 不再依赖本地进程细节；保留原结果核验与 retry 资格。
5. **接入 Orca。** 包装现有 worktree/terminal 创建与身份、监督、同终端 continuation、输出与停止；移除吞掉 close/stop 错误的路径。原 terminal v2 与将来 supervised 分协议实现；当前配置不能自动升级到新协议。
6. **收拢业务入口。** Start/Observe/Stop、决策发送、retry、手工恢复的宿主事实均走公共契约；只保留一套准入、领取、槽和业务状态转换。按能力选择支持范围，PR 5 再完善答案应用证据/交互。确认 UI/CLI 不引用具体后端类型。
7. **更新实现文档。** 随实际行为更新 hosting、state-model、data-contract、配置示例与开发说明；提供能力矩阵、生命周期调用图、旧记录升级/拒绝案例。证据放 docs/validation/ 下并由本阶段验证报告链接，平台版本和未执行项必须列出。

## 验证矩阵

| 层次 | 检查与证据 |
| --- | --- |
| 公共契约 | B-A1～B-A9 对两个适配器运行同一场景；断言平台调用次数/精确 ref/状态与资源保留，不仅断言返回文本 |
| 故障与恢复 | start/stop/reply 响应丢失、磁盘写失败、重启、重复 operation、同 ID 异负载、身份替换、慢观察/取消；未知结果不得重发 |
| 兼容 | runtime 旧版本、历史 attempt、原配置、watch、answer、local retry、accept-result、PR completion 和 PR 3 生命周期相关测试 |
| 原生 local | 隔离测试仓库中真实 Codex 启动/输出/停止；Windows 进程树与 PR 3 关闭客户端存活，Linux 原生退出/回收/分离观察；仅交叉编译不能代替原生证据 |
| 原生 Orca | 隔离测试 worktree/terminal 中启动/观察/停止/重连、同会话定位；记录版本、ref、残留资源和能力证据。不得操作生产任务 |

运行 `go test ./... -count=1 -timeout 10m`、`go vet ./...`、支持平台构建及 `git diff --check`。针对并发观察/取消/事实应用运行 race 检查（在支持工具链的平台），不可执行时记录环境阻塞。真实平台缺失是相关必需验收未完成，不能把 fake 或 skipped 当成替代。

## 完成与交接

- B-A1～B-A10 有逐项证据；两正式后端的核心能力和既有行为回归通过，所有未支持的可选能力有理由。
- 最终完整 diff 中业务层直接调用平台 CLI/OS 的旧执行路径已移除或明确归入适配器；仅配置映射可以识别旧 host 名称。
- 提交、推送、创建面向 main 的 PR；实现任务不自行合并。审核修复与验证通过并确认合并后才启动 4B。
- 回退需先停服并核实执行身份；已产生新外部动作的状态不能用旧快照覆盖。保留原始文件与产物，提供版本不兼容时的明确拒绝。
