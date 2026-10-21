# 测试集执行记录与只读测试管理 Plan

日期：2026-10-21，审计修订。依据 [Spec](../spec/test-records.spec.md)，结果只记[专属验收](../acceptance/test-records.md)。普通开发，不纳入 roadmap；用户已授权按本计划实施；实际结果仅在专属验收登记。

## 1. 调查、整合与前置

调查基线/规范版本见 Spec 第 1 节，代码位置与取舍见第 2 节。复用原 Report/Definition、每入口 `managed-<id>.json`、真实测试反馈、EvidenceService、项目认证命令和 Query；不建立测试框架、修复工作流或上传会话。

已核对：`cmd/steerline/tests.go` 用 STEERLINE_HOME 发现应用，没有 -home 参数，且旧子命令共用必填 request-id；register 必须独立校验参数。项目和应用 HTTP 命令均有 1 MiB 限制，Runner 单 artifact 为 1 MiB；本次不能用“沿现有预算”掩盖整目录/大日志无法提交。Spec 已固定逐报告、小 summary、超限保留记录的做法。

Runner 所需公开 artifacts/Git 已具备，不依赖其新功能或真实模型。已读取 [worker 生命周期](../product-design/worker-lifecycle.md)，本次没有新增或修改节点/异常/验证矩阵规则（W/E/V 无差异）。原 worker/停止/恢复、系统生成及候选材料冻结沿最新主干的 [项目维护收敛](../spec/project-maintenance-simplification.spec.md)，不恢复人工 apply，也不新建并行生命周期矩阵。未发布版共同切换扩展后的报告字段，不兼容旧包、不回填历史。

## 2. 实施步骤

### 2.1 扩展原报告与纯复验判断

- 在现有 `internal/testmanagement` 报告 owner 扩展 Report：随机记录 ID、时间、定义快照、命令/分支、可选复验引用和证据清单。只按当前代码边界放置类型，不为移动文件而重分模块。
- `cmd/test-feedback` 在开始执行前冻结定义/选择/记录 ID/命令，结束后添加结果/后置 Git；原 managed.go 的结果判定继续承担真实断言，保留按 ID 输出和原 summary。所有生产者/消费者共同更新。
- selection/results 继续表示清单 ID；入口本来能报告稳定子项时才保留可选子项范围，用于部分展示，不新增通用场景注册表。只有整体结果的入口按整体处理。
- 项目记录和候选共享解析/规范化摘要/字节校验；历史复验只检查单份最新可比报告是否覆盖原失败，不跨报告/提交/分叉拼通过，不存一套可编辑“已修复”状态；历史结果/复验不复用候选 Project() 的 clean 准入，脏/未知源码如实标注，固定 SHA 证明仍归原候选投影。

### 2.2 项目记录存储与登记

- 在原 SQLite/store/ports 扩展项目记录表及必要测试集/时间索引，主身份为项目+报告 ID；保存不可变报告、声明证据摘要和附属证据/来源观察，入库序号用于分页上界。不给每个场景建表，也不复制候选状态。
- 每份报告一笔事务：校验报告、项目、已存在父记录同测试集、自引用/冲突、规范化摘要和已接收 summary；同内容幂等。引用新 ID 的既有父记录天然保持先后，不建立可编辑复验 DAG 或跨机时钟准入。
- 缺失/过大 summary 留不可用原因和原引用，记录可见；已上传字节摘要错误则拒绝。补齐只能匹配原声明摘要，事务更新附属观察，不改结果。提供 Register/Catalogue/List/Detail/Summary 窄用例。登记原请求对账使用 owner 的单记录回执读取，重新校验完整原输入及附带证据，不调用 UI Detail/历史投影；暂时存储失败保持未知，只有明确无效/身份冲突才判拒绝。
- 历史目录直接从已保存定义快照合并旧 ID；当前目录读取复用项目维护基线，失败不能拦住记录查询。

### 2.3 CLI、脚本与有界传输

- `cmd/steerline/tests.go` 增加独立 register 参数分支，`-project/-report-dir` 与原 STEERLINE_HOME 发现；不传生成/应用参数。复用既有应用客户端/项目命令/认证，发送记录内容而非服务器本机路径。
- CLI 逐报告读取原子发布的 `managed-*.json` 和已声明 summary：报告 128 KiB、summary 512 KiB 原始字节上限，完整编码信封至多 1 MiB。路径/链接/摘要沿既有 helpers；不提高通用预算、不读取未声明文件、不做分片。报告有效但 summary 不可用仍登记并显示诊断。
- 记录/证据解析、规范化、预算检查放业务 owner，CLI 仅执行本机文件读取与命令传送；Runner 用相同纯校验，不从 CLI 包导入实现。
- test-feedback 新增可选 `-record-project` 与 `-rerun-of 'test_id=record_id[,test_id=record_id]'`；结束时调用同一 register 用例。CLI 与 test-feedback 共用本地登记日志；registration.json 在发送前先保存未知状态，显式补登按尝试追加并保留原请求信息，控制台区分测试失败和登记失败。报告 ID 保持不变，补登记只重发原报告，原测试退出码不变；standalone register 有失败/未知项非零。
- 保留原 Full/build-only/Go 定向入口，未适配集合不报整体通过；产物仍用新 out/原忽略目录。同步 docs/testing.md 的扩展格式、例子、单集复验映射及补登记流程。

