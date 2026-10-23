# SteerlineFlow 目录迁移验证

日期：2026-10-23。普通开发，用户直接要求先 rebase，再按[项目规范](../../../SteerlineProjectGuideDoc/project-standards.md)迁移流程文档、引用和程序读取路径。工作分支 `codex/steerline-flow-layout`，rebase 基线为 `origin/main` 的 `75f8981b`，保留此前新增项目规范及改写介绍入口的修改。

## 迁移范围

| 原位置 | 当前维护位置 |
| --- | --- |
| `docs/spec/` 中的 Spec、共同约束和索引 | `SteerlineFlow/spec/` |
| `docs/plans/` 及混放在 `docs/spec/` 的实现 Plan | `SteerlineFlow/plans/` |
| `docs/roadmap/` | `SteerlineFlow/Roadmap/`，保留原任务身份、范围和状态 |
| `docs/testing.md`、`docs/testing/` | `SteerlineFlow/testing.md`、`SteerlineFlow/testing/` |
| `docs/acceptance/` | `SteerlineFlow/testing/acceptance/`，包含验收计划、历史报告和精选证据 |
| `docs/validation/` | `SteerlineFlow/testing/validation/`，保留原验收跳转说明 |
| `docs/archive/` | `SteerlineFlow/archive/` |

产品设计、源码架构、接入说明及 `runtime-install-debug.md` 继续维护于 `docs/`。项目流程入口为 [SteerlineFlow/README](../../README.md)。当前导航、AGENTS、项目内 roadmap-runner 引用同步调整；固定 Git 快照与外部仓库的历史路径保留原含义。

[测试材料路径](../../../internal/testmanagement/model/documents.go)统一为 `SteerlineFlow/testing.md` 与 `SteerlineFlow/testing/test-list.json`，供自动生成、项目维护、候选材料核验及报告生产者共同使用。调试包生成器、双项目初始化、测试脚本、前端夹具和 E2E 契约同步分开读取 `spec/` 与 `plans/`。仅有旧测试材料的项目会报告缺少新材料，不新增旧目录回退。

## 已完成验证

- 文档链接及 roadmap 静态身份检查通过：97 个身份文件、3 个草稿条目、3 个正式条目；只验证路径和身份，不宣称核验了外部实际 DAG。
- L1 通过，L2 受控核心交付冒烟通过；L2 在新目录中创建任务契约并完成交付核验及清理。
- 最终路径相关 Go 定向回归通过：8 个包、272 项测试记录、0 失败；覆盖测试生成与项目材料、清单一致性、CLI、任务模型、产品 DTO 和 SQLite 测试夹具。
- 前端相关 3 个测试文件、19 项用例通过。
- 调试模板中的 marker 程序实际通过与失败两种执行、受管报告解析测试通过。
- 使用 `C:/Project/SteerRunner/bin/steerrunner.exe` 生成全新双项目调试包，manifest 为 `ready`；两个项目均具备新位置的 Spec、Plan、测试规范与清单，任务契约初始化成功。
- 对比迁移 diff，409 份历史机器证据均为内容完全相同的重命名；活动 spec 目录不再混放 `.plan.md`。

执行入口与原始证据保留于本次工作区忽略目录 `.evidence/flow-layout/`：`l1/`、`l2/`、`updated-path-tests/`、`producer-fixture/`、`web-tests.log`、`debug-package.json`；调试包为 `C:/SteerlineDebug/dev-20261023-110116-75f8981b-dirty/`。包内 manifest 和证据记录实际构建及脏工作树状态，不作固定提交的当前通过证明。

最终 Go 定向回归命令（再次运行须使用新输出目录）：

```powershell
go run ./cmd/test-feedback -out <新的证据目录> -- ./internal/testmanagement/... ./internal/projectmaintenance/... ./cmd/test-feedback ./internal/task/model ./internal/product/query ./internal/product/api ./cmd/steerline ./internal/storage/sqlite -count=1
```

## 广泛回归中的失败与边界

