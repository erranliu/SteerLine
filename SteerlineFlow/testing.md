# 测试指南

本文按 [SteerLine 测试架构规范](../SteerlineProjectGuideDoc/testing-architecture.md)组织，是本项目日常测试入口。各级说明见 [L1](testing/l1.md)、[L2](testing/l2.md)、[L3](testing/l3.md)、[L4](testing/l4.md)；系统读取 [test-list.json](testing/test-list.json)。

## 日常执行

每次代码修改后、提交 PR 前运行下面两个固定测试集；根据影响补充 L3 功能测试及 L4 专项。纯文档变更检查链接、清单一致性和 `git diff --check`。L1 目标整体 1 分钟内，L2 目标整体 5 分钟内，实际总耗时包含准备、执行和清理；冷编译或机器争用超出目标时如实记录。

```powershell
go run ./cmd/test-feedback -level=L1
go run ./cmd/test-feedback -level=L2
```

| 级别 | 本项目内容 | 环境与选择 |
| --- | --- | --- |
| [L1](testing/l1.md) | 一个最小单元/集成集合 | 隔离 Go 测试，无模型；不再累积各功能的完整 owner 回归。 |
| [L2](testing/l2.md) | 一个受控核心交付冒烟 | 真实 SteerLine/Git/SQLite，公开 Runner/PR fixture；不需要真实 Runner、模型或 GitHub。 |
| [L3](testing/l3.md) | 可并行功能流程 | controlled，资源隔离，总并发最多二；按受影响 ID 选择。 |
| [L4](testing/l4.md) | 边界、不可并行功能、真实链和 Go 测试集 | controlled/live/isolated；live 必须单独精确选择，按需运行。 |

```powershell
# L3 定向功能；省略 scenario 时运行所选 suite 的该级全部场景
go run ./cmd/test-feedback -level=L3 -suite=recovery,runtime -env=controlled -scenario=recovery.waiting-merge-restart,runtime.service-lifecycle
# L4 系统边界；包含项目维护的 runtime 全集会串行运行
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -scenario=runtime.path-isolation
# L4 可复用 Go 全集（既有 Full），不默认执行
go run ./cmd/test-feedback -level=L4 -suite=go
# owner 定向诊断不代表登记测试集整体通过
go run ./cmd/test-feedback -- ./internal/runnerclient ./internal/runnerbridge -count=1
```

Full 仅指全部普通 Go 测试，已登记为 L4 `go.full`，不包含 e2e 标签流程或 Web 前端测试。前端与 HTTP 检查用 `scripts/test-web.ps1`，其他浏览器及独立脚本入口见 [L4 列表](testing/l4.md)；未接入受管报告的入口不能凭 Agent 自述登记通过。

## 环境与证据

从仓库根目录运行，基础需要 Go、Git。浏览器场景需要 Windows、Node/npm 和 Edge；真实链还需固定独立 Runner EXE、本机 Codex 登录及场景声明的外部权限，详见 [E2E 执行说明](../tests/e2e/README.md)。临时/AI/真实 PR 调试先遵循 [独立调试流程](../docs/runtime-install-debug.md)，不得使用或修改安装目录及保留现场。

每次使用新的 `-out <绝对目录>`，默认写系统临时目录。检查退出码、实际选择、源码版本、测试终态及清理证据；跳过、零匹配、超时或缺证据不能算通过。失败保留原日志，补测使用新目录，不自动重试。运行内收集证据时用项目忽略的 `.steerrunner/artifacts/<唯一执行ID>/` 新目录。

结果和未验边界写入任务专属 [验收记录](testing/acceptance/README.md)，测试列表只说明测什么。报告格式、登记和复验见 [报告约定](testing/reports.md)。历史验收沿用其冻结版本与当时级别，不代表当前代码通过。

安装部署的历史生成记录接管修复见[专属验收](testing/acceptance/install-deployment-generation-20261022.md)：部署定向回归覆盖三代 Runner 构建及错误输入、命名空间、候选、缺失 Run 和只读证明；自动测试使用隔离现场，实际安装维护单独记录。

## 维护

修改测试内容时，同步实现、分级列表、JSON、[E2E 登记](../tests/e2e/suites.json)及执行器选择/证据校验。JSON 使用 `id/name/level/environment/command/check`，可选 `note`，不新增未支持的字段。ID 保持验证目的；改级后旧级别选择明确失败。

系统自动测试安排读取固定版本的 Plan、本文与 JSON，不展开链接文档。因此必跑规则保留在本文，各 ID 的环境、入口、主要断言及执行约束必须在 JSON 中自足。代码变更选择 L1、L2，再按影响补充 L3/L4；无需自动测试的 Plan 必须明确原因。实现 Plan 映射验收项、测试 ID、环境、命令、执行时点、通过条件及证据位置。

```powershell
python scripts/check_doc_links.py
go test ./cmd/test-feedback -run 'Test(TMRegisteredCatalogueMatchesExecutableManifest|ScenarioContentListMatchesRegisteredIDsAndLevels)$' -count=1
git diff --check
```
