# 退役消费者清理验证

范围依据：[Spec](../../spec/retired-consumer-pruning.spec.md)、[Plan](../../spec/retired-consumer-pruning.plan.md)。基线 main 9413865，含 #202–#204；最后一次 rebase 只加入候选文档，不改变已测试源码。

## 精简结果

| 范围 | 净变化 |
| --- | ---: |
| 主包直接目录生产 Go | -313 行 |
| 全仓生产 Go | -526 行 |
| 测试 Go | -272 行 |
| 全仓 Go，含测试 | -798 行 |

统计物理 Go 行，含空行/注释；生产排除 _test.go。逐文件数据见 [source-summary.json](source-summary.json)，测试减量单列。

## 删除与保留映射

- 日志解析：删除没有生产构造或调用的 Pi/旧 exec stdout normalizer、DetectNormalizer、DecodeExecLine、格式探测及 fallback，连同复制旧 stdout stream 的测试 fixture。旧 Codex 文件的 clip 仍用于实际 history user-agent 元数据，原实现移到 conversation_adapter.go；当前会话 source/mapper 与协议未改。
- 通用日志 framing：ReadJournalBatch 保留。移除无人调用的 Activity 模式、seq 去重/回退和 cursor LastSeq/HaveSeq/FallbackSeq；文件身份、截断/替换、UTF-8/CRLF、物理 offset、扫描/记录预算及超长行后继续采集的用例保留。原 seq 回退测试转为 framing 不解释原始 seq 的反例。中心 ActivityWriter/镜像、跨 reopen 去重、尾行修复和按 run 隔离保留。
- 发布元数据：删除空 CreateDescription、PublicationSettings.Create、复制、固定校验与 TUI Create 行；删除退役发布责任常量。worker-only 与不支持 publisher 的反例保留；query/close、冻结 provider/Git/remote/auth 引用不变。当前快照 v6，重算有效摘要的 v5/v2 仍拒绝；严格 metadata 解码拒绝旧 create 字段。SQLite 12/runtime 17/PolicyResources v3 保留，资源绑定完整当前 profile 摘要，无迁移或回退。
- 夹具：更新全局示例、项目发现、catalog 隔离/错误优先级和 CLI 正例为 worker；local worker + command/Gitea 的支持矩阵同步，host_service 的反例保留。之前确认失败的 TestFiveGlobalLunaProfilesLoadAndFreeze、TestLoadProjectsSupportsZeroPartialAndExplicitConfigs 均实际执行通过。
- quick：原 normalizer/旧 stdout stream 选择替换为当前 TestConversationMapperNativeTwoTurnFixture、TestHistoryAdapterRejectsOversizedNativeCursor；原 AppServer 恢复回执选择保持。退役功能专属用例删除，不保留测试复制实现伪装生产覆盖。

## 验证

所有最终报告均 complete/pass，报告 SHA-256、命令及命中数见 [validation.json](validation.json)。完整日志留在 bin/retired-consumer-pruning/。

- 日志/会话/镜像：12 parent/subtest；新增原始 seq 不解释的边界反例独立通过。
- 固定发布/配置/CLI/TUI：6 包、34 parent/subtest，0 fail/skip。包括有效 v6、新旧字段与旧快照拒绝、原 provider 重启/remote drift、凭据隔离与当前例子/发现/CLI 正例。
- L1 quick：15 包、215 parent/subtest，所有组通过；新 owner 选择实际命中，现有 all tags/Windows/Linux/生产与测试依赖守卫未改。
- L3 controlled：delivery.basic 与 recovery.waiting-merge-restart 通过、cleanup_complete；worker 真实 Git/loopback PR 交付，v6 原身份重启、不重发及唯一 TaskDelivery 保持。
- 40 包编译、Windows/Linux amd64 CLI 构建与 diff --check 通过。无退役源码消费者或悬空 quick 选择。

未跑 Full、完整 L2 或 live 模型：变更为无消费者代码删除与派生快照字段清理，执行和原生权限协议不变；受控交付/恢复及原有实际消费者的确定性回归覆盖本次影响。没有创建或认证新执行平台。最初编译诊断的失败输出保留在 build-diagnostic，未作为通过证据。