额外运行受影响包的 owner 回归，原失败保留于 `directed/`；使用 Go overlay 将修改过的 Go 源文件恢复为 `75f8981b` 的原始内容，结果保存于 `baseline/`，复现以下主干已有失败：

- `TestDeploymentInterruptedRecovery`：原夹具创建 `plan.md`，共享函数读取 `task.plan.md`，在缺文件处失败。
- `TestHTTPConsumerBoundary`：既有依赖闭包触达 `internal/runnercontract`，与当前白名单不符。
- `TestInstallLaunchersWritesCRLF`：实际启动器包含 `restart.bat`，原断言未包含该文件。

本次将部署恢复夹具统一为新目录的完整任务材料后，`fixture-directed/` 进一步执行到原先未到达的恢复子场景，仍有接管证据身份不一致和 Windows ACL 失败。这些结果不登记为通过，不据此修改实际部署恢复或 HTTP 业务行为。

本次迁移的定向验证与上述广泛回归分开判断，原失败和补测分别保留；不将本次结果表述为全量回归通过。

## 提交前再次自查

按用户补充要求，`runtime-install-debug.md` 保留于 `docs/`，不归入 SteerlineFlow；根导航、测试入口及其他引用已修正。

第二轮核对每份迁移源文件与目标文件、活动程序读取路径、动态任务材料路径和全仓文档引用，修复两处遗漏：roadmap 编号工具的 Python 测试夹具改用 `SteerlineFlow/Roadmap` 及分开的 `spec/`、`plans/`；真实执行夹具提示词明确写出 `SteerlineFlow/plans/task.plan.md`。另修正原型说明和验收跳转页中旧目录的链接显示文字。

自查后的结果：

- `python scripts/test_roadmap_task_id.py`：10 项测试通过。
- `python scripts/check_doc_links.py`：564 份 Markdown、3565 个本地链接，0 错误。
- `python scripts/roadmap_task_id.py check`：97 个身份文件、3 个草稿条目、3 个正式条目通过。
- 再次执行 L1、L2 均通过；原始证据为 `.evidence/flow-layout/self-review-l1/`、`self-review-l2/`。
- 迁移清单无缺失目标、无旧目录副本，409 份历史机器证据内容不变；活动 `spec/` 中无混放 Plan。
- 源码旧路径扫描只剩 `internal/projectmaintenance/rules_test.go` 中刻意保留的旧目录拒绝反例；其余保留的文档旧路径属于固定 Git 历史读取记录或 SteerRunner 外部仓库。

读取逻辑从测试材料常量追踪至生成服务、项目维护、候选材料核验和报告生产者；同时核对调试包生成、双项目绑定及 E2E 的 Spec/Plan 分目录路径。审计摘要为 `.evidence/flow-layout/self-review-path-audit.json`，`git diff --check` 通过。

## 同步最终主干及复验

提交前远端新增 `81820e2a`，本分支再次 rebase 到该提交。保留其部署生成记录接管修复及测试证据，将新增的 `install-deployment-generation-20261022.md` 同步移至本目录，合并两个验收导航条目，并修正测试指南新增的验收链接。`runtime-install-debug.md` 继续保留在 `docs/`。

在该主干上重新运行 L1、L2 均通过；部署恢复定向命令 `go run ./cmd/test-feedback -out <新目录> -- ./internal/app -run '^(TestDeploymentInterruptedRecovery|TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite)$' -count=1 -timeout=240s` 通过，165.042s wall，证据分别在 `.evidence/flow-layout/rebased-l1/`、`rebased-l2/`、`rebased-deployment/`。因此上文原基线的部署恢复失败已被新主干修复及本次复验取代；HTTP 依赖边界和启动器断言的已有失败记录仍保留，不宣称全量回归通过。

最终链接检查为 565 份 Markdown、3567 个本地链接、0 错误；10 项 Python 测试和 roadmap 身份检查再次通过。以最终主干为对比的迁移审计无缺失文件或旧目录副本，409 份历史机器证据仍为内容不变的重命名。
