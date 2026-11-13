# 人工核验验收计划

SteerLine 已接入实现，使用独立 Runner 构建验证。真人签收待安排。[Spec](../../spec/manual-verification.spec.md) · [Plan](../../plans/manual-verification.plan.md) · [测试选择](../../plans/manual-verification.tests.json)

| ID | 方式 | 步骤与通过条件 | 触发与范围 |
| --- | --- | --- | --- |
| MV-A1 | 自动 | 通过 Plan 生成及候选准备入口，覆盖合法文件、无文件和错误字段、重复 ID、越界目录、非法 URL、非法占位符。确认同批文档产物和固定提交来源。Plan 更新后旧候选仍读取原冻结要求。 | P1 完成后。覆盖 MV1。 |
| MV-A2 | 自动 | 用公开 Runner/PR 夹具验证首次发布输入的草稿意图、平台确认、重复/未知回执、普通 PR 和缺字段诊断、原 PR Ready、并行候选和直接合并。核对无核验候选的原交付和 DAG 后继。 | P2 至 P4 稳定后。覆盖 MV2、MV4、MV5。 |
| MV-A3 | 自动 | 在隔离项目通过浏览器启动两个候选的真实 Git 提交和受管本机程序，核对参数、cwd、Head、URL、进程和日志。覆盖失败重试、并发点击、刷新、服务重启、停止、清理、旧 Head 保存被拒绝、脏 worktree 保留和历史结果。 | P4 完成、Web 生产构建后，Windows/Go/Git/Node/Edge，公开 Runner/PR 夹具及本地远端。覆盖 MV2–MV4。 |
| MV-A4 | 自动 | 用受支持的固定 Runner 和专属 GitHub 目标提交候选。直接查询平台证明确已创建草稿且当时无人签收。用真实 PR Head 启动预览、推送新提交并核对待办切换。完成结果后在平台标记 Ready，合并专属 PR，核对唯一交付及资源清理。 | P4 完成，Runner 构建与专属目标具备后单独执行。覆盖 MV2–MV5。 |
| MV-M1 | 人工 | 打开「人工核验」，选择专属草稿 PR，核对页面命令和 SHA 后启动。打开预览并逐项记录一次通过和一次失败。在新提交上重启，确认旧结果可查且当前进度清空。停止并按页面提示清理，确认历史保留。 | 自动链稳定后安排。参与人和窗口待定，具体构建和启动入口在验收前固定。覆盖 MV3–MV5。 |

