# Roadmap 静态初始化闭环 Plan

依据 [Spec](../spec/roadmap-static-initialization.spec.md)。普通开发，不启动 roadmap-runner，不创建开发 Task。

1. 在原 Roadmap 端口增加准备状态和显式初始化动作，复用 PrepareStatic、Git 读取/工作区 guard、命令日志和回执；保持材料发布与初始化分开。准备状态只有固定提交事实，缺材料提前阻断。
2. 初始化动作与同号查询串行，结果缺失时只读核对“确保静态文件完整”的原意图；没有原完成记录时不自动重跑。固定来源就绪缓存只保存两个不可变提交的核验结果，工作树和引用每次重新观察。
3. Web 显示材料路径、初始化入口与待提交提示，按项目隔离未决操作。调试包在初始提交前准备同源材料，保留已有指南。
4. 在仓库外独立 clone 为 SteerlineTest 准备三份固定资产、初始化说明和导航，运行该项目最小 L1/L2，提 PR。测试采用该 PR 的精确提交，不改安装版数据库/服务或直接更新真实 main。
5. 原生整链复用原 Go/Runner/Codex、隔离 Git/provider/SQLite 入口，读取真实 sample-demo，核验六个内部代号/六条直接边、十八份完整文档及正常结算。只验证规划/发布/入图，不执行201–205业务任务。

## 验证计划

证据根：`C:\SteerlineDebug\wt-762b-roadmap-init`；各调用使用独立目录，不覆盖失败。项目自身报告按其规则留在独立 clone 的 `.steerrunner/artifacts/`。

| 覆盖 | 命令与环境 | 通过条件 |
| --- | --- | --- |
| RI-01–03 | `go run ./cmd/test-feedback -- ./internal/app ./tests/e2e/internal/driver -run '^Test(RoadmapStaticInitialization|RoadmapInitializationPending|RoadmapPublishedGuide|DebugRoadmapMaterials)' -count=1` | 正反例全部实际通过、零 skip；含真实 Git/SQLite 日志及固定来源缓存边界 |
| RI-03 | Roadmaps Vitest 定向及完整 web.unit | 初始化/待发布/跨项目 pending 行为正确，无挂载副作用 |
| RI-04 | SteerlineTest `python tools/test-project.py --level L1`、`--level L2` | 同源资产与导航、旧样本核心闭环通过；无真实业务派发 |
| RI-04 | 显式 `-tags=e2e ./internal/app -run '^TestRoadmapPreparedSteerlineTestLocalCodexPromotion$' -count=1 -timeout=25m`，固定独立 STEERRUNNER_EXE/STEERLINE_ROADMAP_SOURCE_REPOSITORY/STEERLINE_ROADMAP_EVIDENCE | 实际PR固定源码和六节点草稿；真实模型、文档发布、DAG、原请求/停止证据齐全，零原安装状态写入 |
| 回归 | 固定 L1/L2、web.unit、go.http；按影响补原 Roadmap 发布 owner | 实际清单/非零/退出0、无意外 skip，HTTP 原请求渠道保留 |
| 治理 | doc links、diff检查、固定正文/资产字节与摘要核对、独立只读审核 | 规则正文0变化，资产同源，无 inline fallback，审核发现修复补验后闭合 |

结果写入 [专属验收](../testing/acceptance/roadmap-static-initialization.md)，不预填通过。新 UI 不在本轮自动部署；现有安装项目需材料 PR 合入 main 并正常更新后才满足固定来源门槛。
