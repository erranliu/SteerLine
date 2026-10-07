> 退役执行草案：原任务 #42 为人工组织验收活动，不作为当前执行入口。编号保留历史且不复用；现行行为、状态和安排由[验收文档](../../../acceptance/phase1.md)维护。本文件保留原契约/计划供追溯，不修改外部既有 Task/DAG，也不执行下列旧计划。

# Plan：第一阶段集成验收

任务 ID：42；任务名称：第一阶段集成验收。唯一执行依据：[Spec](phase1-release-acceptance.spec.md)。硬前置：#39、#83，一个实现 PR 交付。实施前按 Spec 核对中心固定契约；本文件不改写旧 Task。

1. 核对本机前置、当前中心契约及能力，选择干净组合构建，生成安装 exe/摘要/commit/home/config 与双本机后端的基线清单。#40/#41、远端及跨格式兼容不在本次门槛。
2. 扩展现有 E2E 适配层以选择精确安装产物及持续运行现场；不能用 runner 重新构建的另一个 exe 代替安装链证明。复用 #39 夹具和交付/恢复断言，不复制业务实现。
3. 在同一个 24 小时固定安装版现场中完成两条本机依赖链及一个 single 任务，核对真实问答、人工合并、唯一交付、后继 base 和资源结算；并行做一次独立源码调试构建/停启。按 Spec 的 60 秒采样、预热/初末空闲窗口和固定阈值验收，失败修复后重跑。
4. 在持续运行窗口之外，用同一安装 exe 完成当前格式等待合并重启及交付，核对原任务/执行/会话/请求/PR，无替代 worker；不做版本选择或提升。
5. 完成 P1A1–P1A5 的验收矩阵、运行说明与残留资源清单，保存 docs/validation/phase1-release-acceptance/；本任务不修改 #53 的固定契约或直接写入其依赖。

## 测试计划

本任务实施时提供 scripts/phase1-local-acceptance.ps1，接收 Baseline（安装基线清单）、Mode（Soak/Restart）及本次空 EvidenceDir；这是待交付的规划命令，不宣称当前可运行。Soak 在一个现场内执行两条依赖链、single 任务、调试共存和 24 小时采样，下表前两行属于同一次运行。Restart 使用同一固定安装产物，另建等待合并现场。沿现有 testenv/driver/native/evidence 扩展安装产物选择、持续采样和手动合并记录能力；实现路径或参数变化时同步本 Plan。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 独立本机组合：P1A1、P1A4、P1A5 | Windows、精确安装 exe/home、双真实本机后端及测试 clone | ./scripts/phase1-local-acceptance.ps1 -Baseline <清单绝对路径> -Mode Soak -EvidenceDir <本次空目录> | #39/#83 交付且本机能力核对后 | 两条链真实交付、唯一 Delivery/后继 base 及安装入口摘要一致 | 本次证据目录/chains/ |
| 独立本机持续运行：P1A2 | 上行同一安装现场，调试使用独立 clone/home | 上行同一次 Soak 命令，不重复跑业务链 | 24 小时窗口内交付五任务；末尾全部结算后取空闲窗口 | 真实 24 小时、采样覆盖、同负载资源阈值、调试共存及清理符合 Spec | 本次证据目录/soak/、coexistence/ |
| 独立本机恢复：P1A3 | 同一安装产物，另建当前格式等待合并现场 | ./scripts/phase1-local-acceptance.ps1 -Baseline <同一清单> -Mode Restart -EvidenceDir <本次空目录> | 持续运行窗口之外 | 同构建重启保持原身份并完成交付，无第二 worker/PR/Delivery | 本次证据目录/restart/ |
| L1 固定最小入口 | 隔离 STEERLINE_HOME，无 live 开关 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/phase1-release-acceptance/l1` | 集成修复和自查完成后一次 | 固定组非零命中且无 fail/skip，业务/依赖守卫通过 | l1/summary.json 与原始日志 |

需要新增自动用例时，在实际受影响包登记 P1A 到测试的唯一映射及定向命令；当前纯文档收缩只检查任务身份、依赖、链接与范围，不启动模型或长运行。每次新空证据目录，不覆盖失败证据，不默认跑 Full、完整 L2 或无关 L3。人工操作未执行可登记 roadmap，但已发现的真实失败不能改写为待人工通过。