### 2.4 Runner 接线，候选读取保持

- EvidenceService 按原公开 artifacts/请求校验和持久原 Evidence；项目 tick 从已存证据登记同一份解析报告为项目记录。原 Evidence 行增加已投影 artifact 数量，成功登记后才推进；按 artifacts 追加数量大于已投影数量的条件有界读取，Git 刷新不重投影，不每轮全库重放，不新增队列/worker。
- 项目记录暂时写失败不能回滚/吞掉已保存的原候选证据；即使之后候选已放弃/交付、Runner 不在线，tick/重启仍能用本服务保存的原证据补登记。冲突明确诊断，不推进投影 artifact 数量。有界批次遍历失败后仍处理其他候选，以进程内轮转游标避免坏候选占满批次；同批报告先登记可用父记录，不依赖 artifact 枚举顺序，不新增持久队列。CLI 与 Runner 同报告去重，核验来源附加而不改原报告。
- PlanTests 保留原 EvidenceView/当前 Git 新鲜度/Run 绑定/固定材料门槛，只新增深链接。原系统生成/停止不重构，独立脚本记录不反向注入候选通过。

### 2.5 查询与 Web

- 在原 api/httpapi/httpcontract/applicationhttp/client/web server 只增加 Spec 的目录/记录/详情/summary 路由及登记命令；typed DTO，浏览器无写结果入口。
- 服务端组合过滤，列表走测试集/时间索引；沿现有 keyset 游标加记录入库上界，父子关系读取也带上界。证据观察变化提示刷新，不增加持久查询会话/快照库；游标绑定项目/筛选。
- Web 新增 tests feature 懒路由/侧栏，复用样式、Query keys 与项目隔离；目录/记录/定义、筛选/URL/空/坏状态、明确口径汇总、当次快照/单份复验及 summary 查看按原型落地。只查询必要页/详情，不全项目下载原始日志或遍历候选。
- 候选计划测试仅补记录跳转；来源定位严格匹配。离线/隐藏/重连/迟到响应沿原连接机制，不新增连接管理器。

### 2.6 最小真实闭环

- 在现有 e2e testenv/driver 中新增 L3 controlled `testing.records`，复用唯一 `scripts/new-debug-release.ps1` 创建 C:\SteerlineDebug 下的 SteerlineTest 独立包，AI 使用包的 `--simulate` 入口；方案生成时固定，不能启动后换。静态原型检查不启动产品现场。
- 包内准备一个小型测试项目的实际脚本/断言，测试目录只登记其真实入口；首次故意失败，修正夹具提交后再运行通过，经同一报告格式/正式 register 入库、API 查询及真实浏览器看到复验链和证据；再重放一次确认不重复。脚本以真实退出码/断言生成报告，不能由 driver 直接往库塞结果。
- 不为了这条场景运行 SteerLine 整套 L1 或递归执行 testing.records 自己；不直接伪造 passed/failed JSON。真实 test-feedback 导出由其 L1 owner 验证，闭环中的小型项目脚本验证独立项目生产者接线。报告形状来自同一契约，无额外生产框架。
- 部分范围、离线补登记、分叉、跨项目/实例、失败注入和候选跳转主要由定向 L1/组件测试证明，不在 L3 再展开完整矩阵。唯一闭环只证明脚本→CLI→DB→API→浏览器正常失败/修正通过/幂等。
- 同步 suites.json、E2E README、展示清单和 selector。记录新场景的独有覆盖及 home/repo/DB/端口/out/构建目录所有权，按原测试架构隔离和清理，不另写调试包生成器。

## 3. 测试计划

