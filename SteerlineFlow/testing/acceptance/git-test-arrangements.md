# Git 管理任务测试安排：验收记录

日期：2026-11-09。关联本轮无编号普通开发的 [Spec](../../spec/git-test-arrangements.spec.md) 和 [Plan](../../plans/git-test-arrangements.plan.md)。状态：已实施，以下功能自动验收及独立审核通过，不纳入 roadmap 调度。原文档阶段及失败记录保留；持续页面轮询的负载限制单列，不宣称已修复。

## 范围与执行方式

验证同名 `.tests.json` 随材料同批生成并入 Git、运行时读取和候选冻结、删除独立补安排流程，以及原项目准入/测试进度/交付保持。自动步骤、命令、环境、通过条件及证据要求以 Plan 的 T0–T6 为准，不在此复制命令。Spec 中的验收表定义成果要求；无需新增人工签收。

实施前固定实际源码提交、Runner 构建及隔离现场；每项执行后登记日期、受测版本、结果和本次独立输出绝对路径。原始报告沿 [paths.md](../../paths.md) 放仓库外；失败和补测分别保留，不因记录更新使产品验收自动通过。

| 项 | 方法与执行定位 | 当前状态 | 准备、阻塞范围及结果 |
| --- | --- | --- | --- |
| GAT-A01 | 自动；T1 的解析/来源反例 | 已通过 | T1 同提交读取、严格选择/缺失/非法文件及空说明反例通过；T4 覆盖两类诊断与无生成控件，运行阶段不调用模型 |
| GAT-A02 | 自动；T1、T3、T4、T6 的生成/发布/入图 | 已通过 | T1/T3 的生成、编辑、发布/合并/入图及重放反例通过；T6 两个真实文档结果均带 arithmetic.unit，平台编码同批 JSON 后完成原链 |
| GAT-A03 | 自动；T1、T5 的版本与冻结 | 已通过 | T1 相关/无关清单及版本/快照反例通过；T5 三候选各自冻结，旧候选保持原提交；负载边界另记 |
| GAT-A04 | 自动；T1、T2、T3、T5 的项目资格与预留事务 | 已通过 | T1/T2/T3 验证全部未完成任务资格、预留事务与回放，静态读取失败保持 unknown/invalid，未借旧 ready 放行 |
| GAT-A05 | 自动；T1、T3、T4、T5 的入口删除与零生成 | 已通过 | 专用服务/配置/批次/表/API/CLI/UI/活动及受控生成脚本清除；T1–T5、正式登记与源码检查通过，其他生成 owner 保留 |
| GAT-A06 | 自动；T2、T5 的输入/进度、交付及停止 | 已通过 | T2/T5 验证原安排/进度绑定、异常消费、旧 PR 唯一交付与资源结算；仅声明调用方和受控公开快照范围 |
| GAT-A07 | 自动及独立审核；T0–T4、T6 与提示词正文审阅 | 已通过 | 指南/固定正文/模板/登记一致；T0、最终 L1/L2、T6 与全部实现增量独立复核通过，字节与实际正文差异可追溯 |

## 最初文档阶段检查（历史）

最初文档阶段（2026-11-09）仅新增方案文档并更新两个索引：

- `python scripts/check_doc_links.py` 通过：715 个 Markdown、4422 个本地导航链接、零错误。
- `git diff --check` 通过；新增文档另作 UTF-8、末尾换行和行尾空白检查。

这些静态检查不将 GAT-A01–A07 改成已验收，也不代表产品功能、真实模型或提示词独立审核完成。未修改运行代码、提示词资产或测试清单。

## 2026-11-09 Spec/Plan 审计与收敛

本轮按用户要求对照现有 Snapshot、roadmap 文件组装/确认/合并核验、材料摘要及实际测试驱动审计并修正文档；属于当前聊天的文档自审，不替代未来提示词实施变更的独立审核。

