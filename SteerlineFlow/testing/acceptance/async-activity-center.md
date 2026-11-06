# 异步活动中心专属验收

2026-11-06；按用户后续授权完成首期实现及独立代码审核，验证结果如下；未部署。普通开发，不分配 roadmap Task ID。人工体验后置，未因自动回归通过填为人工验收完成。

依据：[Spec](../../spec/async-activity-center.spec.md)、[Plan](../../plans/async-activity-center.plan.md)。实现基线 `a54280510802c1208d9a1a3ff0ec473cad4ebd24` 加本工作区未提交变更；schema 39 → 40，无旧格式迁移。受控公开 Runner fixture 与实际 Go/Web 二进制摘要、源码状态和调用参数由每次 summary/builds 固定，未使用真实模型或真实 PR。

## 自动验收

下表保留验收目标，实际覆盖与报告在后文逐项登记，不把一条专项通过泛化为所有边界均实测。`runtime.async-activities` 与 Edge 驱动已实现并登记 L3/controlled。

| ID | 方法与最小操作 | 最小通过条件 | 触发 / 阻塞范围 |
| --- | --- | --- | --- |
| AAC-A01 | 自动；从真实 SQLite 登记四来源；覆盖本地等待首次启动项→journal→正式候选、重复请求、正式记录位于第二页/其他筛选、旧实例未匹配 applied/unknown 请求 | 四来源可发现；本地尚无 journal 也可见，不产生新提交；各阶段沿原 ID 只显示一次；同批次复用与并行候选正确；关联故障不伪造未登记；旧实例占位不复活；跨 home/项目/实例不串数据 | 来源及前端接线后；阻塞首期交付 |
| AAC-A02 | 自动；参数化现存/未知状态，构造请求 applied、generated 未 ready、待合并、winner/未选中、历史 pending question、恢复、批次 failed 但关联生成运行/未知/未 Settled、部分失败/重试、零项与测试重跑 | 业务结果/活动结束/TaskDelivery/资源结算分开；批次先 waiting 后随原关联结算进入终态，业务失败摘要不被子生成成功覆盖；提醒按来源精确相关性，部分重试不清无关失败；false 保留未完成含义 | 映射实现后；阻塞首期交付 |
| AAC-A03 | 自动；关闭 UI 运行三类流程；重复查询和故障注入；统计 Git/Runner/提交/恢复/停止及业务写入调用 | 无 UI 仍推进；活动 GET 零外部取证/业务写入；查询故障不新建或重发工作，不改业务期限、权限或 actor 执行路径 | owner/API 接线后；阻塞解耦边界交付 |
| AAC-A04 | 自动；浏览器刷新、同构建停启服务、SQLite 重开、部分生成提交后准备失败并由新批次替换当前引用、命令未知、旧实例迟到响应 | 原活动与生成关联可重建；旧批次仍随原 GenerationService 事实完成结算投影；业务失败快照独立保留；没有活动专属重跑/停止/替代执行；缺时间不伪造 finished_at；未知原检查不重放；迟到响应不覆盖当前范围；登记事务失败不留半份事实 | 持久化接线后及跨模块验证；阻塞首期交付 |
| AAC-A05 | 自动；单来源超时/坏记录、整体断连、缺观察元数据、成功读到不变 revision、测试进度故障；构造超过两页活动 | 故障和过期不改业务结果；保留最后有效信息；采样时间不冒充上游心跳；未加载页不删记录；稳定集翻页不漏不重，计数完整或明确未知 | API/前端接线后；阻塞首期交付 |
| AAC-A06 | 自动；按下述交互矩阵验证顶部入口＋右侧抽屉，组件和真实受控浏览器分别覆盖，核对原页面入口 | 符合 Spec 第 6.2–6.4 节；零活动入口、未知计数、桌面/窄屏、筛选/导航/焦点/更新均准确；关闭不取消工作；历史批次不冒充当前批次；原检测/准备/执行页面和原请求行为保持 | 前端接线后；阻塞首期交付 |
| AAC-A07 | 自动；新受控场景与 Plan 必要回归，检查实际报告及清理 | 三类工作、重启、部分故障和页面关闭有跨模块证据；L1/L2、受影响正式专项通过；零实际 skip/零匹配，原资源按规则结算 | 完整实现后；阻塞自动验收完成 |

### AAC-A06 界面与交互覆盖矩阵

