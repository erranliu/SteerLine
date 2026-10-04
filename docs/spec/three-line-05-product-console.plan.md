# Plan：产品查询与控制台契约收窄

任务 ID：77；任务名称：产品查询与控制台契约收窄。唯一执行依据：[Spec](three-line-05-product-console.spec.md)。硬前置：#74、#38。一个独立实现 PR 交付。

## 实施步骤

开工实际基线为 main `621707d534b62e4b7df015463c53949715ba8be3`。#38/#39/#74/#75 已合入。协调已明确共享接线顺序：#76 先完成独立复审并真实合入；本任务先完成专属 DTO、注入端口、纯页面投影与控制台中性 fixture 准备，再基于协调提供的实际 main SHA 切换 ownership.md 第 1–6 项消费者。不能以接口草案或纯文档 PR217 代替执行 namespace 真实交付。

1. 按 #38 实际消费者列 DTO 字段及映射，移除产品聚合内部记录；小接口按查询/命令/连接拆，旧定义立即删除。

2. 迁纯投影及唯一 query/client，切换 app/CLI；console 迁包并完整保留 #38 行为与中性 fixture。

3. 扩展产品边界守卫及 DTO 编解码代表，更新 #38/#39 和已形成 #45 的路径/接线说明；不改变其产品验收范围。

实施中维护 `docs/validation/three-line-05-product-console/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下新包及 `TestTM*` 是实施时需要接通的精确目标，不表示本次文档 PR 已创建测试或入口。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/steerline/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM05.1、TM05.2、TM05.3 页面准备 | Windows；纯 DTO/投影，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/dto-preparation-01 -- ./internal/product/api ./internal/product/query -run '^TestTM05(ProductDTO\|PageProjection)$' -count=1` | 专属 DTO/纯投影完成后；共享消费者切换后另存新目录重验 | 两个父测试及其序列化/私有字段/Unicode/候选历史子测试非零通过；准备结果不替代真实产品接线验收 | `docs/validation/three-line-05-product-console/dto-preparation-01/` |
| 定向 Go：TM05.2 | Windows；#38 产品 fixture，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/console -- ./internal/console -run '^TestCC[124]$' -count=1` | 候选 UI 迁移后 | 既有 CC1/2/4 非零命中；身份/草稿/历史/停止事实保持 | `docs/validation/three-line-05-product-console/console/` |
| 定向 Go：TM05.1、TM05.3 | Windows；真实 app/client 和 SQLite 读快照 | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/product -- ./internal/product/api ./internal/product/client -run '^TestTM05(ProductDTO\|Snapshot\|OriginalRequest)$' -count=1` | 公开 DTO 和客户端切换后 | DTO 不含内部记录；同快照与原请求拒绝/对账代表实际通过 | `docs/validation/three-line-05-product-console/product/` |
| 定向 Go：TM05.1–TM05.3 | Windows；真实服务与受控后端 | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/wiring -- ./internal/steerlineintegration -run '^TestCandidateConsoleWiring$' -count=1` | 默认工厂迁移后 | 复用 #38 CC3/4 实际命令接线，无 UI 业务写权 | `docs/validation/three-line-05-product-console/wiring/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-05-product-console/l1` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-05-product-console/l1/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/L3/live：公开形状和路径调整由 fixture、读快照及真实应用接线验。#38 CC5 原输入证据复用；若输入/渲染实现变动，仅补受影响 Windows 手工代表并记录实际命令。

控制台迁移准备位于独立临时模块（不进入生产目录）；使用当前 api DTO 编译既有 CC1/2/4，最终须在真实 `internal/console` 和 app/CLI 构造切换后重新运行，不能作为 TM05 完成交付。原契约 Plan/digest 隐藏断言由 UI fixture 与产品 TaskDetail 源映射共同承接；fixture 不再造完整 TaskContract。唯一交付断言由明确的候选 `delivery_outcome` 承接，不再向 UI 传完整交付持久记录。决策历史的原 JSON 解码归 query，UI 只消费已类型化的问题、答案或未知格式诊断。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-05-product-console/README.md`，注明复用证据、定向重验依据及未具备能力。

#79 接产品读取需求，#80 接产品工厂；可与 #39 固定构建验收并行，默认构建换入本任务后定向重验改动的产品入口。
