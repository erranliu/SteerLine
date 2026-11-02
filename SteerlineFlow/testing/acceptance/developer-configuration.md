# 全局开发者配置验收

2026-11-02，普通开发；用户已确认交互和技术方案，后续授权独立子代理审核、修复并提交 PR。本次不调度 roadmap、不部署安装版、不运行真实模型。

## 成果与边界

- 配置页新增独立开发者分类，当前开关为「环境未就绪时阻止执行」，默认开启；所有项目共用当前 home 的 `execution.json`。
- `internal/developerconfig` 集中维护明确布尔配置、默认值、构建能力和不可变策略；应用服务及 SQLite writer 显式注入，项目配置与客户端请求不授予免检资格。
- 跳过仅影响项目环境资格：手动启动、新增候选、自动派发及最终预留一致。真实状态继续显示；目标占用、Runner/方案可用性、任务依赖、版本与名额继续检查。
- writer 原子保存 `environment_readiness_skipped` 事实，后续更新保持不可变；原请求重放读取原事实，重新开启阻塞后仅新预留恢复检查。不新增数据表、迁移或旧版本读取分支。
- 保存沿用摘要冲突检测及原子替换；重启生效。保存与重启之间沿用原配置变更保护，暂停新受理。界面不把草稿显示为已生效，配置页统一汇总两个表单的离页保护。未编辑的表单使用共享的最新快照，编辑时才冻结原摘要；连续保存不同分类不会误冲突，原草稿仍按原摘要校验。
- `build.ps1 -Web -Release` 同时给两个产品 EXE 添加 `release` 标签；后端拒绝非默认开发覆盖，Web 隐藏整类入口。当前普通开发构建仍提供开发者配置。
- 本次不改固定提示词、材料、Runner 输入正文或提示词组装。用法及发布移除方式见[开发者配置说明](../../../docs/integration/developer-config.md)。

## 自动验证安排

| 项目 | 通过条件与入口 |
| --- | --- |
| DVC-A01 配置与加载 | 默认门禁开启；开发覆盖从全局文件加载；原快照不热变；重新加载恢复默认；`TestBuildPolicy`、`TestDeveloperConfigurationLoad`。 |
| DVC-A02 全局条件保存 | 新 reader 读取同一全局值；保留执行方案；旧摘要写入拒绝；正常恢复开关；`TestDeveloperConfiguration`。 |
| DVC-A03 服务入口 | 手动与自动达到材料准备、新候选达到预留；执行方案不可用和目标适配占用仍拒绝；真实环境不伪造 ready；`TestDeveloperReadinessService`。 |
| DVC-A04 事务及重放 | 手动/自动/额外候选按 writer 策略放行；候选自称跳过不放行；版本仍拒绝；重启后新候选恢复门禁、原请求保留原事实；`TestDeveloperReadinessReservation`、`go.storage`。 |
| DVC-A05 配置交互 | 默认开关、保留其他配置、冲突留稿、取消、响应丢失双向核实、只读、待重启及发布隐藏；同页双向连续保存使用新摘要，并行草稿保留原摘要，取消后使用最新已保存值；`web.unit`。 |
| DVC-A06 浏览器 | 零项目配置、键盘开关、两个表单各自未保存保护、保存/重启显示、1440/375 宽度不溢出、发布隐藏；`configuration-navigation.spec.ts`，受控 API，不当作原生后端验收。 |
| DVC-A07 发布能力 | `release` 标签下重复开发配置与事务验证；编辑文件不能启用；API validate/save 拒绝；双产品真实发布构建及 PS5.1 构建参数夹具。 |
| 回归 | 固定 L1/L2；`go.http`、`go.storage`、`go.environment`、`web.unit`；L3 `testing.project-environment-readiness` 保持默认门禁完整流程。 |

## 初次实现结果（7d85a9e 基线）