以下是 AAC-A06 的自动子场景，不另增 Task 或验收通过声明；组件覆盖显示/事件，真实浏览器覆盖布局、焦点与路由。执行时每行保存实际断言及适用截图/日志。

| 子场景 | 操作与通过条件 |
| --- | --- |
| 入口与范围 | 0 项、有进行中、有需处理、未知/部分计数逐一核对；入口在项目顶部右侧且与连接状态共存；项目内跨路由保留，无项目/全局设置不误显示；真实切项目/换实例造成 Workspace 重挂载后抽屉仍开、旧数据/游标/本地项丢弃并重新读取，无旧名称/旧条目闪现；离开项目或换 home 关闭 |
| 桌面开关 | 顶部按钮、关闭按钮、Escape 可操作；右侧覆盖不改变原业务布局；点击未覆盖原页面可交互且不关闭抽屉；关闭不改变后台工作 |
| 窄屏与缩放 | 320px、断点两侧及 200% 缩放；窄屏全宽覆盖/遮罩、背景滚动锁与模态焦点正确；关闭/筛选/详情可达，无遮挡按钮或整页横向滚动；断点变化保留数据/筛选/有效焦点 |
| 筛选与列表 | 首次默认进行中、重开恢复该项目筛选；三种空态/首次读取/失败保留分别正确；排序/翻页/长文本/历史摘要可用，普通刷新及加载更多不抢滚动位置 |
| 状态更新 | 新活动、失败、需处理及完成不会自动打开/关闭抽屉、跳筛选或抢焦点；无逐轮 toast/逐秒 aria-live；观察断连保留上次卡片和确认时间，不显示假实时转圈 |
| 导航与处理 | 查看详情/去处理仅导航并关闭，正确定位候选/检测；导航保护取消时抽屉保留；历史批次只读展开不指向当前批次；没有抽屉内业务控制按钮 |
| 键盘与焦点 | 入口/关闭/筛选/加载更多键盘可达；桌面非模态可进入原页面，窄屏焦点限于抽屉；Escape/关闭返回入口，详情跳转移交目标；状态有文字，不单靠颜色，减少动态效果偏好生效 |

## 人工验收

| ID | 参与与操作 | 通过条件 | 安排 / 状态 / 阻塞范围 |
| --- | --- | --- | --- |
| AAC-M01 | 用户或指定产品体验参与者；从顶部入口同时观察检测、准备和两个候选，桌面边看原页面边查看右侧进度，辨认等待原因、需处理与最近结束，进入原详情；补窄屏体验 | 能清楚区分启动待确认、实际推进、等待用户/外部和结束结果；顶部提醒易发现且不打扰，抽屉信息密度可接受，原工作入口易找 | 参与人及窗口待安排；待人工验收；自动验收完成后集中后置，不阻塞无关工作，不替代自动功能缺失 |

## 环境、证据与结果登记

- 自动单元/集成为 isolated，跨模块为 controlled，使用真实 SteerLine/Git/SQLite/HTTP 和公开 Runner fixture；浏览器使用隔离 Edge/构建资产。不证明真实模型、Orca 或真实 PR 链。
- 原始输出按[路径清单](../../paths.md)放入 `C:\SteerlineDebug\<scope-id>\test-results\<级别>\<test-run-id>`；定向诊断放 diagnostics 独占目录。每次使用新目录，不搬用其他工作现场或安装数据。
- 实施时逐项追加实际源码 commit、固定构建摘要、完整命令/选择、日期、结果、summary/原日志绝对位置、外部调用计数和资源清理证据。失败与补验分开保留，尚未执行不能填已验收。
- 实施结果见下列报告与覆盖说明；原始失败不删除、不覆盖，报告中的清理不完整项保持不完整。

## 2026-11-06 独立文档审核

用户明确要求独立子代理审核。本轮只读审核者为 `audit_async_activity_docs`，核对 Spec/Plan/验收规划与当前源码；主聊天核实并修正文档，没有实施功能或运行产品测试。以下是文档缺陷修正，不计为自动功能验收通过。

