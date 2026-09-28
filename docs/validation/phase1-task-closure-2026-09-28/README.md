# 原中心 #3 / #4 交付结项

2026-09-28，原中心 SQLite 的 Task #3、#4 均从 `in_progress` version 2 进入 `closed` version 3，并分别写入同一 `task_deliveries` 表。[结项后回读](receipt.json)记录 graph revision 16、两项 Delivery、原失败 Run、其他活跃 Run 和服务重启结果。

| Task | 固定 Spec 门槛 | 经 PR provider 核验的原实现 | Delivery 与历史 |
| --- | --- | --- | --- |
| #3 | H-A1–H-A16；逐项见[验收索引](../phase1-existing-acceptance-2026-09-28.md) | [PR #39](https://github.com/erranliu/SteerLine/pull/39)，head `c3bc30f25a4834fe1f7f3e647d3740b95b106c81`，merge `a8aca9769a4687501a94718480c212b70c268592`；另核验 #92/#93 | Delivery 固定原 PR；原 Run `c17f5ab4481a4125d45f64f99e4f8612` 仍为 blocked/failed，没有冒充新候选。 |
| #4 | P01–P13；逐项见[验收索引](../phase1-existing-acceptance-2026-09-28.md) | [PR #16](https://github.com/erranliu/SteerLine/pull/16)，head `292483eb389b188d5d93d048cc587d6e8e50bada`，merge `1a9f95ba96880f2ff7a9aea36325c70b2653e783`；另核验 #61/#97 | 原 Dispatch `ctx_06a3ae2ebfff` 已 abandoned 且无残留资源；原远端 terminal `term_0ac115ff-11f6-4b4c-9427-588310ad3ecb` 的 runtime/句柄/incarnation 匹配且为 `operator_close`。中心将唯一 unknown verify Attempt `dc2a08c8295fbfe4989d124339d674b2` 记为 stopped 后写 Delivery；原 Run 保持 blocked/failed。 |

结项前使用独立 `STEERLINE_HOME` 和原库 SQLite 一致性副本运行相同 manifest。副本 #3/#4 均关闭且后继图前进，#14/#15/#30 的 Run、Dispatch 和进程身份不变，项目占槽保持 true。原库操作前另保存 SQLite 与 runtime 投影备份；原库两个命令也分别返回 `closed` version 3，graph revision 15/16。

首次用新二进制读取活跃中心时发现当前格式的 `coordinator_terminal` 字段漏解码，由 [PR #102](https://github.com/erranliu/SteerLine/pull/102) 修正；副本对账又发现并行 worker 占槽须保留，由 [PR #103](https://github.com/erranliu/SteerLine/pull/103) 修正。结项后首次服务启动把所有 closed Task 错折叠为 succeeded Run，SQLite 拒绝该次事务；[PR #104](https://github.com/erranliu/SteerLine/pull/104) 修正并通过副本启动重建/保存测试。原库 Delivery 未回滚或改写。最终服务以 merge commit `02b126d261ad33653b81a30eeb7b8349c01a49a8` 的版本化二进制重新达到 `running/ready`，SHA-256 `AEC851E77F7C4D5E6874BEC3061C192B0E406DB74525C797735469997FC75D3C`；#14/#15/#30 的原远端 Dispatch 仍在运行且占槽。

此结项只针对原 #3/#4 Task 在创建前已合并的实现，不替代 #6 的当前目标故障验收或原 Task Delivery。长期无人值守矩阵仍属于 #32。
