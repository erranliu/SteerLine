# 旧 Runner Check 接入退役 Plan

2026-10-29，已实施并验收。[Spec](../spec/legacy-environment-check-retirement.spec.md) · [验收](../testing/acceptance/legacy-environment-check-retirement.md)。普通聊天实施；不调度 Roadmap、不部署。本轮按审计后方案实施。

## 1. 基线与执行顺序

源码 HEAD `7617cddc7efac5716c8c38bb63a0157a100acac0`，叠加当前未提交 readiness/普通 Run result 流程为受保护基线。实施开始记录 dirty 文件和内容摘要；不得 reset、覆盖既有变更或用旧 HEAD 运行结果替代本轮验证。

按 P1→P2→P3→P4→P5 顺序收口，不用“页面隐藏了”或“旧命令不调用了”代替完整删除。Runner 新契约固定只约束 P2 的公开 wire 契约实现；独立业务/存储/入口删除可先执行；真实独立 EXE 是 LCR-A07 前置，不要求为写 Spec/Plan 启动模型或生成新包。

## 2. 实施步骤

### P1：固定删除清单与双方契约

1. 从 Spec 第3节入口沿调用关系区分旧 Check、当前项目环境活动、普通 Run 和通用 checkpoint/检查函数，记录待删文件/字段/调用及保留消费者。重点覆盖 `runnerbinding`、service/stop/takeover、state preflight 和 reset，不能只搜 CLI 文本。
2. 固定 Runner 移除后的 capabilities/schema、service binding、takeover 与 verify-stopped 实际公开样本及版本。本仓只消费确认样本，不手写猜测空字段维持旧解析器。若交接未确认，记录具体契约缺口，不新增兼容接线。
3. 仅对本次明确拥有的旧隔离现场使用原版本核实停止；没有旧现场时记录不适用，不为覆盖此项新造旧 Check。安装环境及他人现场不查、不停、不迁移。

### P2：删除旧业务、存储与 caller，收拢共同生命周期

1. 删除 `internal/environmentcheck`、`runnerstate/environment_check.go`、旧 `app/environment_checks.go` 与仅为旧功能设置的 port/service/actor 调用。保留 `internal/environmentadaptation` 及其 Store、current 引用和普通 Run 原请求。
2. 从 `project_runtime.go`、`client.go`、`environment_admission.go`、`state_preflight.go`、`runner_build_binding.go` 去除旧 Check 构造、绑定和读取。准备占用继续检查全部新活动与原候选，不删互斥、原重放或最终资格事务复核。
3. 删除旧 Runner command 分发、Check DTO/capability/schema 和仅为其保留的 fixture。联动 `runner_takeover.go`、`application_verify_stop.go`、`runnerbinding/takeover.go`、`runnerclient/verify_stopped.go` 与 service binding：去掉旧 Check 参数、枚举、证明及 pending stop；保留 Run/生成/新环境活动清单、原摘要/实例/路径校验和未知义务。方法签名直接收口，不传空 slices 保留无消费者的参数。
4. 删除 SQLite 两张旧表、reader/store/ports、计数与请求索引；在当前 schema34 基线上改为35，沿原 unsupported_schema 拒绝旧库，不实现迁移。测试和独立调试使用新库，不触碰安装库或导入旧记录。
5. 更新 `tests/e2e/internal/driver/debug_reset.go`、原清理器与共享 fixture：删除旧历史表名和 Check 安全函数，保留 reset 之前的服务/Run 完整结算及 namespace/path 身份保护，不能以删除旧逻辑为由减弱原 Run 安全断言。

### P3：删除产品入口，保留新准备和派发体验

1. 删除 product API/client/HTTP 中旧 options/list/detail/request/start/stop 接口、路由白名单、命令 registry/intents/authority 和旧投影字段。未知旧入口沿既有错误行为，不新增 redirect、空成功或旧 payload 接受分支。
2. 删除旧 `environment-checks.tsx`、专用 types/测试、Query 与 query invalidation、app lazy route、标题和入口、新准备页底部的旧历史链接。
3. 同目录保留 `project-environment.tsx`、`use-readiness.tsx` 和其组件测试；保留 `/environment`、项目环境计数、材料/原报告/停止以及任务/派发 readiness blocker。不因目录名而大规模搬迁，不把新报告中的 checks 当旧 Check 删除。

### P4：测试、fixture 与真实契约交接

1. 删除旧能力专属测试和资产；共用测试替换为退役后的公开 fixture，并覆盖下方 LCR-A01–A07。新定向用例统一前缀 `TestLegacyCheckRetirement`，实施时按真实所属包安排，不能以空匹配通过。
2. 用不含旧 capability/schema/Check 字段的单一样本验证启动、接管、停机和 reset。增加必要反例证明 Run 证据缺失、身份不符、未知停止仍拒绝；不把原拒绝断言全部删掉。
3. 扩展既有 L3，不新增一套退役 E2E：共享生命周期、原候选重启交付、两项目两环境的正式准备/门禁。公开 fixture 中新准备仍经普通 submit/inspect/result/stop，不直接写 ready。
4. 真实 Runner 固定独立 EXE、version、SHA、资源摘要后，实现计划用例 `TestLegacyCheckRetirementPublishedContract`：核对退役后的公开契约，使用新独立 state-dir 启停空闲服务并核实结算，零 Agent/业务输入。缺交接输入时保留 LCR-A07 待验，不偷偷选择源码 bin 或安装版作为回退。