| 问题 | 核实依据 | 文档修正与后续验证 |
| --- | --- | --- |
| P1：批次 failed 可能早于子生成结算 | `internal/app/maintenance_worker.go` 的 failPreparation / 非当前批次处理；`internal/testmanagement/model/records.go` 的 Settled | Spec 区分业务结果、关联生成/资源结算、活动结束，Plan 保留部分提交失败的关联并只读汇总历史批次；AAC-A02/A04/A07 加反例 |
| P2：仅 journal 会漏本地等待首次启动候选 | `web/src/app/use-candidate-starts.ts` 的 LocalStart.first / items | 只读合并现有本地项、journal、正式 ID；明确本地未发送项不保证刷新恢复；AAC-A01 加阶段去重 |
| P2：Workspace 重挂载丢失抽屉状态 | `web/src/app/app.tsx` 的 home/project/instance key | UI 打开/筛选偏好放在 keyed Workspace 外，查询留在内；AAC-A06 实测 remount |
| P2：旧实例 journal 可能重新生成当前占位 | `web/src/shared/api/journal.ts` 保留历史；`web/src/app/use-candidate-starts.ts` 按 instance 筛选 | 临时项按 home/项目/实例限定，旧未匹配请求留在原回执历史；AAC-A01/A04 覆盖 |
| P2：需处理提醒缺来源相关性/退出依据 | `web/src/features/runs/state.ts` 的 currentCandidate；批次/命令无通用 resolved 关系 | 按来源当前引用与精确关联计算提醒，历史结果保持，候选先检查参与/交付/停止边界；AAC-A02 加部分重试及历史问题反例 |

修订后独立复核：2026-11-06 由同一只读子代理重新核对修订稿，确认上述五项均已实质关闭，Spec、Plan 与验收规划对应一致，未发现修订引入的新可确认问题。功能验收仍全部待验收，本次未实现或运行产品测试。

## 2026-11-06 实施与独立代码审核

首期已接入检测（环境检测、显式主分支检查）、测试准备批次和任务候选，增加顶部入口与右侧抽屉；原页面、业务操作与原请求处理保留，只对环境历史检测增加最小只读定位。活动 GET 不持有 actor、Runner/Git 客户端或写入能力。批次原关联在初次生成登记事务中持久保存，当前引用与历史独立；观察元数据不变成业务版本或执行权限。长期设计见[异步活动中心](../../../docs/product-design/async-activities.md)。

独立只读审核者 `review_activity_implementation` 首轮确认三项问题，主聊天修复并补回归，同一审核者复核均实质关闭，未发现新增可确认问题：

| 问题 | 修正与回归 |
| --- | --- |
| 已停止/退出候选含未知或已接受未终结操作，SQL 粗筛漏掉进行中条目 | 粗筛包含未知副作用，真实 SQLite 验证 unknown/accepted → completed 的列表、计数和历史一致 |
| 本地准备 worker 时间掩盖关联生成的外部观察失败 | SQL 读取各 Generation 的独立元数据；未结算子项取最旧成功观察、缺失 unverified、已知错误 unavailable；历史重开及多子项回归，超时诊断有界保存 |
| 新 journal 请求改变无限分页 key，丢失已加载页 | 列表和请求关联独立 query；加载 100 条后追加 journal 不重置分页，不提交业务动作 |

最终增量审核确认：受控 Runner seed 缺失的 recovery/run_observation 元数据补齐只作用于测试 peer，不放宽生产门禁；调试模板补当前 development/2 契约要求的显式 full_access 权限，不改提示词；删除未使用 Associations 字段，SQL 关联仍为唯一事实。提示词正文/组装/提交接线无差异，UTF-8 正文字节变化 Δ0，未更新黄金样本。新增导航保护取消/接受、超时元数据回归通过。

### 实际报告

所有报告根为 `C:\SteerlineDebug\wt1369-activities\test-results`，本节目录均在该根下，`summary.json` 为实际选择、终态、源码和构建证据。命令前缀均为 `go run ./cmd/test-feedback`；正式专项采用 Plan 中级别/suite/scenario 和显式 `-out`，定向项用 `--` 显式过滤，不能当作 owner 全集。