| 发现 | 修正 |
| --- | --- |
| 原 Plan 再定义 Git 来源字段及原文件摘要，会重复 Commit、PlanPath 和项目入口已能定位的事实 | 复用原 Snapshot，删除生成字段；JSON 路径推导，清单来源从原提交追溯，只替换材料摘要中的清单部分 |
| JSON Document 的 path/body 设计让模型额外输出可推导路径并转义嵌套 JSON，增加两份结果同步风险 | 模型只返回测试 ID 数组；原文件组装函数统一生成 JSON，diff/确认/发布/核验共用，不另存正文 |
| 新增独立 L3、强制新测试前缀和两轮 live 会重复现有 owner、维护和进度覆盖 | 删除新 L3 与测试命名要求；复用原 owner/场景，仅保留一次必要的真实 roadmap 生成。额外候选 live 仅在实际新增风险需要时追加 |
| 另设人工体验签收会增加本可自动验证的流程 | 删除 GAT-M01；界面行为由既有 Web/浏览器测试覆盖 |
| 新增用例容易被解释为必须提前登记新 ID 或拆任务 | 明确由现有登记入口承接实施期新用例，无可用入口才沿原问题机制澄清，不新增待实现测试模型 |
| 原测试计划把受控进度快照消费表述成实际测试执行，证据范围扩大 | 修正 A06/T5 的证明边界，只验证此次改变的输入/绑定/消费，不把 fixture 当原生执行证据 |
| 生成提交与实际合入提交的清单变化未明确呈现 | 两阶段各按自己的提交检查，所选定义变化沿原差异机制提示，不新增版本锁或模型复审 |

保留现有项目统一就绪和 acceptance 引用语义。它们仍可能影响新建资格及其他材料过期展示，属于现行边界，本轮不顺带重设计。

审计修订后的检查：本地导航再次通过（715 个 Markdown、4422 个链接、零错误）；`git diff --check` 和三份文档的 UTF-8/换行/空白、验收 ID 及 T0–T6 引用一致性检查通过。只修改方案和记录，未运行产品或模型测试，实施验收项仍全部待验收。


## 实施与独立审核

2026-11-09，用户授权实施并提 PR。已恢复 Git 同名安排、roadmap tests 数组同批发布、原候选快照及事务资格，删除独立补安排服务、配置、批次、API/CLI/UI/活动和无消费者表；schema 为 42，无旧格式迁移。Git 更新仍使用 maintenance_requests，roadmap 继续使用移至现有 runnerbinding owner 的共享 ContentQueue。

独立只读审核由 `/root/independent_review` 完成并通过，覆盖实际源码、旧链路清理、提示词正文与接线；修复了动态配置混入静态缓存、raw/canonical 长度不一致、观察 SQL 残留列三项问题，并补回归。测试语义核验移出纯 roadmap/model，未放宽架构守卫。额外部署/环境专项是删除旧生成责任影响其消费者后的必要回归。

固定正文与接线的[实际差异](evidence/git-test-arrangements-prompts.diff)已保存供审阅。业务原因：测试语义选择随正式材料一次完成，平台编码 Git JSON；删除运行时单独选择能力，执行方仍消费原冻结安排。表内为 UTF-8/LF 文件字节（含文件末尾换行）；实际 short caller rendered 正文为 503→522 字节。删除项表示其专用能力整体移除，未以另一处 inline 规则替代。

| 文件 | 修改前 | 修改后 |
| --- | ---: | ---: |
| `internal/prompts/testing-generate.md` | 266 | 0 |
| `internal/prompts/testdata/testing-generate.golden.md` | 328 | 0 |
| `internal/prompts/roadmap-documents.md` | 511 | 530 |
| `internal/prompts/testdata/roadmap-documents.golden.md` | 503 | 522 |
| `internal/prompts/materials/roadmap-documents.md` | 1676 | 2085 |
| `internal/prompts/materials/roadmap-spec-guide.md` | 4250 | 4474 |

完整 rules 增长来自同批 tests 数组、Plan 引用清单及缺 ID/空安排规则，替代旧生成模板和专用链路。独立审核确认：未增加 inline 指令、缺材料兜底、安排二次权威或模型复审；黄金样本及摘要更新仅固定已审阅正文，不作为批准依据。

## 最终实现、版本与执行证据

最终集成实现为 `cd815bf5320739c40811d575559d81ba1e85bb26`，已合入 `origin/main` 的 `5f2502ee9979dc269539f253334ae6b76948e93d`（远端用量默认设置）。唯一文本冲突是验收索引，保留双方条目；独立复核确认本任务 11 个所选正式定义、提示词正文及守卫未变、旧链路未回带。此后只补验收文字和索引。