### P5：文档、独立复审与交付

1. 更新 `docs/integration/steerrunner.md`、当前架构/协议/执行配置/数据模型/worker 生命周期中的旧功能要求，Spec/Plan 索引及受影响测试登记。旧环境检查 Spec/Plan 实施后标记已退役；历史 acceptance 和冻结证据保持原版本记录，不机械修改旧方案全文。
2. 同步现有 readiness/adaptation 文档涉及“旧基础检查占用”的现行要求，明确只剩项目文档/脚本/普通 Run/result 业务链；不借此改轮次、L2、PR 或 ready 标准，也不把额外 artifacts 能力列成本次前置。
3. 本轮不计划修改固定模型正文；如实际涉及提示词、接线、资产或对应测试/CI，按治理门禁提供实际正文 diff、字节账本、业务原因与独立审核，不能用黄金样本更新代替批准。
4. 独立只读复审实际快照：确认没有旧功能消费者、共同安全校验未被一起删除、当前未提交新功能完整保留；确认剩余 `Check/checks` 引用的真实语义与消费者。修复确定问题后复查，完整证据记 acceptance，不修改原失败报告。

## 3. 最小验证安排

按[路径清单](../paths.md)使用 `C:/SteerlineDebug/wt-5069-legacy-check-retirement/{diagnostics,test-results,packages}/` 的每次全新绝对叶目录。命令中的 `<新目录>` 执行前替换，不复用旧输出。L3总并发最多二，源码/EXE/Web构建与 live 沿原串行约束。

| 时点 / 验收 | 命令与环境 | 通过条件 |
| --- | --- | --- |
| 本轮文档；LCR-A08静态部分 | `python scripts/check_doc_links.py`；`git diff --check` | 新链接/围栏/表格及范围正确，无源码改动；不记功能通过 |
| P2–P4；A01–A05 | `go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/runnerclient ./internal/runnerbinding ./internal/app ./internal/storage/sqlite ./internal/taskrunner ./internal/product/... ./tests/e2e/internal/driver -skip '^(TestProjectEnvironmentExecutionPublishedResultRoundTrip|TestRunnerFullBindingPublicCLI|TestLegacyCheckRetirementPublishedContract)$' -count=1 -timeout 30m` | 受控 owner 实际枚举通过、零 skip；上述三个外部前置用例从本次选择中排除，分别单列既有原生验收或A07，不删除测试、不冒充通过；共同 Run 拒绝/恢复与 reset 反例保留，新增退役用例非零命中 |
| 代码修改后及提交前；A06 | `go run ./cmd/test-feedback -level=L1 -out <新L1目录>`；`go run ./cmd/test-feedback -level=L2 -out <新L2目录>` | 固定最小集和正式交付冒烟及清理通过，fixture 不要求旧 Check 能力 |
| P3；A04/A06 | `pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/test-web.ps1 -EvidenceDir <新Web目录>` | Web、HTTP/server及生产构建通过；旧入口/请求退出，新准备、任务门禁、轮询及停止有效 |
| P4；A02/A03/A06 | L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario=runtime.service-lifecycle -out <新L3目录>` | 同home多项目/原实例绑定、重启与结算保持，无旧 Check 前置 |
| P4；A02/A03/A06 | L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -scenario=recovery.waiting-merge-restart -out <另一新L3目录>` | 原候选冻结/重启交付及未知零重发保持，原 Run 证明未被放宽 |
| P4；A02/A04/A06 | L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario=testing.project-environment-readiness -out <另一新L3目录>` | 两项目两环境经普通 Run/result 准备、精确放行和失效拒绝，零旧 Check 依赖，停止清理完整 |
| Runner新构建交接后；A07 | 固定 `STEERRUNNER_EXE`、`STEERRUNNER_VERSION`、`STEERRUNNER_SHA256` 后，`go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/runnerclient -run '^TestLegacyCheckRetirementPublishedContract$' -count=1` | 该用例为计划新增，实施后须实际命中；真实新契约及空闲服务启停/结算通过，零模型/业务输入 |
| P5；A01/A08 | 按 Spec 范围审查生产符号/资产/当前文档的剩余引用，静态检查及独立复审 | 剩余命中只有有用途的通用含义或明确历史；无兼容分支/占位字段/重复入口 |

不默认运行 Full、全部 L3/L4、模型或真实 PR。当前三条原生准备已有的未通过项归原验收，不能为本退役任务扩大为修 Runner 执行或目标环境。缺新 Runner 构建、零匹配、skip、失败与资源未知如实登记，不能以受控通过代替真实交接。

## 4. 完成条件

Spec LCR-A01–A08 全部有本轮证据；共同安全反例、固定 L1/L2、Web、三项既有 L3、真实新契约与独立复审通过。仅有实现/fixture而无真实构建交接时，状态为“实现完成、联验待验”，不能写整体完成。未操作安装环境，不新增旧数据读取或迁移路径。