| 验证 | 命令选择（另加对应 -out） | 实际结果 / 报告目录 |
| --- | --- | --- |
| L1 最小集 | `-level=L1` | 通过；`L1/20261106-05`，包含依赖边界和提示词黄金守卫 |
| L2 交付冒烟 | `-level=L2` | 通过；`L2/20261106-03`，隔离受控现场、原资源结算 |
| Web 全集 | `-level=L4 -suite=web -scenario web.unit` | 通过；`L4/web-20261106-04`，包含新增导航保护回归 |
| 存储全集 | `-level=L4 -suite=go -scenario go.storage` | 通过；`L4/storage-20261106-02`，含审核三项中 SQL/历史保存回归 |
| HTTP 正式专项 | `-level=L4 -suite=go -scenario go.http` | 通过；`L4/http-20261106-01`，未与 E2E 压力组合并行 |
| 环境正式专项 | `-level=L4 -suite=go -scenario go.environment` | 通过；`L4/environment-20261106-01`，非零完整选择 |
| 新活动 E2E + Edge | `-level=L3 -suite=runtime -env=controlled -scenario runtime.async-activities` | 通过；`L3/activities-20261106-04`，包含最终分页/新鲜度修复及断点/等效缩放重排 |
| 等待合并重启 | `-level=L3 -suite=recovery -env=controlled -scenario recovery.waiting-merge-restart` | 通过；`L3/waiting-merge-20261106-01` |
| 原项目维护 | `-level=L4 -suite=runtime -env=controlled -scenario runtime.project-maintenance` | 通过；`L4/maintenance-20261106-07`，原失败另存下文 |
| 活动定向 | `-- ./internal/storage/sqlite ./internal/product/query -run TestAsyncActivity -count=1` | 通过；`diagnostics/review-fixes-20261106-01` |
| 原接口及登记选择器定向 | 精确 App 活动读取/公开身份、测试目录/并行登记相关根 | 通过；`diagnostics/focused-20261106-02`，精确参数见 summary |
| 超时元数据 | `-- ./internal/runnerbridge ./internal/testmanagement -run 'Test(CandidateTimeoutRecordsObservationWithoutChangingExecution\|TMGenerationTimeoutRecordsObservationWithoutChangingExecution)$' -count=1` | 通过；`diagnostics/observation-timeouts-20261106-02` |
| 生产前端构建 | `npm --prefix web ci`、`npm --prefix web run build` | 通过；实际 Web asset/binary 摘要随 E2E build/summary 固定；未部署 |
| 文档与格式 | `python scripts/check_doc_links.py`、`git diff --check` | 通过，最终文档更新后再次检查 |

### 覆盖和限制

- AAC-A01/A02：四来源读取、稳定 ID、公开项目身份、批次业务失败与原子生成结算、历史问题排除、正常准备不误提醒、未知操作与计数一致有映射/SQLite/API 单测；前端覆盖本地→journal→正式去重、旧实例不复活、已加载页保留，真实 Workspace remount 覆盖项目/实例切换数据隔离。不是对所有未来未知业务枚举的通过承诺。
- AAC-A03/A04：新 E2E 真实 SteerLine/Git/SQLite/HTTP 展示三类工作，后台先在无浏览器情况下推进；重复 GET/浏览器不增加原执行输入，关闭浏览器后同构建服务重启仍为原活动/Run，并最终合并、停止和结算。SQLite 重开测试另覆盖部分生成关联、旧批次被替换后独立结算及原失败快照。不声称 E2E 同时并发运行三种业务，也不以 SQLite 反例冒充真实模型故障链。
- AAC-A05：520 条真实 SQLite 翻页稳定集不漏不重；新请求独立关联不丢失已加载页；观察失败不把运行改成失败，来源故障保留旧卡片；多子项新鲜度缺失/过期/已知错误有反例。500 行/载荷预算限制下计数可明确未知，不声称无限历史完整计数。
- AAC-A06：组件和实际 Edge 覆盖顶部入口、零项、手动开关、跨路由、筛选、失败保留、详情导航、320px 模态/背景 inert/滚动锁、Escape 焦点恢复及刷新。导航保护取消/接受由真实 DataRouter 组件回归覆盖。断点 640/641 和 200% 等效 CSS 视口重排最终浏览器补验通过；实际操作系统/浏览器菜单 200% 缩放仍未人工验证，不以视口仿真冒充实机设置。
- AAC-A07：固定 L1/L2 与受影响正式专项按表登记；新场景的 `activity-proof.json`、`browser/activity-browser-proof.json`、截图、trace 和 cleanup 证据在场景目录中。原项目维护完整回归最终在独立输出目录通过。真实 Runner/模型、真实 PR、安装部署与人工 AAC-M01 均未执行。

### 失败和补验留存

