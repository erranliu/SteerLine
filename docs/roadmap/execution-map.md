# 正式执行 Roadmap

本表是用户确认的开发任务与依赖权威。2026-10-13 用户确认当前规划收敛到 `TUI → HTTP API → Go 后端`；随后用户直接授权当前聊天实施 #91，不启动 roadmap-runner 或新执行聊天。流程见[RoadmapGuide](RoadmapGuide.md)。

当前已确认的本机链路已交付。2026-10-14 用户确认将HTTP解耦任务#92、#93纳入本表；#92已独立复审、自动验证并经PR #311真实合并，#93候选实现通过本地自动验收（A2补强），PR #312待独立只读复审与真实PR合并。本次runner按#92→#93顺序推进，不改变两项没有业务硬依赖的规划条件。

## 待执行任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 93 | HTTP 客户端原请求与停服观察解耦 | 无未完成硬前置；复用#91；与#92独立并行，新增写入仲裁仅属客户端journal | 候选自查及自动验收通过（A2补强后复验），PR待独立复审与合并；代码起点 `ab814da9d5fcec17f3ba46b59827734d171ba6b7`，分支 `codex/http-client-request-lifecycle-93` | [Spec](../spec/http-client-request-lifecycle.spec.md) · [Plan](../spec/http-client-request-lifecycle.plan.md) | [专属验收](../acceptance/http-client-request-lifecycle.md)：CL93-A1–A3与固定L1通过；A2已补充真实submitted观察的四种进程交错及mutation反证；无新增人工项；待独立只读复审与PR合并；PR [#312](https://github.com/erranliu/SteerLine/pull/312) |

### 本批并行与开工条件

#92拥有httpapi与app/execution_service_wiring.go的HTTP endpoint构造段；#93拥有httpclient私有journal/OS观察和consoleapp构造。二者互不消费对方新增实现，public API/httpcontract、httpfs格式/ACL和业务规则保持；不存在#92→#93或反向依赖。后合入者在最终候选按各自Plan复验实际TUI/HTTP应用接线，测试/调试资源按既有隔离要求安排。

用户另告知Worker三层职责重构已在另一worktree实施，其约定保持公开HTTP/DTO。#92不得修改worker三层、actor/可靠性或adapter组装；共用execution_service_wiring.go只改HTTP endpoint段，必要冲突由后合入者保留两方成果并定向验证。#93不涉及该文件/业务服务。若实际发现公开签名或所属接线被改动，先重核契约/消费者；不能覆盖在途工作或消费猜测接口。当前不以三层重构完成作为两项的硬前置。

本表只确认范围与调度条件，不代表已创建执行聊天/worktree。启动时固定已提交且执行工作区可读取的实际Spec/Plan commit及路径，再核对最新默认主干和真实在途修改；每项独立实现、复审、必要验证、真实PR合并及验收同步后记为实现完成。

#94/#95/#96继续保留在[草稿及冲突核对](drafts/roadmap.md#与worker三层重构的冲突核对)，不纳入本批：#94/#96的规则/生命周期所有者与三层重构重叠；#95的最终交付抽取有独立收益，但actor/PR桥接与记录转换的当前接线尚在重组。三层成果进入主干后按真实剩余范围重新审计，不自动转正或将这些草稿建议变成执行前置。

## 已交付任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 92 | HTTP 服务端命令边界收敛 | 复用#45/#91，无未完成硬前置 | 实现完成；[PR #311](https://github.com/erranliu/SteerLine/pull/311)已真实合并，merge `e3010edcb4905977e6244f2f896be140fa718a8f`；三项P2已修复并独立终审通过 | [Spec](../spec/http-command-boundary.spec.md) · [Plan](../spec/http-command-boundary.plan.md)，固定版本及起点 `35d4d65350fa6f6f284e9e91a6bd657ba7abb947` | [专属验收](../acceptance/http-command-boundary.md)：HC92-A1–A3通过；HTTP60项/app2项及clean最终head CLI3项通过，历史固定L1 12组/553项经核验复用，无新增人工项；[复审记录](https://github.com/erranliu/SteerLine/pull/311#issuecomment-6165070946) |
| 91 | TUI 经 HTTP 接入 Go 后端 | 无未完成硬前置；复用已交付 #45 与现有 TUI | 实现完成；独立复审/自动验证/真实合并已核验 | [Spec](../spec/tui-http-client.spec.md) · [Plan](../spec/tui-http-client.plan.md) | [专属验收](../acceptance/tui-http-client.md)：TH-A1–A6 通过，真终端/live 操作已验；TH-M1 窗口渲染/人工体验待验；[PR #299](https://github.com/erranliu/SteerLine/pull/299) 已合并 |

