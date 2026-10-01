# 测试设计审计与基线修复结果

日期：2026-10-01。依据：[修复 Plan](../../../spec/test-baseline-repair.plan.md)。[原始失败基线](../baseline-2026-10-01/README.md)保留历史事实。

最终候选 `1343772` 的完整三进程回归通过：343 个核心顶层入口覆盖完整，各分组和其他包均零失败；准备 2.57 秒、执行 756.27 秒。18 个测试/子测试按原有原生或显式验收条件跳过，无新增 skip。见[原始完整汇总](full-summary.json)和[原失败到当前通过用例的逐项映射](results.json)。原始 JSON/stderr 保留在汇总中的输出根。

## 设计审计与处理

三级选择、显式 tag、真实 Git/SQLite、受控外部组件和资源所有权边界合理。失败主要源于夹具和断言没有跟随当前执行、发布、Decision 和投影契约。本轮未修改生产代码，未删除测试、扩大时限或恢复旧格式兼容；保留真实进程、Git、SQLite 与业务/故障断言。

| 原失败簇 | 设计问题与修复 | 保留的行为验证 |
| --- | --- | --- |
| service/lifecycle/project-view/idle/corruption | 通用本机夹具用了不支持的 host_service + Gitea。改为支持的 GitHub command 组合，只映射 Git 网络操作到隔离 bare remote，remote 身份查询仍真实 | 进程/启动身份、锁、陈旧心跳、回执丢失、配置变化及无操作轮询 |
| config/global-policy/projects/CLI/console | 必需字段缺失，负例被无关前置校验挡住；配置选择比较未绑定 Profile digest | 配置所有权、危险参数拒绝、版本摘要失效、固定引用、请求 ID 先持久化、幂等查询 |
| Git sandbox/local Windows profile | GitMetadataWrite 或发布快照不完整 | 精确 writable roots、主仓库/外仓库拒绝、冻结权限保持 |
| Decision/start replay | format version 和路由 project identity 不完整，纯投影拒绝输入 | 内容校验、归一化、start replay 不接受未发送的当前答案 |
| app-server/stop/dead recovery | 人工 process/session 没有原权限快照。纯规则/回执单测复用当前类型化 receipt fixture，不扩大为原生权限验收 | start/操作同事务、停止意图先于外部动作、未知回执不重发、原问题/新答案保持、迟到代次拒绝 |
| capability fixed agent | 修改默认 agent 后才构建 backend；能力证据索引缺 agent version | 固定实现/target/agent 的精确证据，新默认不复用旧证据 |
| environment recheck | 提交刷新后继续断言旧 Execution 指针；补夹具前提诊断并重取权威记录 | 不重建活跃 Dispatch、原身份保持、替换 incarnation 拒绝 |
| discard/delivery | 人工 WorkflowRun 缺原 v4 快照；注册项目后才换 Gitea provider | 持久化丢弃意图、未知/已合并拒绝、固定发布身份、清理与工作区事实、幂等、无副作用 |
| retired SQLite | 依赖旧文案空格形式，实际拒绝仍报失败 | `unsupported_schema` 错误码，数据库原字节保留 |

## 按当前契约调整的预期

- `TestOrcaPreflightDoesNotRequireUnusedLocalCodex` 改为 `TestOrcaPreflightRequiresConfiguredCodexPolicyMapping`：当前 Windows Orca/Codex 权限映射需要固定原生 Codex，即使没有 model override。新例使用合法 worker/provider 并明确拒绝缺失 executable。
- Orca 的 opencode 分支没有已验证权限映射，明确断言不可选择；Codex 分支继续验证当前默认 agent 改变不能重定向冻结 Profile。不补未知平台支持。
- 未注册 Orca 组合的负例使用明确未注册 agent 和禁止调用的 executable，验证注册门槛在外部探针前拒绝。原残缺 publisher 配置无法检验该目标；合法配置与各项静态权限拒绝仍由专属测试保护。
- 反馈阶段先验证原冻结目标保持，再验证原子 phase/CAS 和迟到 actor。后来修改默认 Host 本身不能使原执行丢失合法冻结配置，不保留已经过时的默认配置漂移拒绝预期。
- 交付分支原来传入不存在的 WorkflowRun 却要求分支漂移错误。现明确核对 `inconsistent_delivery`、verifier 零调用和无写入；真实冻结发布目标/漂移由既有 publication 测试继续保护。

这些调整均有明确当前消费者或既有契约依据，不把失败条件直接改成成功。完整旧失败名单的 49 个测试/子测试终态均对应到本轮通过用例；唯一重命名已在 results.json 标明。

## 验证范围

先按失败簇定向：配置/生命周期、Git/Decision/停止恢复、丢弃/交付通过；`cmd/steerline`、integration、console 包通过。核心原失败集的集中定向运行仅剩未注册组合负例，修正后该例和固定 agent 证据选择单独通过。

L1 quick 通过，准备约 0.9 秒、执行 23.0 秒，原始报告在主机临时目录 `steerline-test-feedback/20261001T123227.715640800Z/summary.json`。固定候选后仅再跑一次 `go run ./cmd/test-feedback -workers=3`，完整清单和终态核对通过。

修改仅为测试文件和文档，生产接线、E2E 环境和场景代码未变；沿用 `67427ec` 上已通过的 L2/L3 与两项目检查，不重复这些场景。没有执行真实平台、远端、UI 或多平台矩阵。完整通过是默认确定性回归，不是原生平台认证。