- `L1/20261106-01`：初始 query 导入领域模型触发依赖守卫；改为 storage 中性事实+纯映射，未放宽守卫，后续 L1 通过。
- `L4/web-20261106-01`：旧 App mock 白名单未包含活动 GET；仅补新读取路径，未放宽业务操作断言，后续 Web 全集通过。
- `L3/activities-20261106-01`、`-02`：先发现原始仓库身份与公开项目 ID 不一致，再发现窄屏原背景横向溢出；分别修复公开 scope 和模态背景裁剪，`-03` 通过。此前 owner 定向的 SQL/string 扫描、夹具空值归一、游标计数等失败均保留原 diagnostics，不反写为成功。
- `diagnostics/observation-timeouts-20261106-01`：新超时测试复用的夹具按 request selector 返回普通错误，未真正等超时；改为始终等待 context 的专用 peer 后 `-02` 通过，未修改产品错误预期。
- `diagnostics/owners-full-20261106-01`：扩大 owner 诊断未整体通过，不能当作 go.full。既有 `TestCM71ProductDependencyBoundary` 因 HEAD 已存在的 `api/run_usage.go`→billing 导入失败，已用 `git show HEAD:...` 核实非本变更新增，未扩大依赖许可；App 广泛集合在原默认 10m 下超时，不能宣称完整通过。另并行场景数量断言随新增受控场景从 3 改 4 后精确补验通过。
- `L4/maintenance-20261106-01/-02/-03/-04`：原调试夹具先缺 state/v8 recovery，再缺 same-snapshot run_observation，第三次暴露旧调试配置缺显式 access_mode，均在项目启动阶段阻断，未进入业务验收。补齐既有 peer 协议元数据及调试模板显式权限后另目录补验；未放宽生产协议门禁或 schema。失败包分别为 `C:\SteerlineDebug\dev-20261106-130051-a5428051-dirty`、`dev-20261106-130542-a5428051-dirty`、`dev-20261106-130758-a5428051-dirty`，保留现场。它们的原 proof 标 stopped/resources_settled=false；首个现场独立 verify-stop 仍因启动未完成的原项目闭环事实不齐而拒绝，不将失败清理反填通过。

- `L4/maintenance-20261106-04`：补 sandbox 后仍在项目启动被当前 development/2 run_configuration 要求 full_access 拒绝；最终模板显式 full_access，受控 peer 不运行原生 agent，不放宽原 schema。第四失败包 `C:\SteerlineDebug\dev-20261106-131255-a5428051-dirty` 同样保留。前三失败包的原 `home/application-api/shutdown.json` 均 clean=true、各项目 local_closed/locks_released=true，后两次原 Runner stopped/resources_settled=true，首个 Runner attempted=false；已核对原 backend/Web PIDs 不存在。独立 verify-stop 缺原项目 checkpoint 仍拒绝，故仅补充原退出事实，不反填测试 proof/cleanup_complete。

- `L4/maintenance-20261106-05`：两个项目启动成功，原浏览器准备期收到 invalid；只读 SQLite 核实旧调试仓库缺 `SteerlineFlow/environment-project.json`，未进入生成。补齐当前已有项目入口契约后另目录补验，不在产品读取增加 fallback。此次 proof 为 stopped/resources_settled=true、passed=false，资源结算与测试失败分开记录；失败包 `C:\SteerlineDebug\dev-20261106-131447-a5428051-dirty` 保留。
- 独立源元数据补充：观察保存失败时使用调用前已提交 revision，不能把尝试自增但未提交的版本写入健康元数据而阻挡后续成功观察。`diagnostics/observation-revisions-20261106-01` 的桥接/生成保存失败及超时四条精确回归通过，不改原保存/重试行为。

### 旧调试材料入口补齐审核材料

`debug-template/projects/SteerlineTest/SteerlineFlow/environment-project.json` 是原项目读取契约已经必需的固定事实入口，新增正文如下；本文件之前不存在（0 字节），当前 UTF-8 117 字节，末尾含换行：

```json
{"version":1,"requirements":["SteerlineFlow/testing.md"],"testing_catalogue":"SteerlineFlow/testing/test-list.json"}
```

业务原因：原调试回归因缺入口在材料读取阶段阻断，引用模板已经存在的测试要求/清单即可使用现有契约，不增加内联规则、默认路径兜底或模型调用。固定提示词正文/组装接线和黄金样本均未变，提示词正文差异无、UTF-8 字节变化 Δ0。入口资产独立审核和实际补验结果在最终结果补充中登记。

