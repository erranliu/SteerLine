# 解耦后的代码精简

日期：2026-10-04。用户授权执行死代码清理、重复小规则合并、严格 JSON 解码去重并提 PR。基线 `ace6181a050e7598e83a235b37bf625d7855289b`，分支 `codex/post-decoupling-simplification`。实施与测试范围见 [Plan](../../spec/post-decoupling-simplification.plan.md)。

## 删除与唯一所有者

[声明清单](declarations.json)保存原位置、动作、行数和原源码字节 SHA-256（含原 CRLF/doc comment），共 46 项：27 个无消费者函数、9 个专属类型/别名/常量、10 个已合并的函数副本。

消费者审查覆盖基线全部 tracked Go，包括各 OS/tag 和测试，以及脚本、CLI、嵌入资源和接口。27 个函数在 Go AST 中仅有自身声明，没有调用、函数值或同名接口成员；项目无 MethodByName/go:linkname 动态分派。Bubble Tea Model.Init 的隐式调用明确保留，未按没有显式调用删除。ansiGreen 在删除无人消费的 statusColor 后失去唯一消费者。原格式拒绝、协议枚举、有效能力及原生 opt-in 断言没有删除。

| 去重内容 | 唯一所有者 / 完整消费者 |
| --- | --- |
| 32 位小写十六进制 ID | `execution.StableExecutionID`；四份副本删除，参数/恢复、Orca worktree/relay/readiness、workerflow prompt 和 app 的目录函数值全部切换 |
| 等价切片比较 | `slices.Equal`；三份副本删除，准备输入、清理计划、会话能力及原前置追加测试全部切换；nil 与空切片继续相等 |
| 已保存答案摘要 | `activityview.DecisionAnswerSummary`；中心审计日志、TUI 任务详情和会话详情共用，未让 console 重新依赖中心 |
| 严格单值 JSON 解码 | `execution.DecodeStrictJSON`；14 个生产调用覆盖三个实现的 profile、问题路由、binding/resource/state、来源绑定/游标/marker 和 contracts；各 codec 保留身份/版本/大小与校验顺序、首个解码错误类型及原尾随错误文案；contracts 仍显式 TrimBOM，原生载荷不剥 BOM；流式 journal 与准备收据独立校验保持 |

`message_identity.go` 整文件删除；源码格式化仅限受影响文件。没有新 package、端口/持久格式改变、业务状态机重写或外部动作改变。

## 实际净行数

按 Git 基线和当前全部 Go 文件的物理行统计，含空行/注释；非 `_test.go` 口径包含测试辅助。文档不混入 Go 精简数字。源码集合以路径及 LF 规范化文件 SHA-256 排序后汇总，见 [源码摘要](source-summary.json)。

| 口径 | 基线 | 最终 | 净变化 |
| --- | ---: | ---: | ---: |
| 全仓非测试 Go | 64,293 | 63,895 | -398 |
| 全仓测试 Go | 32,585 | 32,679 | +94 |
| 全仓全部 Go | 96,878 | 96,574 | **-304** |
| 主包直属非测试 Go | 19,894 | 19,753 | **-141** |

## 验证

以下都对应同一 Go 源码集合 `2a6424de5c0d41ff11bd0e76150950d452c9ad2b0ef463cd53154fdef26b8f78`。运行时 HEAD 尚为基线，工作区包含本次变更；不冒充 clean-head 测试。之后只新增/更新验证文档，不改变已验证 Go 源码。

| 验证 | 实际结果 |
| --- | --- |
| 严格 JSON 与原 contracts BOM/未知字段/单值用例 | 2 包，11 个 parent/subtest run/pass，0 fail/skip；complete/pass；1.7s |
| 原 codec 身份/版本/问题路由/绑定及答案展示、TUI 决策 | 5 包，35 个 parent/subtest run/pass，0 fail/skip；complete/pass；5.1s |
| L1 quick，含中心闭环、SQLite、纯规则、全 OS/tag 守卫、console 守卫、第三实现接线 | 15 包，215 个 parent/subtest run/pass，0 fail/skip；complete/pass；准备 1.4s、执行 272.7s |
| 产品构建 | Windows/Linux amd64 的 steerline、test-feedback、orca-probe 全部通过；Linux 为交叉构建，未执行 Linux 测试体 |
| 静态 | 删除清单和实际 diff 核对；`git diff --check` 通过 |

精确命令见 Plan；原始报告在 `bin/post-decoupling-simplification/{json,focused,quick}/`，摘要与哈希见 [验证摘要](validation.json)。旧测试名称和断言保留，只有前置追加测试改用等价 slices.Equal；新增解码拒绝/错误类型和答案摘要用例。没有用零匹配或编译代替测试命中。

首次构建定位到三个历史来源 helper 消费者及自动 import 整理遗漏，全部修正后构建和上述验证通过；未删除断言或豁免失败。测试仅使用临时 STEERLINE_HOME、隔离 Git/SQLite/协议 mock，不启用原生验收开关；没有操作真实任务数据库或产品服务。未跑 Full/L2/L3/live。