执行后按次记录受测 SHA、构建及环境、实际操作人或测试入口、执行时间、原报告和证据位置、通过或失败及未验范围。受控与真实结果分别登记。 [验收流程与记录](README.md#每项验收记录)

## 草稿发布前置

2026-11-13 核对 GitHub 上 SteerRunner `main` 和本机固定构建。两者来源均为 `8be179f29e0b9eac8572f447439a15cd7d776213`。固定 EXE `0.2.0-dev.4` 的 SHA-256 为 `b6bcf5631dc3aa3687723400a73bce2794094e15eb1e39d1d3ba2c6dccdc7bb9`。离线能力返回的 `command` 发布设置仅支持 `base_url`、`query` 和 `authentication_refs`，且拒绝额外字段。

独立 Runner 分支 `codex/manual-verification-draft-publication` 增加 `publication.settings.draft` 和平台草稿证据。固定本机提交 `9700ba3016a78a454a4e4b3d3965c13c616058ad`，构建版本 `0.2.0-dev.4-draft`，SHA-256 `c9a9938a04eca65b33a8329d663b84e225ae58f2addffe752b8fd985663d730b`。构建清单标为 dirty，原因是隔离 checkout 中留有先前未跟踪的测试 EXE，已提交源码可由该分支复核。真实运行未使用安装目录。Runner 分支待上游合入。

## 提示词变更与独立审核

SteerLine 生成规范 `internal/prompts/materials/roadmap-documents.md` 与固定仓库副本 `SteerlineFlow/Roadmap/document-rules.md` 的规范 UTF-8/LF 均为 2,626 → 3,308 字节。短指令正文保持原 548 字节。新增正文为：

> 仅当任务必须由人观察或操作时，在 documents 项内返回可选 manual_checks 对象，并在 Plan 引用其中的核验项 ID、说明人工判断的原因。按 output_schema 填写版本、启动程序及参数、核验步骤和可观察的通过条件。启动命令以固定提交中的项目启动材料为依据，可用相对于 worktree 的 cwd；本机预览使用 http://127.0.0.1:${PORT}，args 中也使用同一个 ${PORT}。通过显式解释器和脚本参数表达准备或构建步骤。无人工核验时省略该对象；需要人工核验却缺可验证的启动依据时返回待澄清问题。平台编码并发布同名 .manual-checks.json。

该规则使生成结果携带可审阅的启动依据，模型只选填该对象，服务端负责校验及 Git 文件编码。独立审核确认固定 Git 提交和同源规则接线，另指出评审编辑和旧伴随文件删除缺口，已补齐。规则摘要在独立审核后更新，固定 L1 核验同源正文、摘要及源码边界。

Runner 的 `publish` 默认正文沿用原版，草稿模式在 `publication.target` 追加 13 字节的 `,"draft":true`，及 141 字节的固定模板（含首尾换行）。新增正文为：

> 首次创建 PR 时使用平台的草稿模式（GitHub CLI 使用 gh pr create --draft）。如果原 PR 已绑定，继续使用原 PR。

独立审核发现线索提前绑定可绕过首次草稿核验、Head 需严格校验以及模板未纳入资源摘要。修订后两条绑定入口共用草稿资格校验，保留 `initial_draft_verified`，固定模板计入资源摘要并登记公开模型输入契约。Runner 发布、资源及提示词定向测试通过。

## 验证记录

SteerLine 基线 `97590c29` 加本次工作区变更，Windows，独立 home 和专属远端分支。固定 L1、L2、Web 生产构建、核验 JSON 的定向测试、Plan 生成结果采纳、真实 Git detached worktree 的 Head 与脏状态检查、Web 组件命令及旧 Head 进度隔离通过。L1 报告 `20261113T045547.843501300Z/summary.json`，L2 报告 `20261113T045630.353267700Z/summary.json`，均位于本机 `%TEMP%/steerline-test-feedback/`。

真实 PR #831 的业务断言通过，报告目录 `20261113T050720.007024700Z`。平台确认初始草稿，核验记录先于人工签收生成。启动两个实际 Head、首版记录失败、拒绝过期 Head 的写入、新版通过、停止并移除两个 worktree、Ready、合并和唯一交付均有原证据。该轮整体因新目录缺少状态白名单而判为失败。修正后用当前受管清理入口单独核对该现场，返回 `confirmed=true`。另有早期联验分别停在环境准备和阶段确认，未计入通过。 [专属测试 PR #831](https://github.com/erranliu/SteerlineTest/pull/831)

`manual-verification.local-codex` 正式通过，报告目录 `20261113T052427.391531400Z`，`scenario.json` 确认资源清理完整。Edge 从项目侧栏进入页面，启动首个 Head `43fc5bdc77f093eaecd0e88920c3c8e17c6027ae`，访问本机预览并保存失败。推送 `6af527cbbf1f82cde36ce760894791f1b75db5df` 后，API 拒绝原 Head 写入，新 Head 启动并逐项保存通过。两个进程和 worktree 均已停止并清理，PR 标记 Ready、合并后仅生成一条任务交付。该轮用已冻结的手工伴随文件，以生成结果受控测试和真实 Git 文档发布测试独立覆盖 Plan 生成到文件落盘。隔离 fixture 使用开发构建的 `skip_environment_readiness`，这轮未验证项目环境检测。 [专属测试 PR #835](https://github.com/erranliu/SteerlineTest/pull/835)

真实 Git 文档编辑、发布和删除旧归属文件的 CRLF 回归单独通过。`go.http` 和固定构建的 `go.runner-contract` 通过。`go.roadmap` 全集有 Git 超时及活动投影断言失败，`go.storage` 的活动分页关联断言未通过，`go.runner-result` 的环境结果封存子项报 `pending/read_failed`。对应报告目录为 `20261113T051210.642120300Z`、`20261113T051855.264281600Z`、`20261113T053600.083914200Z`。完整的双候选受控浏览器矩阵及真人验收仍待完成。

## 合入后核验与关闭待办

长期规则见[人工核验设计](../../../docs/product-design/manual-verification.md)。新增定向验证覆盖合入时的 PR Head、远端目标分支 Head、远端分支在解析后移动时拒绝启动、同 SHA 不同来源分别记录、全部结论齐备后关闭，以及重新打开。Web 组件、生产构建、文档链接和测试清单检查通过。固定 L1 和 L2 在常规系统临时目录通过，报告分别为 `20261113T055808.822860600Z/summary.json` 和 `20261113T055938.668311100Z/summary.json`。初次沙箱运行的 Git 配置写入及报告原子重命名遭拒，结果未计入通过。

首轮扩展真实链已验证两个合入后来源的实际 worktree 与远端结论，随后在等待关闭状态时超时，报告 `20261113T055342.473511100Z/summary.json` 确认清理完成。关闭操作已修复为在原记录上保存状态，定向 Go 测试覆盖持久化前的状态更新。 [专属测试 PR #836](https://github.com/erranliu/SteerlineTest/pull/836)

修复后的 `manual-verification.local-codex` 通过，报告 `20261113T060502.616183200Z/summary.json`。Edge 在草稿阶段从页面启动并记录，API 核对旧 Head 拒绝写入、合入后两种来源的实际提交和预览、结论隔离、关闭、重新打开和再次关闭。四次 worktree 均已移除，记录一次任务交付，正式报告确认资源清理。合入后来源选择与关闭的页面操作由组件测试覆盖。 [专属测试 PR #837](https://github.com/erranliu/SteerlineTest/pull/837)

## 变基后 Roadmap 回归

变基后模拟 PR 创建回执丢失时，活动仍在等待核实，但真实 Git 准备及推送超过一分钟后，原观察按现有规则已过期。测试现在按读取时刻校验新鲜度，并断言观察时间仍是回执丢失前的成功观察。补测以 `32476518` 加本次测试改动运行，真实 Git 发布和准入的四个场景、CRLF checkout 和人工核验固定提交测试全部通过。原始报告位于本机 `%TEMP%/steerline-test-feedback/20261113T062631.785845100Z/summary.json`。该次运行是针对受影响用例的补测。
