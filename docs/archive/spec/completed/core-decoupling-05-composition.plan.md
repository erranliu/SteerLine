# Plan 内置实现组装与主包依赖反转

任务 ID：66；任务名称：内置实现组装与主包依赖反转；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-05-composition.spec.md)及[共同约束](core-decoupling-constraints.md)。硬前置：#65 整体交付；一个实现 PR。

## 实施顺序

1. 列出所有产品构造入口，约定通用目录和显式注入构造的签名，确认不存在向实现暴露 Service 的依赖环。
2. 外层构造现有三个内置实现的目录/codec/工厂；用实际产品入口消费，移走 root 具体配置受理、默认和组合。
3. 串行更新 CLI/console/命令/恢复/查询/测试工具；会因构造改变失效的测试本轮改成中性依赖或移入外层，root 同包测试不导入 steerlineapp，移除旧默认构造和 package init 注册，保持产品功能闭环；离线读取不要求 target 在线或启动能力，仍拒绝新的变更。控制台不导入 steerlineapp，由外层注入公共来源以避免依赖环。
4. 清除 root 剩余生产平台命名/字面量/导入和无用 shim，检查直接/传递依赖与目录实例隔离，运行最小外层集成验证。

## 所有者与删除

`execution_registry.go`、Config/Profile 工厂与静态判断、backend/capability/snapshot 组装、NewService/Claim/Query/console 构造和 cmd/steerline/cmd/test-feedback 入口。承接 #62–#65 通用端口。

移除 root 内置注册、具体工厂、配置平台 key 字面量、权限/版本/能力组装和所有平台 import；已阶段迁移的空 shim/别名删除。root 生产文件名及源码 codex/orca 命中在本轮达到零，不能把生产清零遗留给 #67。既有 CLI 调用都迁到唯一外层入口。

## 验证与命中核对

实际 cmd 产品构建和离线只读查询；目录 pure 回归覆盖重复/未知 key、版本拒绝、缺端口、两个目录互不污染。中心中性 fixture 加一组外层服务/Claim/固定快照恢复/查询接线，复用 quick 的 Claim 到唯一交付闭环，不建立另一份中心状态机测试。

实施时先记录所选旧用例的原位置、新位置及断言归属。下列 CD 用例名称是本任务交付的验收入口：可以把现有断言迁入同名用例或用带子用例的入口组合，不能另跑一份相同矩阵。每个命名入口及约定的各实现包必须实际命中，不能仅以进程 exit=0 或其他包有测试判断通过；原有生产缺口才补断言。外层集成包可由生命周期阶段按实际消费者创建，不得导出任意中心私有写入口。

```powershell
go build ./cmd/steerline ./cmd/test-feedback
go run ./cmd/test-feedback -quick -- -p 1 -parallel 1 ./internal/steerline ./internal/steerlineapp ./internal/steerlineintegration -count=1 -run '^(TestCD66ExplicitCatalogAndEntrypoints|TestCD66FixedProfileRecovery)$'
go list -deps ./internal/steerline
git diff --check
```

最终验证将定向组与 quick 合在一次现有入口中；execution/contracts/query/sqlite/remote 协议包已由 quick-rules 运行，不再重复单独跑整包。CD63 的 remote codec 用例由 quick-rules 命中；必须核对按包/用例的实际 JSON 测试事件。调试失败可以定向重跑，稳定后不重复矩阵。

这些命令在本次规划中未执行。Spec 的各项验收对应通用/原生 fixture、固定身份与提交屏障、依赖和删除审查，实际命中数量与旧到新映射随实现记录。纯规则不跑平台矩阵，集成只跑受影响代表路径；quick 稳定后一次，没有具体失败不扩为全量。

## 验收映射与 PR 交付清单

| Spec 验收 | 入口/产物 | 必须核对的实际结果 |
| --- | --- | --- |
| CD66-1 | TestCD66ExplicitCatalogAndEntrypoints | 缺目录/key/版本、重复 key 和缺能力在目标变更前拒绝；离线持久查询仍可用，两目录实例不共享配置或全局注册。 |
| CD66-2 | 构建及入口清单 | cmd、服务、Claim/任务命令、查询、控制台和测试工具均使用同一外层构造链，旧默认构造无消费者并删除。 |
| CD66-3 | TestCD66FixedProfileRecovery | 代表执行通过外层保存/恢复原 key/codec/目标；旧配置改变不替换原会话，生产根包扫描与传递依赖清零。 |

PR 附本阶段消费者/所有者和删除清单、版本变化、旧到新测试路径映射及实际命中；未运行的原生证据单独列限制。下游只能接收已实际接通的契约，不以空包、双路或仅声明接口替代交付。

## 交接

2026-10-02 实际验收：clean `755885d52f23c1bb98c4b8c4e7e9cdaf6b97454b` 在完全访问环境命中 core 6、rules 90、targeted 22，全部通过、无跳过。新增中心命名入口复用资源候选断言，并验证丢失准备记录及启动缺目录的写入前拒绝；未替换或豁免原组合。复审问题、失败诊断及最终报告见[验收记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/core-decoupling-05-composition/README.md)。

#67 获得生产清零及可独立构造应用的目录、工厂和通用端口，最终只验守卫与整理残留测试。

格式改变明确更新版本并拒绝旧开发格式；不加迁移/双写/fallback。保留原请求、资源身份、深复制及单写者事务。PR 和验证记录使用实际行为与支持限制，不能把结构迁移算作 remote 权限证据补齐。

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../acceptance/history.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