- `L4/maintenance-20261106-06`：原准备/生成已经 ready，进入首次候选启动时被 environment_not_ready 拒绝，证明旧场景未执行现有环境准入步骤；proof stopped/resources_settled=true、passed=false，失败包 `C:\SteerlineDebug\dev-20261106-131921-a5428051-dirty` 保留。补验场景仅在原启动前增加公开 environment_start 和 ready 等待，不写资格或绕过门禁，原测试断言全部保留。

旧受控场景环境材料增量：新增 `SteerlineFlow/environment.md`（0→180 UTF-8 字节）声明 manifest5、固定 PowerShell 脚本、无依赖；新增 `SteerlineFlow/scripts/verify-environment.ps1`（0→68 UTF-8 字节）在原生调用时明确抛错，受控 peer 沿已有 scriptedEnvironment 返回同绑定结果，不能误用真实 native 执行。新增正文：

````text
# Controlled debug environment

```steerline-environment
{"version":5,"script":"SteerlineFlow/scripts/verify-environment.ps1","interpreter":"powershell.exe","dependencies":[]}
```
````

```powershell
throw 'Scripted public peer fixture; native execution is forbidden'
```

这些是旧受控调试现场的版本化材料与公开准入步骤补齐，不变更产品提示词、组装规则、schema、资格写入或任何原测试断言。实际字节含末尾 LF；独立资产审核与本轮补验结果后附。

最终增量独立审核：`review_activity_implementation` 已逐项核对失败版本修复、模板 full_access 的 development/2 schema 交集，以及三个项目材料资产的完整正文和原始字节（0→117/180/68）。同意这些旧受控夹具补齐；无产品 fallback/资格写入/原断言放宽，固定提示词及组装正文 Δ0。未发现新增可确认问题。该审核只读，不代替本节实际测试结果。

## 最终结果

2026-11-06：最终 `L1/20261106-05`、`L2/20261106-03`、`L4/web-20261106-04`、`L4/storage-20261106-02`、`L4/http-20261106-01`、`L4/environment-20261106-01`、`L3/activities-20261106-04`、`L3/waiting-merge-20261106-01`、`L4/maintenance-20261106-07` 均实际通过。专项选择、源代码状态、Go/Edge 环境、冻结二进制 `build_sha256` 和清理事实见各原始 summary；不是 go.full 或真实模型验收。

维护 `-07` 的真实浏览器保留全部原断言，实际覆盖初次准备、跨项目隔离、本地更新、不同材料的生成、并行候选独立快照、旧候选材料过期后保留原文档和旧 PR 合并交付，最终 `proof.json` 的 passed/stopped/resources_settled 均 true，顶层 cleanup_complete=true。该场景通过证明补齐旧夹具的现有前置后原业务回归保持，不覆盖/删除 `-01` 至 `-06` 的失败。

首期功能代码与顶部入口＋右侧抽屉已实现并完成独立审核；原操作页面保持。人工 AAC-M01、实际浏览器菜单 200% 缩放及真实模型/PR 链未执行；扩大 owner 诊断的既有依赖守卫失败及超时仍按上文保留。未部署，也未改原安装现场。

验证完成后的最终独立复核：同一只读子代理逐一读取上述 9 项最终 summary 和维护 -07 proof，确认全部通过、适用 E2E 清理完整；此前代码与资产问题均关闭，文档与实际结果一致，未发现本次交付阻塞。该复核未修改源码、未执行测试或部署。

## 观察边界收紧（2026-11-06，PR 交付轮）

用户授权：独立只读资源隔离、来源超时预算、关联零摘要、SQL 粗筛契约，同步 Spec/Plan，验证后独立审核并提 PR；不部署。首期实现仍在同一工作区，本轮 PR 一并包含；下列为本轮增量，不覆盖上文原测试。

- AAC-A08：实际观察池禁写、与业务连接分离、观察池饱和/不可用不阻断保存、context 取消昂贵 SQL、原 owner 关闭资源；来源独立 1 秒总预算、部分故障保留；mode=links 每来源一次关联且零摘要/列表，API 参数与公开身份、UI 独立关联和分页。
- AAC-A09：真实 SQLite 以无业务粗筛的全事实投影为参考，对四来源 ongoing/attention/recent 断言 SQL 超集，并检查最终分页集合/计数一致；候选 288 组合加检测/显式检查/批次与子生成组合，包含未知和缺失字段、无 Runner 投影、停止和未核实副作用。