基线为 `7d85a9e908102a4f12210bbba91408f39362e6a7`，受测内容为本聊天未提交工作区；各原报告保留实际 worktree 状态，不宣称干净提交验收。所有持续证据位于 `C:\SteerlineDebug\wt-2c36-developer-config\`。

| 验证 | 实际结果与证据（相对上述根） |
| --- | --- |
| 默认定向 | 通过；`diagnostics/20261102T063808352Z-52e69942e6694a6a8d6790064353b404/summary.json`。 |
| HTTP 全集 | 通过；`test-results/L4/20261102T064132221Z-17eb3407275545169e49dd22bcaa00a7/summary.json`。 |
| Web 全集 | 最终通过；`test-results/L4/20261102T071542844Z-1c4a0a950bf14615be2abd4007913819/summary.json`，包括同页双向保存与并行草稿保护。 |
| Edge 浏览器 | 最终 3/3 通过；`diagnostics/20261102T071910292Z-2856283aa8cb421cac62d64232681faf/` 保存 trace 和宽窄屏截图；前轮截图也已检查。 |
| 构建连接夹具 | 22 项通过；`diagnostics/20261102T064244177Z-fef1d3cf96714f78b00e8f61639f0148/result.json`。 |
| L2 | 通过；`test-results/L2/20261102T064333974Z-96fa002a13de4ed58e362581ba3d3477/summary.json`；后续仅补历史开发免检重放的存储分支，默认资格路径不变，最终存储及 L1 单独复验。 |
| 最终 L1 | 通过；`test-results/L1/20261102T071437067Z-7f0b4bc637af481d84ad621618cb72cb/summary.json`。 |
| 最终存储全集 | 通过；`test-results/L4/20261102T064550502Z-af32f824975f46f6b3d62dc4effefb5d/summary.json`，包含历史免检预留重放和恢复门禁反例。 |
| 最终发布定向 | 通过；`diagnostics/20261102T064708618Z-07d4cf0d5bbb48d1ac2d47769f405b65/summary.json`；包含发布启动拒绝非默认配置、API validate/save 拒绝及不可伪造的事务策略。 |
| 最终实际发布构建 | 两个产品 EXE 构建通过；`diagnostics/20261102T071740829Z-9033eb72254049aea4310492e7b0020e/release-build.json`，同目录 `release-executables/` 保留 EXE。未部署或启动产物。 |
| 最终实际开发构建 | 两个产品 EXE 构建通过；`diagnostics/20261102T071753154Z-3f77e49a57bf46c1b0dd50feebf9f9f6/development-build.json`；源码 `bin/` 为开发构建。 |
| L3 默认门禁全流程 | 通过；`test-results/L3/20261102T064548407Z-2c992cb8c1834058911db30b98a3066c/summary.json`。 |
| 环境 owner 全集 | 两轮均 failed/incomplete。最终轮 `test-results/L4/20261102T070734663Z-0f5537beef5045949650a5186270ddf9/summary.json`：0 断言失败、0 skip，其他 3 包通过，app 达到 25 分钟包上限，96/118 个主测试完整通过，余 22 个未完成。不能记为完整 `go.environment` 通过。 |
| 环境 owner 剩余补验 | 22 个主测试全部通过，49 个测试终态、0 失败、0 skip；`diagnostics/20261102T073408235Z-20b6f41023064a93b310ebd8c2b780df/summary.json`，实际选择另存 `supplemented-app-tests.json`。最终正式轮与该诊断的并集完整覆盖本机可执行的 118 个 app 主测试，缺失 0；核对见同目录 `app-root-coverage.json`。这不改变原注册全集的失败状态。 |
| 两项首轮失败的定向验证 | 边界修正与原安装夹具均通过；`diagnostics/20261102T065629159Z-81e119cdc27943beb702a2642884cdab/summary.json`。安装夹具另 10 轮通过；`diagnostics/20261102T065747974Z-ccf7026d8b3c4b3c81c278734c100ff8/summary.json`。 |

早期 `diagnostics/20261102T063618788Z-ac4bfa1b070241fda3d34bfa404dbebc/summary.json` 记录了新增纯策略包尚未加入存储依赖允许清单的失败；已将该包纳入纯值检查，并仅允许配置/Web/存储消费者引用此包，不扩大应用或执行依赖。最终 L1 复验该边界。最初 sandbox 内 Git 初始化和私有文件替换因权限失败，随后使用正常隔离进程执行；不将这些失败当产品通过证据。

首次 L1 和存储回归分别保留于 `test-results/L1/20261102T063947013Z-e09e3596e4a740068f1bf9a343fa49b5/` 与 `test-results/L4/20261102T063945122Z-28465b1fb03640688b8a7fad424a5ad2/`。后续未保存保护和历史重放修正由最终轮覆盖，不用前轮代替。

环境 owner 首轮 `test-results/L4/20261102T064133246Z-2e970401503d4e4e84f67c9eeb5350d1/` 的两项失败为 `TestHTTPConsumerBoundary` 漏登记新纯配置包，以及未改动的 `TestInstallExecutableCopiesAndOverwrites` 首次复制报 `Access is denied`。前者只补充新纯值依赖的明确名单；后者隔离复测及 10 轮通过，未确认权限错误根因，不放宽保护或增加重试。随后原包在 `TestTMGenerationCancelledMissingSubmissionRequiresOfflineProof/accepted-original` 执行期间到达 25 分钟包预算，后续用例未完整执行；本轮按 failed/incomplete 保留。最终轮未与本聊天另一后端 E2E 同时执行，预算和断言不变；只读进程清单显示机器上另有其他工作区的 Go owner/L2 并跑，不能声称机器负载独占。不能以定向通过宣称首轮全集通过。

同页快照补强后的首次构建在新测试的 `getByRole` 选项中发现不受支持的 `exact` 属性；保留 `diagnostics/20261102T071653231Z-56380f51dbbc4ad1b1984a25b579d996/release-build.json` 和原日志。移除此无效选项后实际类型检查、发布及开发构建通过，见上表；没有修改查询的断言内容。

最终未增加包超时、未跳过原用例：先用 `go test ./internal/app -list '^Test'` 枚举本机正常标签下的 118 个测试根（仅清单，不计测试通过），清单保留在 `diagnostics/20261102T072256923Z-f3c467423799413490a9b35844facc99/app-test-inventory.txt`。从正式轮 JSONL 的完整 pass 事件计算差集，再通过原 test-feedback 定向入口完整执行其 22 个父测试及全部子例。最终覆盖核对为 96 + 22 = 118、缺失 0；包级单次预算内的完整成功仍未取得。补验 259.020 秒；注册集合的负结果与补验正结果分别保留。

文档链接检查通过（647 文件、4042 本地链接、0 错误），执行清单/场景登记一致性两项通过，`git diff --check` 通过。

本阶段未验范围：真实模型/远端环境执行、实际安装部署及人工体验签收；初次实现未取得完整环境 owner 的单次包预算内成功。本轮使用正式应用/SQLite 与公开 Runner fixture，浏览器部分单列受控页面证据。

## 独立审核、修复与 main 整合

用户授权后由两个全新只读子代理分别审核后端，以及 UI/发布构建。后端初审无确认缺陷；UI 审核确认两项 P2：

1. 保存成功后晚到的旧 GET 会覆盖共享缓存，导致值回退或再次使用旧摘要。两个表单现在先取消同 key 的在途读取，再发布已保存快照；双向晚到读取回归均通过。
2. 生效状态使用了冻结草稿的摘要，导致另一分类保存/重启后漏报或误报待重启。冻结快照现在只保护草稿与条件写入；保存/生效展示使用最新共享快照，并分别提示未保存草稿和全局待重启。双向兄弟保存/重启回归均通过。

提交前 fast-forward 到 main `6eb03d21`，保留其环境配置轻量读取、资格读取优化及开发权限精简。三处冲突分别合并验收导航、后端 `environmentOptions` 和前端双查询 hook：策略由同一个 `optionsFor()` 提供给配置和资格响应；前端优先使用已确认配置的策略，资格读取挂起不阻断显式开发覆盖，未知或离线缓存不授权覆盖。新增独立 hook 五项回归及无存储配置投影测试。

两名子代理完成修复后的独立复审：后端未发现需修正问题，核对正常权限的 4 包 10 项定向报告通过；UI/发布复审未发现遗留问题，独立执行 3 文件 28 项通过。两轮均未修改代码、未冒称原超时全集通过。固定提示词、规则材料与组装未修改。

本轮证据仍相对 `C:\SteerlineDebug\wt-2c36-developer-config\`；各原报告记录真实源码状态，下面结果不覆盖前述历史失败：

| 验证 | 结果与原报告 |
| --- | --- |
| 正常策略及配置投影 | 4 包 10 项通过；`diagnostics/20261102T082153225Z-b993ac912a3341e2a9af4f6665c29ad5/summary.json`。 |
| release 策略、加载/API/预留 | 通过；`diagnostics/20261102T082947658Z-229a59f7a9274b39a645546c0e7e0179/summary.json`。 |
| 固定 L1 | 通过；`test-results/L1/20261102T082946782Z-326a06bc512b484c9075eaebcb9c598c/summary.json`。 |
| 固定 L2 | 通过；`test-results/L2/20261102T082949113Z-2a7133bb873c4c1fa502e0e1117d94e8/summary.json`。 |
| Web 全集 | 通过；`test-results/L4/20261102T083926651Z-a7f8ef05d2eb4ca2a834234c31e81b13/summary.json`。 |
| HTTP 全集 | 通过；`test-results/L4/20261102T083922778Z-9d2ff0dfddf74f158d9341c41688254d/summary.json`。 |
| release 双产品构建 | 通过；`diagnostics/20261102T084328677Z-80438662dfdd4cb9b5054a8a0df2db1f/release-build.json`，同目录保留 EXE。 |
| 开发双产品构建 | 通过；`diagnostics/20261102T084349619Z-195c6bed2f2b4ae69ad4284eb5de9f56/development-build.json`。源码 bin 保持开发构建。 |
| Edge 浏览器 | 3/3 通过；`diagnostics/20261102T084451989Z-8fbfe740d9d2473aa47221de9c7dab2c/`，保留 trace 与宽窄屏截图。 |
| 环境 owner 全集 | 在 `6eb03d21` 基线上的本功能修复树完整通过：4 包 338 终态、0 失败、0 skip，complete=true/passed=true，1442.429 秒；`test-results/L4/20261102T082642476Z-9ef09210f2f74d7a8f202e3b07586ae1/summary.json`。前述初次实现的失败与补验不改写。 |

功能与审核修复已提交为 `53ab80f214cc92a4acf46dc9038b798c41e7388f`，创建 [PR #432](https://github.com/erranliu/SteerLine/pull/432)。PR 创建期间 main 又前进到 `82935861`（环境资格与项目测试解耦）；以 `6860d53a` 保留双方历史自动合并，无冲突，未手工更改本功能代码。两名子代理再次只读复核：策略加载、双响应字段、占用保护、预留/重放、两项 UI 修复及三项浏览器场景均未被合并覆盖，无新增发现。

合并后的验证单独登记，不将上表旧树全集结果称作新树验证：

- 固定 L1 通过：`test-results/L1/20261102T085322745Z-8c8ab1118c6e454aac7940c743e9c9b8/summary.json`。
- 开发开关与上游环境资格不依赖项目测试的定向验证通过：`diagnostics/20261102T085321519Z-77d9811ac3b54f909eed050b991e6370/summary.json`。
- 上游批量读取精确字节/进程数及原始失败原因两项通过：`diagnostics/20261102T085423050Z-d94e11e054e04773946dd2623807c35d/summary.json`。
- 固定 L2 通过：`test-results/L2/20261102T085322937Z-9d4436da1612415199146fc2642ede55/summary.json`；受控场景及清理均完成。