以下是实施和交付时使用的定向入口。所有选择须检查实际非零匹配，不把不存在或零匹配的命令记为通过。结果及新证据根统一登记[专属验收](../acceptance/test-records.md#实施结果登记)，每次使用新的输出目录。

每类用例只归一个职责前缀；下表不用多个 '^TestTR' 大范围反复执行同一套测试。L1 在对应既有 owner 维护，未自动被固定入口发现的用例才补 scoped.go，不重复聚合调用。

| 验收项 | 级别/组、环境与时点 | 命令 | 通过条件/证据 |
| --- | --- | --- | --- |
| [TR-A01](../acceptance/test-records.md#tr-a01) | L1 / tr.producer，临时 Git/out；2.1/2.3 后 | `go run ./cmd/test-feedback -out <证据>/producer -- ./cmd/test-feedback -run '^TestTRProducer\|^TestTMManaged\|^TestTMRegistered' -count=1` | 开始前冻结/运行中清单变化、原结果和退出码、部分/零匹配、record 参数/导出；summary/原报告/调用计数 |
| [TR-A02](../acceptance/test-records.md#tr-a02) | L1 / tr.register，临时 repo/DB/认证服务；2.2/2.3 后 | `go run ./cmd/test-feedback -out <证据>/register -- ./internal/testmanagement ./internal/storage/sqlite ./internal/app ./internal/product/httpapi ./internal/product/applicationclient ./cmd/steerline -run '^TestTRRegister' -count=1` | 无候选/认证/项目/路径、真实编码 1 MiB 边界、缺/大 summary 可查询、同摘要补齐/错误摘要拒绝、分条成功/幂等/冲突、离线/未知/原退出码、发送前日志与追加尝试、已提交未知回执只读对账/完整证据重验；信封/注册状态/事务证据 |
| [TR-A03](../acceptance/test-records.md#tr-a03) | L1 / tr.retest，纯报告；2.1/2.2 后 | `go run ./cmd/test-feedback -out <证据>/retest -- ./internal/testmanagement -run '^TestTRRetest' -count=1` | 单份完整通过、部分/分次/跨 SHA 不拼接、再失败/未完成/缺证据/不可比、脏/未知源码复验标注而不强制提交、分叉/无关联/迟到/跨机时钟；原报告和投影 |
| [TR-A04](../acceptance/test-records.md#tr-a04) | L1 / tr.runner，公开 Runner fixture；2.4 后 | `go run ./cmd/test-feedback -out <证据>/runner -- ./internal/testmanagement ./internal/app ./internal/storage/sqlite -run '^TestTRRunner\|^TestTMEvidence\|^TestTMPlan' -count=1` | 同报告不同顺序去重、索引失败保留原证据/未推进 artifact 数量，随后候选放弃/交付及 Runner 离线仍从已存证据补登，失败批次仍推进其他候选、轮转不饥饿、父子 artifact 乱序正确，成功后不全库重放；PlanTests 原归属/Git 门槛，调用计数/结果 |
| [TR-A05](../acceptance/test-records.md#tr-a05) | L1 / tr.query，隔离项目数据；2.5 后 | `go run ./cmd/test-feedback -out <证据>/query -- ./internal/testmanagement ./internal/storage/sqlite ./internal/app ./internal/product/httpapi ./web/server -run '^TestTRQuery' -count=1` | 目录坏仍可查历史、组合过滤/时区、游标项目/筛选/入库上界、链跨页、汇总和已存 summary/不可用、观察变化刷新提示；响应/零写结果副作用 |
| [TR-A06](../acceptance/test-records.md#tr-a06) | L1 / Web Vitest + 编译，接口 fixture；2.5 后 | `npm --prefix web test -- src/features/tests src/features/runs/plan-tests.test.tsx src/app/project-switch.test.tsx src/shared/api`；`npm --prefix web run build` | 只读导航/筛选/URL/摘要缺失/候选往返、离线/隐藏/迟到隔离、部分不冒充完整；组件报告/构建日志 |
| [TR-A07](../acceptance/test-records.md#tr-a07) | L3 / testing.records，controlled 新包/SteerlineTest/真实 Go-CLI-Web-Edge；定向 L1 后 | `go run ./cmd/test-feedback -level L3 -suite testing -scenario 'testing.records' -env controlled -out <证据>/flow` | 实际小脚本失败→提交修正→通过→原报告重放，CLI/DB/API/浏览器和清理；scenario.json/summary/CLI输出/截图 |
| 设计文档 | 静态文件，本轮和实施收尾 | `python scripts/check_doc_links.py`；`git diff --check`；按原型 README 检查受影响交互 | 文档/锚点/验收映射，原型无 JS 异常；只记设计证据 |

不运行 Full、完整 L1/L2、其他 L3/L4、真实模型/PR：本次只增加报告生产/消费和查询，单个受控闭环足够证明接线；只在授权的新建隔离现场执行产品验证。编译/原型不能替代上表真实断言。

## 4. 人工与交付

[TR-M01](../acceptance/test-records.md#tr-m01) 由用户在自动验证后安排体验窗口，只阻塞体验签收，不豁免自动失败。实现交付须 TR-A01–A07 有本次范围内的实际证据、相关原候选回归和文档同步；结果只更新验收，不把 Plan 写成运行状态表。
