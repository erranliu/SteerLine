# #210 与 main 冲突集成验证

集成 main `a8ff0f01d38299d77ecc9cd4463b78a1286e1bec`（#38/PR208、#211 修复），PR 原父提交 `0d5020122c4785099b36d91350708d5c435f3766`。采用 merge 保留双方提交，不重写已推送历史。

四处冲突：console/model.go 的 Payload/DecisionIdentity/环境目标与候选操作；execution_profile.go 的固定 profile 优先读取；native_environment_cd67_test.go 的原始/改名 terminal 恢复等待条件与身份；requirement_admission_cd67_test.go 的必要发布政策负向输入。

保持 main 的候选 UI、精确候选/会话/回执关联与 #211 修复语义。新增 candidates.go、candidates_test.go、candidate_console_test.go 及共享段切换到 product/api、execution/model/backend 的唯一类型；无旧定义、alias 或转发恢复。[九文件比较](semantic-review.json)确认去除类型/import 归属差异后与 main 的行为一致。

Windows amd64 / Go 1.26.4；各 runner 使用独立 STEERLINE_HOME，无原生 opt-in 和 CC5 manifest。实际命令、parent/subtest、run/pass/skip 见 [coverage.json](coverage.json)，原始 Go JSON/stderr 各自保留。

| 定向选择 | 结果 | 报告 |
| --- | --- | --- |
| 全 console（含 CC1–CC4） | 67 parent/subtest run/pass，0 skip | [console](console/summary.json) |
| CandidateConsoleWiring、CD67 Core/Additional、原始/改名 terminal 恢复、必要发布资格 | 48 parent/subtest run/pass，0 skip | [integration](integration/summary.json) |
| backend Markdown PR clues 与 testprocess 后代清理修复 | 6 parent/subtest run/pass，0 skip；两个包均非零 | [repairs](repairs/summary.json) |
| Windows/Linux `go build ./...` | 退出 0；Linux 只编译 | [build](build.json) |

上述范围依 [Plan](../../../spec/three-line-01-types.plan.md) 的冲突集成补充执行。未重跑 Full/L2/L3/live 或全部 L1；保留原 #73 与 main 的既有验证，本次只重验接线和实际受影响行为。CC5 Windows 人工证据延期随 main 保留，不被受控 TUI 测试替代。

最新候选源码与实际 import 索引见 [source-index.json](source-index.json)；原 #73 source-index/coverage 和首次失败/修复报告保留原值。当前架构索引、test-mapping.json 已指向/登记此次集成。没有新的未解决冲突或验证问题；#74 后继归属及原身份/版本/提交屏障保持。
