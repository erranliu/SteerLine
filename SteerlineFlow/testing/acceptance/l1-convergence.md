# L1 固定最小集收敛与 L4 完整回归

日期：2026-11-20。状态：实现、验证与独立审核通过。用户授权按建议收敛 L1、下沉完整回归、保留关键门禁、同步清单文档，验证后独立审核。普通开发；不启动 roadmap，不改变业务、固定提示词正文、黄金样本或 CI。

## 实现范围

- Go 固定根由 26 个收为 17 个：task/model、task、runnercontract 不再整包执行，而明确选择六个代表根；固定提示词四根、清单三根、原子持久化及真实仓库架构/Runner 门禁保留。
- 前端原 client/state 两文件中的五个用例原样移入两个 minimum 文件，API 准备逻辑抽为共用 test-support。其余原断言留在原文件，L4 web.unit 自动枚举原文件与 minimum；没有复制或删除断言。
- 新增 isolated/L4 go.rules（三个规则 owner 全量）及 go.boundary（边界和普通 integration 两包全量）。均仍归 go.full；不改普通测试标签或 Full 范围，不将真实 Runner 契约专项混作本次 DTO 规则测试。
- 提示词人工反例提取为 TestPromptGovernanceSourceBoundaryFixtures，L4 执行完整矩阵。原源码守卫与 TestThreeLayerRepositoryBoundary 共享同进程一次完整 Check；Runner 边界删除重复源码扫描，独有 Windows/Linux 产品闭包及退役目录断言保持。完整 Check 继续包含开发者/原生配置与提示词检查，生产检查器无改动。
- 执行器明确拒绝缺少固定 owner 根及额外 owner 根（即使报告 Assigned 被改写），并验证固定调用均带精确 -run。原零匹配、跳过、终态完整、退出码及 managed 规则保留。清单、分级列表、行为选择及路径说明同步；E2E manifest 无新增场景，继续校验其与正式清单一致性。

业务原因是阻止普通 owner/文件的新回归自动进入每次必跑 L1；完整故障矩阵由按实际行为影响选择的 L4 保持。真实仓库架构检查曾补足 L1 通过但 Full 失败的缺口，故保留完整矩阵。只收窄默认复用范围，不降低完整回归断言。

## 提示词、接线与实际正文差异

固定模板、materials、黄金样本、prompts 组装及生产接线正文均无差异，CI 文件无差异。本次实际变化位于 L1 选择及边界测试：

```diff
- TestPromptGovernanceSourceBoundary: 人工反例循环 + ScanSource(真实仓库)
+ TestPromptGovernanceSourceBoundaryFixtures: 原人工反例循环（L4 全集）
+ TestPromptGovernanceSourceBoundary: checkRepositoryBoundary(t)，检查原 prompt boundary 违规
- L1 boundary: SourceBoundary / ThreeLayerRepositoryBoundary / NativeConfigurationOwnership
+ L1 boundary: SourceBoundary / ThreeLayerRepositoryBoundary
- RunnerExecutionBoundary: ScanSource + 开发者配置过滤 + 原产品闭包
+ RunnerExecutionBoundary: 原产品闭包（源码统一由完整 Check 检查）
```

固定正文实际 diff 为空；不更新黄金样本。`prompt-byte-audit.json` 保存 54 文件逐项字节/hash，黄金使用原始字节逐字比较，其余同时记录 HEAD 原始、checkout 原始及 LF 规范化值；下表非黄金为 LF 值，不将 checkout CRLF 当正文改动。首次审计把 Go/CI 的行尾差异误作正文差异，已改按上述显式口径核对，没有改任何源文件来使审计通过。

| 未变来源 | 文件数 | 字节前 → 后 |
| --- | --- | --- |
| 固定短模板 | 18 | 9,735 → 9,735 |
| 完整规则 materials | 8 | 16,423 → 16,423 |
| 黄金样本（原始字节） | 24 | 15,104 → 15,104 |
| prompts 组装与测试 | 2 | 15,273 → 15,273 |
| 生产提示词源码守卫 | 1 | 3,980 → 3,980 |
| CI | 1 | 1,246 → 1,246 |

实际测试选择/检查组织差异保存在 `test-wiring.diff`；对应 UTF-8/LF 字节如下。增长用于明确选择和拒绝漏根/额外根，不新增模型行为规则。

| 变更文件 | 字节前 → 后 |
| --- | --- |
| `cmd/test-feedback/local.go` | 10181 → 10510 |
| `cmd/test-feedback/local_test.go` | 6175 → 6299 |
| `cmd/test-feedback/scoped.go` | 28639 → 28940 |
| `cmd/test-feedback/scoped_test.go` | 15079 → 16036 |
| `cmd/test-feedback/task_scope.go` | 382 → 663 |
| `cmd/test-feedback/web.go` | 6203 → 6219 |
| `internal/testkit/boundary/prompts_test.go` | 2545 → 2607 |
| `tests/integration/runner_boundary_test.go` | 2097 → 1955 |

