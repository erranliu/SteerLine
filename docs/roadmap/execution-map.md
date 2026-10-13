# 正式执行 Roadmap

本表是用户确认的开发任务与依赖权威。2026-10-13 用户确认当前规划收敛到 `TUI → HTTP API → Go 后端`；随后用户直接授权当前聊天实施 #91，不启动 roadmap-runner 或新执行聊天。流程见[RoadmapGuide](RoadmapGuide.md)。

## 当前任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 91 | TUI 经 HTTP 接入 Go 后端 | 无未完成硬前置；复用已交付 #45 与现有 TUI | 源码/自动验证已交付；待独立复审/合入 | [Spec](../spec/tui-http-client.spec.md) · [Plan](../spec/tui-http-client.plan.md) | [专属验收](../acceptance/tui-http-client.md)：TH-A1–A6 通过，TH-M1 待验；[草稿 PR #299](https://github.com/erranliu/SteerLine/pull/299)，未合并 |

## 推进边界

当前只有 #91：补齐实际 TUI 的 HTTP 能力、切换客户端接线、删除 #44 WinUI 实现/专属配置、验证本机完整链路。Go 保持业务权威，TUI 界面/执行功能保留；不直接访问业务库或宿主业务文件。

旧二阶段 #43/#46–#52 及延期 #40/#41 退出规划，编号不复用；旧三节点和页面不作为前置。WebUI、SSE、主 Agent 和远程部署不在本轮交付，原型仅参考；后续需要时重新规划，不从归档派发。

启动需固定已提交的实际 Spec/Plan commit/路径，核对主干与任务状态。本次由用户直接授权实施；Spec/Plan 固定 commit `83a6cc4a7321994a55e5a9ae0f308927eb9fb1b3`，执行起点 `efd492cac9940a50966b3ccdb65d56012931d1d6`，分支 `codex/tui-http-client`。交付后登记自动验收、必要回归、独立复审和真实合并，再更新本表；TH-M1 可按 Plan 后置，不将实现完成写成全部已验。

## 待人工验收

最终自动验证固定 commit `2c58de3207ab76aeec5e69ee8421787137051183`（clean）：新独立 simulate 包的真实 TUI/HTTP/Go 场景和 L1 均通过。原生 final 明确来源缺口，未验声明保持，完整正文 HTTP 消费另有测试。精确构建/请求/停止与失败修正证据归专属验收。独立复审和真实合并未完成，暂不按 roadmap 规则记实现完成，不移除草稿条目。

| 范围 | 状态 / 安排 | 明细 |
| --- | --- | --- |
| #91 TUI 输入与操作 | TH-M1 待验；待自动验证/独立包就绪，参与人/窗口待安排，最迟本任务签收 | [记录](../acceptance/tui-http-client.md) |
| 一期单安装版签收 | 待当前支持路线能力/原生准备、固定新安装构建后组织组合/24小时/同构建恢复；窗口待安排，不新增执行任务或成为 #91 无关前置 | [一期活动](../acceptance/phase1.md) |
| #84 步骤辨识与反馈/合并 | WFS-M1 待验；可与一期同现场分别登记，不阻塞无关客户端改造 | [记录](../acceptance/worker-staged-flow.md) |

## 已交付基线与历史

#45 HTTP 基线经 PR #249 合入；#83 隔离、#84 分步和 #87 停服恢复保持原交付/限制。原汇总、PR、失败/风险接受及未验事实完整保留在[清理前执行历史](../archive/roadmap/execution-map-before-tui-http.md)，不重新派发。

#44 经 PR #273 的历史交付不撤改；用户决定删除实现，#91 当前源码已删除工程与专属配置。UI2-M/UI4-M/UI5-M 原未验见[退役记录](../acceptance/archive/superseded/phase2-winui-foundation.md)，退出当前签收要求，不标通过。旧规划见[归档说明](../archive/roadmap/README.md)。

自动补验仍归原范围：[Codex race](../acceptance/local-codex-app-server.md) · [当前 Orca 单终端](../acceptance/orca-terminal-transport.md) · [远端有限样本与限制](../acceptance/orca-terminal-remote-retest-2026-10-10.md)。当前 Orca 本机最终 draft 保护未跑真机、远端首次信任阻塞后原任务自动继续未通过；旧证据不覆盖当前全路线，也不证明 #91 或远程服务部署可用。