定向报告：`diagnostics/isolation-20261106-01` 通过资源池/来源预算/关联测试；`diagnostics/coarse-contract-20261106-01` 真实发现候选 nil projection 的 waiting/queued 组合，以及缺 state 检测被 SQL NULL 粗筛漏掉，原失败保留；修 SQL 为保守超集后在新目录补验。未修改最终状态解释来让测试通过。其余本轮报告与独立审核结果实施后补充。

### main 整合

提 PR 前 fetch 发现 main 已推进到 `b3c654ed`，合入原生执行配置/项目统一就绪/回执核实。安全保存本聊天修改后先快进到 main，再逐项恢复；冲突保留上游配置身份校验、环境 FormatVersion、readiness 路由、模板显式权限，并合入观察字段/入口。两条开发工作都占用了 schema 40，最终唯一版本改为 41，无迁移。整合前本轮 L1/Web/storage/http 已通过，但不视为整合后通过；最终重新验证。未部署。

### 整合后本轮验证

当前基线 `b3c654ed` 加本 PR 工作区，schema 41。本轮实际报告根仍为 `C:\SteerlineDebug\wt1369-activities\test-results`：

| 检查 | 实际报告目录 | 结果 |
| --- | --- | --- |
| 资源/预算/关联/四来源组合契约/App 公开 scope 定向 | `diagnostics/isolation-main-20261106-01` | 通过；精确 `-- ./internal/product/query ./internal/storage/sqlite ./internal/app -run TestAsyncActivity -count=1` |
| L1 固定集 | `L1/isolation-main-20261106-01` | 通过 |
| L2 核心交付 | `L2/isolation-main-20261106-01` | 通过，清理完整 |
| Web 全集 | `L4/web-isolation-main-20261106-01` | 通过，含专用 links 参数与分页保留 |
| 存储完整专项 | `L4/storage-isolation-main-20261106-01` | 通过，含池隔离/禁写/关闭与粗筛组合契约 |
| HTTP 完整专项 | `L4/http-isolation-main-20261106-01` | 通过，按规则与 E2E 压力串行 |
| 环境完整专项 | `L4/environment-isolation-main-20261106-01` | 通过，完整选择 |
| 活动受控 E2E + Edge | `L3/activities-isolation-main-20261106-01` | 通过，含专用关联真实 HTTP 无计数/列表验证、原 ID/Run 保持，cleanup_complete=true |
| 前端生产构建 | `npm --prefix web run build` | 通过，整合后 2192 modules，E2E 冻结 Web 二进制摘要随报告保存 |

L1/L2、正式专项、E2E 的完整命令均按 Plan 第 8 节指定并带对应绝对 `-out`。源身份、选择、实际参数和资源证据以原 summary 为准。整合前验证只作历史参考；本轮不宣称 go.full 或真实模型/PR 链通过。旧项目维护完整回归 -07 属首期前一基线，未用作整合后本轮通过声明。

### 独立 PR 审核与修复

新独立只读子代理 `audit_activity_boundary_pr` 从完整暂存差异（61 文件）核查，而非沿用旧验收结论。确认资源池/预算/专用关联/SQL 契约、main 整合和三个材料资产边界成立，另确认一项 P2：环境已成功 Inspect 后再失败，只有普通 Diagnostic 而无独立观察错误，投影仍能在一分钟内显示 current；恢复后旧观察诊断保留。

修复由环境 owner 在原 Record 保存 `observation_diagnostic`，失败保留上次成功时间；成功读取清除该观察错误，仅当普通诊断确由该观察错误留下时清除它，不覆盖其他业务或首次提交事实。查询只读该字段，将已知观察错误映射 unavailable，业务状态保持。新增真实 owner→SQLite→投影的成功→错误→恢复回归，并验证无关业务诊断不被清掉。不增加通用活动写服务或查询副作用。

`diagnostics/environment-observation-20261106-01` 原失败保留：新测试只推进了提交阶段、未执行独立首次 Inspect 就要求 current；补齐两步后 `diagnostics/environment-observation-20261106-02` 实际通过。产品预期未放宽。修复后按 Plan 补验并独立复核，结果后附。

原独立审核者复核：`audit_activity_boundary_pr` 确认环境 P2 已实质关闭、无修复引入的新确认问题，独立只读检查 owner/SQLite/纯投影回归和最终 L1 报告通过。新观察诊断沿原 owner 同一次事实保存，不改变原提交、资格或结算流程。根聊天继续完成受影响补验，报告后附。