## 验证与证据

受测基线：b9ce126ea0b594b453081ce7f8bb16d33a1139b5；实现后为同 HEAD 加本次工作区差异，不冒充干净提交通过。主要报告根：`C:/Users/lmas1/AppData/Local/Temp/steerline-l1-convergence-6325-20261120-a/`。每次执行使用独立新子目录；原失败与后续结果分开。

环境：Windows/amd64、Go 1.26.4、Node 24.13.0，缓存 as found。初次沙箱 npm offline 缺缓存，普通 npm ci 因目录权限失败；使用正常权限 npm ci 后依赖完整。初次沙箱 L1 的边界组失败且整体到达 300 秒期限，原失败工具输出保留在本聊天；其 sandbox 临时目录在验证后已不可访问，原报告无法重新读取，不能作通过、性能或后续复用证据。正常权限基线 baseline-L1 正式通过，准备 6.0 秒、执行 116.0 秒。存在冷编译及其他 Go 进程，本轮不把这些样本当作独占机器性能对照，也不外推稳定性。

| 验证 | 实际结果 | 本次报告/证据 |
| --- | --- | --- |
| 原 L1 基线 | passed/complete；完整命令 161.167 秒，报告内准备+执行 122.028 秒 | `baseline-L1/`、`baseline-L1-timing.json` |
| 收敛后正式 L1 | passed/complete、66 条执行终态、零跳过；17 个 Go 根 + 5 个 Web 用例。完整命令 42.206 秒，报告内准备+执行 37.456 秒 | `final-L1/`、`final-L1-timing.json` |
| 固定受控 L2 | passed、清理完整；准备 16.098 秒、执行 117.288 秒 | `final-L2/`、`final-L2-timing.json` |
| 完整规则 go.rules | 三包 37 条终态、passed/complete、零跳过；执行 6.869 秒 | `rules-L4/` |
| 完整边界 go.boundary | 两包 235 条终态、passed/complete、零跳过；执行 18.434 秒 | `boundary-L4/` |
| 完整 Web web.unit | 57 文件、523 用例全部通过，零跳过；执行 36.680 秒 | `web-L4-unique-names/` |
| 执行器与提示词完整 owner | 310 条终态、passed/complete、零失败/跳过；wall 185.722 秒 | `executor-prompts/` |
| 前端类型检查 | `node web/node_modules/typescript/bin/tsc --project web/tsconfig.json --noEmit` 退出 0 | `typescript.log`、`typescript-result.json` |
| 原覆盖迁移 | 原 L1 的 105 条 Go 终态在新 L1/两个 L4 中无遗漏；提示词人工子例根显式映射。原两个 Web 文件的 57 个用例名称及出现次数与拆分后四文件完全一致 | `go-coverage-transfer.json`、`web-coverage-transfer.json` |

首次 `web-L4/` 的 523 项原生断言全部通过，但重构预算两条参数化用例同名，正式报告正确判 incomplete；仅将原标题追加 `(case %#)`，原参数、输入与断言无差异。以新目录完整复验通过，不放宽重复身份校验、不覆盖原报告。该标题变化不影响已通过的 L1、Go owner 或前端类型，按影响复用这些证据。

以上耗时仅为本次样本，不据冷编译/缓存不同的前后值宣称提速比例或稳定性；本次正式 L1 完整命令满足一分钟目标。没有执行无关 go.full、额外 L3、真实模型/Runner/远端或安装测试；生产代码和 Full 枚举实现未变，受影响 owner 与原覆盖迁移已定向验证。最终静态检查通过：821 个 Markdown、5,013 个本地链接零错误，git diff --check 退出 0，原日志为 doc-links.log/diff-check.log。五个正式测试集的 managed 终态逐项均为 passed（verification-summary.json）。

## 独立审核

2026-11-20，全部验证完成后，独立只读 reviewer（independent_review）审核通过，无确认的阻塞或可操作缺陷。审核未修改文件或重复运行测试；独立核对当前 diff、未跟踪新增文件及原始报告，并重新核对 54 个提示词相关文件的字节记录。

确认 L1 的 17 个 Go 根与 5 个 Web 用例及关键门禁保留，完整 Check 的开发者/原生配置、提示词与非活动源码范围未减；原 Go/Web 覆盖迁移无遗漏，普通 Full 枚举与标签未收窄；L4 正式选择、完整性、managed 与清单一致。共享 Web 准备逻辑在现有 Vitest 文件隔离配置下成立；523 个 Web 用例、L2 场景及清理与原始终态相符。同名修复仅改变标题，不改变输入和断言。

独立审核明确批准本次测试选择、提示词源码门禁的完整 Check 复用及人工反例下沉。该结论不扩展为 go.full、真实模型、远端或安装链通过。没有剩余必需验证或确认的问题；按用户后续授权以提交和 PR 交付本次代码及验收记录，不包含部署。