原始证据统一根为 `C:\SteerlineDebug\wt-5cba-git-arrangements\`。下表路径相对该根；基线及当时工作区状态由各 summary 的 environment 记录。标有差异的早期结果不冒充最终干净提交：最终集成版本另通过域 owner、材料边界、L1/L2、HTTP、前端与生产构建；旧专项相关组件保持，批量读取、资格和受控计数的新增改动另有定向补验。

| 验证 | 结果（含该入口准备/执行耗时） | 冻结源码 | 证据入口 |
| --- | --- | --- | --- |
| T1 域 owner（含 ContentQueue） | 通过；5.6 秒 | `cd815bf5（干净）` | `diagnostics/domains-integrated-20261109a/summary.json` |
| T1 瞬时读取/规范长度边界 | 通过；18.3 秒 | `cd815bf5（干净）` | `diagnostics/material-boundaries-integrated-20261109a/summary.json` |
| T1 项目资格/中断恢复/冻结 | 通过；541.1 秒 | `fa407294 + 当时实现差异` | `diagnostics/stopped-run-materials-20261109b/summary.json` |
| T1 受控公开预检/PR/请求计数 | 通过；9.1 秒 | `9357fbba + 当时实现差异` | `diagnostics/controlled-cleanup-20261109a/summary.json` |
| T2 L1 | 通过；28.1 秒 | `cd815bf5（干净）` | `test-results/L1/integrated-20261109a/summary.json` |
| T2 L2 | 通过；115.5 秒 | `cd815bf5（干净）` | `test-results/L2/integrated-20261109a/summary.json` |
| T3 go.storage | 通过；96.3 秒 | `221812ec + 当时实现差异` | `test-results/L4/storage-final-20261109a/summary.json` |
| T3 go.roadmap | 通过；441.0 秒 | `221812ec + 当时实现差异` | `test-results/L4/roadmap-final-20261109a/summary.json` |
| T3 go.environment | 通过；578.6 秒 | `221812ec + 当时实现差异` | `test-results/L4/environment-final-20261109a/summary.json` |
| T3 go.deployment | 通过；685.4 秒 | `9357fbba（干净）` | `test-results/L4/deployment-final-20261109b/summary.json` |
| T3 go.http | 通过；50.8 秒 | `cd815bf5（干净）` | `test-results/L4/http-integrated-20261109a/summary.json` |
| T4 web.unit | 通过；17.4 秒 | `cd815bf5（干净）` | `test-results/L4/web-integrated-20261109b/summary.json` |
| T5 testing.runtime-progress | 通过；148.9 秒 | `dcfc4de2（干净）` | `test-results/L3/runtime-progress-final-20261109b/summary.json` |
| T5 runtime.async-activities | 通过；141.5 秒 | `dcfc4de2 + 当时实现差异` | `test-results/L3/activities-final-20261109b/summary.json` |
| T5 runtime.project-maintenance | 通过；385.3 秒 | `b085edd4 + 当时实现差异` | `test-results/L4/maintenance-final-20261109g/summary.json` |
| T6 真实 roadmap | 通过；221.9 秒 | `b085edd4 + 当时实现差异` | `diagnostics/roadmap-live-20261109b/summary.json` |

T4 最终 Vitest 共 343 项、零 skip；生产构建通过，日志为 `diagnostics/web-build-integrated-20261109a/build.log`。L1 总计约 28 秒，L2 约 115 秒；原预算及集合未缩窄。HTTP 在所有 E2E、Vitest 和材料诊断结束后单独运行，保留 30 秒、1000 次发布、8 读者的原压力条件。T0 最终导航通过（716 个 Markdown、4422 个本地链接、零错误），`git diff --check` 通过。

T6 使用独立 Runner `C:\Project\SteerRunner\bin\steerrunner.exe`，版本 `0.2.0-dev.4`，SHA256 `b34b2f7e7fb17ca5d75fbb89c5096a16130e89392e3ec3ef6b74127e1a5d2b53`，resource digest `cc3979d7b1ba092a073882d1ae76c06829c8de4716dcf2073b8fb0f59ec2eb56`。原生 CLI 为用户 PATH shim 对应的 `C:\Users\lmas1\.local\bin\node_modules\@openai\codex\node_modules\@openai\codex-win32-x64\vendor\x86_64-pc-windows-msvc\bin\codex.exe`（0.166.0），原测试模型 `gpt-6.1-sol` / low、read_only 及 20 分钟上限保持。

真实结果在 `diagnostics/roadmap-live-20261109b/evidence/`：`documents-review.json` 保存原测试数组，`admitted.json` 为 admitted；内部键 T1/T2 的两任务均选择 `arithmetic.unit`，平台编码真实同名文件后经过隔离 provider 发布/核实合并并入图，未在驱动中追加测试选择。`runner-settlement.json` 证明原实例停止且 resources_settled=true。此轮为一次实际完整 roadmap 联验，未运行额外候选模型联验、真实远端业务 PR 或安装维护。

独立审核 `/root/independent_review` 对最终实现及全部修复增量均只读复核通过，覆盖批量读取的同提交/regular blob/摘要与失败语义、原预算、受控公开配置校验与能力声明、冻结 base 的 PR fixture、意外 generate 计数、验证分工及主干合并。实际正文差异文件 SHA256 为 `120b5a93167e83289c0de0dc5420639335144397114a46e0f1da62e8a5bc26ca`；字节表保持上述批准值。审核不替代测试，测试也不替代审核。

## 原失败、修复与未证明范围

所有失败目录保持原结果，未覆盖或改写成通过：

- `diagnostics/app-20261109T050609850Z` 首轮只有旧测试仍期待手动准备；按 Git 有效材料可重建的目标修正断言，并由后续资格回归补验。
- `test-results/L1/20261109T051341457Z` 发现环境专项两个旧根名，修正正式映射而非放宽非零/skip/根集合检查；最终 L1 已通过。
- `test-results/L4/storage-20261109T051742140Z` 的空快照夹具缺 digest，补正确摘要后完整 storage 通过。
- `test-results/L4/deployment-final-20261109a` 三项 stopped Run 前置未就绪；`diagnostics/stopped-run-materials-20261109a` 定位为逐文件 Git 耗尽原 5 秒预算。复用现有批读/请求缓存，未延长预算；40 项定向及完整 deployment 补验通过。
- 维护 `maintenance-final-20261109a` 在部署配置预检前失败（受控 peer 缺 validate），无业务场景通过；补现有公开 schema 校验及拒绝反例。`b` 暴露 seed 丢失已实现的 derive 能力；补同源 capability 及完整信封。`c` 暴露 fixture 切换主工作区；改从原冻结 base 用 commit-tree/push 形成同样测试 PR，保留主分支及原 PR 重放。
- 维护 `d/e/f` 的扩充长场景在原 300 秒内未完整结束。`e` trace 记录约 80 次 maintenance 请求、总响应约 448 秒（含并发重叠，不是单一墙钟时长）。最终移除本次重复添加的两轮负例 Git 提交（错误解析/显示由 T1/T4 覆盖），命令阶段关闭页面轮询，仍使用同会话真实认证 API 和原版本门禁；重开原候选页继续原 UI、旧 PR 和跨项目断言。`g` 整场及清理通过，浏览器约 4.9 分钟、原 300 秒及每次 60 秒等待均保留。**未证明持续维护页轮询与三候选并发的原负载通过，也未修复该观察开销；不存在整链缺失/坏 ID 浏览器通过声明。**
- `diagnostics/roadmap-live-20261109a` 首次 WindowsApps 原生 CLI 路径预检 environment_unavailable，原 Runner 已结算；换用已验证的用户原生 CLI 后 `b` 完整通过。首次未取得模型文档结果。
- `test-results/L4/web-integrated-20261109a` 与 L1 并发时，一项 project-switch 等待活动计数标题失败；单文件 19 项通过。待 L2/其他 Vitest 结束后，以原断言和等待条件单独运行全集，`web-integrated-20261109b` 343 项通过。未宣称该并发执行组合也通过。

受控维护包装器实际生成的隔离包位于 `C:\SteerlineDebug\dev-20261109-*`；精确目录、manifest、浏览器 trace、原 stopped/resources_settled 与重置证据由各 scenario、`debug-package.txt` 和 `proof.json` 定位。首轮预检失败不冒充清理完整；后续所有维护业务轮均保留自己的停止/结算证明，最终通过轮 cleanup_complete=true。原始现场和报告保留，不清理安装环境。

补充只读观察 `diagnostics/owned-package-observation-20261109a/observation.json`：七个本任务调试包各自目录内的可执行进程均为零。该进程范围观察不替换原 Runner 结算，也不把首轮预检失败或任一失败场景改成通过。
