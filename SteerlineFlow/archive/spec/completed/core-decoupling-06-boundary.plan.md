# Plan 主包隔离守卫与测试归位

任务 ID：67；任务名称：主包隔离守卫与测试归位；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-06-boundary.spec.md)及[共同约束](core-decoupling-constraints.md)。硬前置：#66 整体交付；一个实现 PR。

## 实施顺序

1. 核对 #66 生产零命中和依赖结果，清点尚留 root 的平台测试/fixture，按单元/跨层/中心规则归位。
2. 补外层结构守卫并纳入最小入口：无视构建标签扫描全部 AST，至少核对 Windows/Linux 生产依赖及额外 import 条件；验证守卫对禁用 import/关键词的拒绝，不建立 runtime denylist。
3. 接入外层中性第三实现，通过实际配置/执行/反馈/恢复端口验证扩展性（持久后重建服务、原身份对账、新人工确认及不重复输入），保留强制准入检查。
4. 执行旧到新命中核对、代表集成与 quick，删除无用 test helper，更新架构/配置索引与所有者记录。

## 所有者与删除

root 含平台名称的测试及 acceptance helpers、fixture、cmd/test-feedback 最小入口、所有实际生产目录/依赖和外层集成入口；生产搬迁必须已由 #62–#66 完成。

删除 root 的平台测试文件及遗留 acceptance helpers（由对应所有者承接，非删除覆盖）、无消费者测试工具和重复镜像矩阵。若仍有生产平台命中则回到 #66 范围修复，不把未完成生产搬迁包装为最终清理。

## 验证与命中核对

外层守卫小测试、第三实现闭环、两条已支持路径的一组代表恢复/提交屏障，以及 remote Protocol 与真实 Claim 拒绝。复用当前主包测试搬到实际包；在 quick 上加入守卫和必要集成选择，最多一组最小闭环，不跑 full。

实施时先记录所选旧用例的原位置、新位置及断言归属。下列 CD 用例名称是本任务交付的验收入口：可以把现有断言迁入同名用例或用带子用例的入口组合，不能另跑一份相同矩阵。每个命名入口及约定的各实现包必须实际命中，不能仅以进程 exit=0 或其他包有测试判断通过；原有生产缺口才补断言。外层集成包可由生命周期阶段按实际消费者创建，不得导出任意中心私有写入口。

```powershell
go run ./cmd/test-feedback -quick -- ./internal/steerlineintegration -count=1 -run '^(TestCD67CoreBoundary|TestCD67AdditionalImplementation)$'
go list -deps ./internal/steerline
git diff --check
```

最终验证将定向组与 quick 合在一次现有入口中；execution/contracts/query/sqlite/remote 协议包已由 quick-rules 运行，不再重复单独跑整包。CD63 的 remote codec 用例由 quick-rules 命中；必须核对按包/用例的实际 JSON 测试事件。调试失败可以定向重跑，稳定后不重复矩阵。

这些命令在本次规划中未执行。Spec 的各项验收对应通用/原生 fixture、固定身份与提交屏障、依赖和删除审查，实际命中数量与旧到新映射随实现记录。纯规则不跑平台矩阵，集成只跑受影响代表路径；quick 稳定后一次，没有具体失败不扩为全量。

go list 的当前平台输出仅作辅助；最终守卫还需解析所有构建条件的源码图并采样 Windows/Linux 生产依赖。最终守卫位于主包外，覆盖全部 OS 生产文件和直属测试；输出文件名/文本零命中及完整依赖检查，无例外名单。第三实现只证明扩展接线，不能作为原生能力认证。

## 验收映射与 PR 交付清单

| Spec 验收 | 入口/产物 | 必须核对的实际结果 |
| --- | --- | --- |
| CD67-1 | TestCD67CoreBoundary | 覆盖所有 OS 文件及直属测试，验证无关键词/import 例外，负向 fixture 能被守卫拒绝。 |
| CD67-2 | TestCD67AdditionalImplementation | 外层中性第三实现经受理、问题、反馈、结算及恢复端口走同一业务事务；根包生产无需增加实现分支。 |
| CD67-3 | 旧到新映射与 quick | 逐条核对原生/跨层/中性断言位置及命中；quick 实际执行守卫、必要代表原生/remote拒绝用例，无重复矩阵。 |

PR 附本阶段消费者/所有者和删除清单、版本变化、旧到新测试路径映射及实际命中；未运行的原生证据单独列限制。下游只能接收已实际接通的契约，不以空包、双路或仅声明接口替代交付。

## 交接

交付主包生产/测试清零、实际依赖方向、旧到新测试映射和最小命令证据；仍缺 remote 原生权限证据以支持范围限制记录，不影响本任务代码隔离判定。

格式改变明确更新版本并拒绝旧开发格式；不加迁移/双写/fallback。保留原请求、资源身份、深复制及单写者事务。PR 和验证记录使用实际行为与支持限制，不能把结构迁移算作 remote 权限证据补齐。