## 推进边界

本轮 #91 已补齐实际 TUI 的 HTTP 能力、切换客户端接线、删除 #44 WinUI 实现/专属配置并验证本机链路。Go 保持业务权威，TUI 界面/执行功能保留；不直接访问业务库或宿主业务文件。

旧二阶段 #43/#46–#52 及延期 #40/#41 退出规划，编号不复用；旧三节点和页面不作为前置。WebUI、SSE、主 Agent 和远程部署不在本轮交付，原型仅参考；后续需要时重新规划，不从归档派发。

启动需固定已提交的实际 Spec/Plan commit/路径，核对主干与任务状态。#91当时由用户直接授权实施；其Spec/Plan 固定 commit `83a6cc4a7321994a55e5a9ae0f308927eb9fb1b3`，执行起点 `efd492cac9940a50966b3ccdb65d56012931d1d6`，分支 `codex/tui-http-client`。交付后登记自动验收、必要回归、独立复审和真实合并，再更新本表；TH-M1 可按 Plan 后置，不将实现完成写成全部已验。此历史版本不用于新任务#92/#93启动。

## 待人工验收

最终自动验证固定 clean commit `24cdd7a116912ef43d4aa845b8a5830c552a21a3`：四个消费/API/文件系统包、边界、L1（12 组/26 包）及新独立 simulate TUI/HTTP/Go 场景通过，原 applied/精确退出/两锁释放及 Job 清理齐备。独立复审八项发现全部修复，最后修复复核无新增问题；Windows discovery 竞争经稳定复现及新包复验修正。#299 实际 merge `56552550641dc787c794bc6e486c94d41b90b0ad` 已进入 origin/main，两个父级与已核对的 base/head 一致；实现完成并移除主草稿任务行。12dbad80 的独立 live/ConPTY 有限样本、各版本/失败、simulate 缺口及剩余人工判断继续分别登记在专属验收，不将合并记为全量验收通过。

| 范围 | 状态 / 安排 | 明细 |
| --- | --- | --- |
| #91 TUI 输入与操作 | ConPTY 键位/Unicode 多行/候选/反馈/final/恢复/停服已验；TH-M1 原生查看窗口渲染、桌面输入法/字体体验待人工判断，最迟本任务签收 | [记录](../acceptance/tui-http-client.md) |
| 一期单安装版签收 | 待当前支持路线能力/原生准备、固定新安装构建后组织组合/24小时/同构建恢复；窗口待安排，不新增执行任务或成为 #91 无关前置 | [一期活动](../acceptance/phase1.md) |
| #84 步骤辨识与反馈/合并 | WFS-M1 待验；可与一期同现场分别登记，不阻塞无关客户端改造 | [记录](../acceptance/worker-staged-flow.md) |

## 已交付基线与历史

#45 HTTP 基线经 PR #249 合入；#83 隔离、#84 分步和 #87 停服恢复保持原交付/限制。原汇总、PR、失败/风险接受及未验事实完整保留在[清理前执行历史](../archive/roadmap/execution-map-before-tui-http.md)，不重新派发。

#44 经 PR #273 的历史交付不撤改；用户决定删除实现，#91 当前源码已删除工程与专属配置。UI2-M/UI4-M/UI5-M 原未验见[退役记录](../acceptance/archive/superseded/phase2-winui-foundation.md)，退出当前签收要求，不标通过。旧规划见[归档说明](../archive/roadmap/README.md)。

自动补验仍归原范围：[Codex race](../acceptance/local-codex-app-server.md) · [当前 Orca 单终端](../acceptance/orca-terminal-transport.md) · [远端有限样本与限制](../acceptance/orca-terminal-remote-retest-2026-10-10.md)。当前 Orca 本机最终 draft 保护未跑真机、远端首次信任阻塞后原任务自动继续未通过；旧证据不覆盖当前全路线，也不证明 #91 或远程服务部署可用。
