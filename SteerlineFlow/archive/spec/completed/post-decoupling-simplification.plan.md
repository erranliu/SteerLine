# Plan：解耦后的代码精简

依据：用户明确要求执行死代码清理、重复小规则合并、严格 JSON 解码去重并提交 PR。基线 `ace6181`，#69–#72 已合入。

## 实施范围

1. 复核全 tracked Go（含测试、全部 OS/tag）、脚本、资源、接口和反射消费者，删除已确认无消费者的函数及其专属类型/import。保留接口隐式调用（例如 Bubble Tea Init）和当前格式拒绝。
2. 复用 execution.StableExecutionID，使用 slices.Equal 替换等价循环；中心日志与 console 共用 activityview 的纯答案摘要。删除原副本，不新增 package。
3. 在现有 execution 包统一严格单值 JSON 解码，codec 保留原始字节/BOM 行为、原尾随数据错误、身份/版本/大小校验与校验顺序。contracts 继续显式 TrimBOM。流式 journal 不改。

不改变端口、持久格式、原生调用、事务/恢复/intent/receipt/ack 屏障，不实施生命周期协调重写。净行数同时统计全仓 Go、非测试文件和主包直属文件；测试和文档新增量单列，不把搬包计作精简。

## 测试计划

环境：Windows amd64、临时 STEERLINE_HOME、隔离 Git/SQLite/协议 mock，不启用 live 开关。证据归 `bin/post-decoupling-simplification/`，提交摘要至 `docs/validation/post-decoupling-simplification/README.md`。

| 级别/组或场景 | 命令 | 时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| L1 定向：严格 JSON 边界 | `go run ./cmd/test-feedback -out bin/post-decoupling-simplification/json -- ./internal/steerline/execution ./internal/steerline/contracts -run '^(TestDecodeStrictJSON|TestContractDecoderPreservesStrictFieldsAndSingleValue)$' -count=1` | decoder 改动后 | 实际命中，未知字段/BOM/尾随值/损坏输入/空输入的接受与错误行为保持，summary.json complete/pass |
| L1 定向：codec 与展示 | `go run ./cmd/test-feedback -out bin/post-decoupling-simplification/focused -- ./internal/steerline/codexworker ./internal/steerline/orcaworker ./internal/steerline/orca_remote_worker ./internal/steerline/activityview ./internal/steerline/console -run '^(TestCD63CodecIdentityAndVersion|TestCD63BindingCodecIsolation|TestCD62NativeFactsAndQuestionRoute|TestDecisionAnswerSummary|TestDecisionAnswerEditorHandlesTwelveOptionsUnicodePasteAndStableCommandID|TestPendingDecisionPreviewHidesRunIdentity)$' -count=1` | 源码稳定后 | 各实际 parent/子例命中；原 codec 身份/版本拒绝及答案呈现不变，summary.json complete/pass |
| L1 quick：中心闭环、规则、边界 | `go run ./cmd/test-feedback -level=L1 -out bin/post-decoupling-simplification/quick` | 最终一次 | 全指定包/入口 run/pass，0 fail，summary.json complete/pass |
| 构建/静态 | `go build ./cmd/steerline ./cmd/test-feedback ./cmd/orca-probe`；Linux 同入口交叉构建；`git diff --check`；删除项消费者复核 | 提交前 | 两平台可构建；没有悬空引用/新增禁用依赖/格式问题；删除与净行数清单匹配 diff |

不选 Full、L2/L3：不改变真实启动、权限、外部协议动作、交付或恢复路径，既有 quick 的隔离闭环足够；不新增原生环境能力。
