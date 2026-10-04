# Plan：第一阶段集成验收

任务 ID：42；任务名称：第一阶段集成验收。唯一执行依据：[Spec](phase1-release-acceptance.spec.md)。硬前置：#39、#41，一个实现 PR 交付。

1. 核对全部传递本机前置交付、目标及能力，选择干净组合构建并生成仅包含本地 Codex/本机 Orca 的固定 manifest 和证据索引。远端目标、权限回执、远端/混合场景不在一期范围，不作为阻塞或待补验。
2. 在 stable 原生入口复用 #39 的本机多候选能力，完成两条本机依赖链：每条一个双候选 Task 加一个后继，两种本机后端均覆盖；核对真实问答、人工合并、唯一交付及后继 base/执行。
3. 执行连续 24 小时 stable 样本并交付至少五个任务，并行 dev select 与调试；按 60 秒采样、预热/初末空闲窗口和固定资源/待办阈值验收，覆盖率或指标不达标时修复并重跑。
4. 在持续运行窗口之外完成 #41 受控提升和当前格式重启，核对原任务/请求/PR 身份以及未知执行提升拒绝。
5. 完成 P1A1–P1A5 的验收矩阵、运行说明与残留资源清单，保存 docs/validation/phase1-release-acceptance/；本任务不修改 #53 的固定契约或直接写入其依赖。

## 测试计划

本任务实施时提供本机组合入口 `scripts/phase1-local-acceptance.ps1`，输入固定本机 manifest 和本次空证据目录；该脚本当前为规划入口，不宣称已经存在或已执行。复用版本、隔离、任务/候选和当前 E2E 适配层，不另建远端能力或全量后端矩阵。实现路径若变化，同次更新本 Plan。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 本机组合：P1A1、P1A4、P1A5 | 隔离 Windows stable/dev、本地 Codex + 本机 Orca、固定构建/仓库/home | `./scripts/phase1-local-acceptance.ps1 -Manifest <本机manifest绝对路径> -EvidenceDir <本次空目录>`；实施时登记实际可运行参数 | 全部前置交付且本机目标核对后 | 两条本机依赖链、唯一交付/后继 base、精确原身份与证据交接通过 | phase1-release-acceptance/chains/ |
| 本机持续运行：P1A2 | 同一固定 stable 构建/home，dev 独立构建 | 同一组合脚本的固定 24 小时运行/采样步骤，实施时记录实际命令 | 两条链准备完成后 | 至少五个真实本机任务；24 小时、采样覆盖、同负载增量及资源结算全部满足 Spec | phase1-release-acceptance/soak/原始采样与任务回执 |
| 本机提升/恢复：P1A3 | 同一隔离本机场景，原会话/数据库/请求保留 | 同一脚本调用 #40/#41 的实际固定入口；实施时登记版本选择/重启命令 | 持续运行窗口之外 | 当前格式重接原会话、一个受控提升、活动/unknown 提升拒绝通过，不重发 | phase1-release-acceptance/promotion/ |
| L1 固定最小入口 | 隔离 STEERLINE_HOME，无 live 开关 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/phase1-release-acceptance/l1` | 集成修复和自查完成后一次 | 固定组非零命中且无 fail/skip，业务/依赖守卫通过 | l1/summary.json 与原始日志 |

需要新增自动用例时，在实际受影响包登记 P1A 到测试的唯一映射及定向命令；当前纯文档收缩只检查任务身份、依赖、链接与范围，不启动模型或长运行。每次新空证据目录，不覆盖失败证据，不默认跑 Full、完整 L2 或无关 L3。人工操作未执行可登记 roadmap，但已发现的真实失败不能改写为待人工通过。