审核修复后已通过：`diagnostics/isolation-main-20261106-02`（四包定向，资源/SQL/公开 API/owner 回归）、`L1/isolation-main-20261106-02`、`L2/isolation-main-20261106-02`。环境完整专项和活动 E2E 使用各自 `-02` 独立目录补验；Web/storage/http 本轮 `-01` 完整专项在同一最新 main 已通过，其职责代码在审核修复中未再改动。

### PR 提交前最终结果

本轮最终记录（基线 `b3c654ed`，最终 schema 41）：

| 验证 | 最终报告目录（同一报告根） | 结果 |
| --- | --- | --- |
| 资源/预算/粗筛/链接/公开 API/环境 owner 定向 | `diagnostics/isolation-main-20261106-02` | 通过，四包完整命中指定根 |
| L1 | `L1/isolation-main-20261106-02` | 通过 |
| L2 | `L2/isolation-main-20261106-02` | 通过，cleanup_complete=true |
| Web 全集 | `L4/web-isolation-main-20261106-01` | 通过 |
| storage 完整专项 | `L4/storage-isolation-main-20261106-01` | 通过 |
| HTTP 完整专项 | `L4/http-isolation-main-20261106-01` | 通过，与 E2E 压力串行 |
| environment 完整专项 | `L4/environment-isolation-main-20261106-02` | 通过，新观察恢复根实际命中 |
| 活动受控 E2E + Edge | `L3/activities-isolation-main-20261106-02` | 通过，cleanup_complete=true，原活动 ID/Run/关联、页面交互及资源结算保持 |
| 生产 Web 构建 | 整合后 `npm --prefix web run build` | 通过 |
| 文档/格式 | `python scripts/check_doc_links.py`、`git diff --check` | 通过 |

独立 PR 审核确认的唯一 P2 已修复并复核关闭，无新增可确认问题。提示词正文/组装/黄金样本未变，材料资产正文和字节已独立审核。范围为本聊天首期活动中心及本轮边界收紧，保留 main 的原生配置身份/就绪门禁/原回执核实；旧格式不迁移，原页面业务操作不变。提交 PR，不合并、不部署。人工体验、实机菜单 200% 缩放和 go.full/真实模型链仍不作本轮通过声明；原失败与未验边界保留上文，不反填原报告。

PR 发布前最终独立复核：`audit_activity_boundary_pr` 逐项核实最终四包定向 40 项通过、零失败/跳过；最终 L1/L2/environment/活动 E2E 与本轮 Web/storage/http 全部通过，适用 E2E 清理完整。环境 P2 关闭，无未关闭确认问题，文档与报告一致，可提交 PR；只读审核未运行测试或部署。

## PR #465 冲突解决（2026-11-06）

将 main `5b8e08bc`（PR #464，原 Run 手动冲突续作）合入本 PR `63da5044`。唯一文本冲突位于 Spec 索引，保留异步活动中心与 PR 冲突续作两条入口；桥接代码自动合并后核对，原 message 保存屏障/未知结果不重发和活动观察元数据均保留。未更改统一观察边界、schema 41 或原业务权限。相对当前 main 的固定提示词/组装/黄金样本无差异，正文/字节变化 Δ0，沿用上游已合入实现，不新增提示词修改。

本次新报告根仍为 `C:\SteerlineDebug\wt1369-activities\test-results`：`L1/merge-20261106-01`、`L2/merge-20261106-01`、`L4/web-merge-20261106-01` 全部通过；四包交叉定向 `diagnostics/merge-20261106-01` 与桥接 owner 全集 `diagnostics/message-merge-20261106-01` 通过，覆盖原 Run message、未知结果不重发、保存屏障、PR 冲突意图及活动 SQL 粗筛/资源预算契约。定向参数为 `-- ./internal/runnerbridge ./internal/taskrunner ./internal/product/query ./internal/storage/sqlite -run 'Test(AsyncActivity|Candidate|PRConflict|SteerRunner|.*Conflict)' -count=1`，桥接全集为 `-- ./internal/runnerbridge -count=1`；定向不代表这些包的全量专项。L2 清理完整。`npm --prefix web run build`、文档链接和 `git diff --check` 通过。

仅更新原 PR 分支以解决冲突，不合并 PR、不部署；原验证与未验范围不反写。
