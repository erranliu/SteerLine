# 三线解耦正式规划核对

日期：2026-10-04。源码核对基线：`28b81d82f4afb3e158cf96bd5cbf257fa48b7adc`，包含 #37 候选基础的 PR #206。仅核对规划文档，未启动实现或创建中心 DAG 任务。

#73–#82 由 `scripts/roadmap_task_id.py reserve` 在共享 Git ref 上原子分配，M01–M10 分别对应 10 份独立 Spec/Plan。正式规格入口见 [Spec 索引](../../spec/README.md)。

核对范围和结果：

- 10 对 Spec/Plan 的 ID、名称、硬前置与主 roadmap、execution-map、Spec 索引一致；依赖无环。
- #73 的唯一硬前置为已具备 #37；#77 的硬前置为 #74/#38；没有把 #39 新增为解耦任务硬前置。
- 改动 Markdown 的相对文件链接有效，表格列数一致；执行 Spec/Plan 不引用 roadmap 承载要求。
- 每份新 Plan 包含测试级别/组或场景 ID、环境、精确命令、时点、通过条件、证据位置与未选择级别理由。未来包/TestTM 入口明确为实施时接通，当前未注册的 candidates 场景不宣称可运行。
- 核对当前 `candidate_workers_test.go` 的 TestMC1/2/5/6 与集成包的 TestCD67CoreBoundary；修正 #39 的 MC5/6 当前主包命令。#76/#81 后续迁移时同步更新唯一实际入口。
- #38/#39 Spec 和 Plan 同步共享所有权、固定构建/资源隔离及按实际影响定向重验条件；远端准入缺口保留。
- `python scripts/roadmap_task_id.py check` 通过：83 个身份记录，下一个 ID 为 83。
- `git diff --check` 通过。

本次未运行 Go/L1/L2/L3/Full 或原生模型测试；实现、测试环境及运行能力均未改动。各任务自己的测试计划随实现执行，本文不作为产品测试通过证据。
