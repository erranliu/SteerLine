# 第三批：保留资源与命令收口

绑定[Spec](../../../../spec/product-simplification-03-resources-cli.spec.md)和[Plan](../../../../spec/product-simplification-03-resources-cli.plan.md)，实施基线8e46537f（第二批已合入）。实现、自查及最小验证完成；多后端、远端协议、多候选、完整任务依赖与唯一交付继续保留。

Task 丢弃不再关闭 PR、删除 worktree 或分支，也不生成这些资源的自动待办。Task 只保存 stop_pending 原执行停止身份；实际活动/未知执行、停止意图和退出确认继续单独核验。工作区/分支/PR 历史定位不因资源缺失而丢失，详情显示“保留，需人工处置”和采样时间，明确不是存在/已清理证明。退出业务不复活候选、不新发业务输入；丢弃/放弃后的合并及迟到多合并仍记录冲突，不覆盖唯一 Delivery 或解锁后继。

删除 tasks/stop/projects/task export；使用 task list、stop-service、console。task graph 只接受 config/cursor/limit，输出 JSON，可自行重定向；分页保留采样身份、截断边界与基本直接关系。补前置、环检测、稳定顺序、依赖/启动资格、control/answer 的原 request/command 查询以及内部 codex-session-launch 保持。

## 删除、保留与副作用分类

| 原消费者 / 动作 | 处理与现有所有者 |
| --- | --- |
| execution_discard_cleanup 的 PR/worktree 清理循环、PR/workspace 进度/结算回执、CleanupProgress 事务 | 删除。保留原执行停止循环和 StopReceipt 原子事务，精确绑定、原操作对账及实际 absence 核验不变 |
| DiscardFacts/丢弃事务的 pull_request_close/workspace_cleanup；Task/receipt.cleanup_pending | 删除破坏性待办，收口为 stop_pending；仍核验全部候选/历史 PR 与未结算事实，已合并拒绝、未知沿原请求 |
| publication.Provider/Client.Close、PRProviderConfig.close、关闭命令解析/冻结/展示 | 无产品消费者，删除；必要 Query/Check、认证引用、固定 remote/base/head 与真实 merged 核验保留。close 即使 null/空对象也严格拒绝 |
| workspace.Cleanup 的 Git worktree remove/branch -D | 退出生产二进制；原精确 scratch 清理辅助代码移至同 owner 的 cleanup_test.go，仅测试编译。真实 E2E 自有 PR/分支/进程/scratch 清理由 testenv/testkit 继续负责 |
| 顶层 tasks/stop/projects、项目菜单 ResolveProject、独立 task export 原子文件发布 | 无别名或转发，副作用前拒绝；console 的 ListProjects、服务三个不同入口、脚本 control/answer/task 命令及内部 launcher 保留 |
| QueryTaskGraph/GraphFilter/NormalizeFilter、核心高级 Project 邻域算法与无消费者 Node.Status | 删除展示投影和状态/方向/深度契约；TaskGraph/Paginate、基本节点/边/边界、直接前后置/Spec/Delivery、核心 Cycle/StableTaskOrder/readiness 保留 |
| Codex 受管执行停止与 observer/句柄关闭 | 保留原过程、身份、receipt/absence 屏障；未知只查原请求，不重复 kill。停止失败不回滚业务退出或清理其他候选 |
| Orca / remote 名为 LifecycleCleanup 的 worker release、消息 ack、协调 terminal 排空后关闭 | 保留各 adapter 原 intent/receipt/ack 和精确原生 ID。它们不是业务 PR/worktree/branch 删除；远端路径仍由其目标协议处理 |
| 探针/反馈临时文件、连接/HTTP body、既有进程/观察句柄关闭 | 保留必要协议/资源收尾；本批没有修改原生 release/ack 行为或新增平台认证 |

## 格式与界面自查

SQLite16、FixedExecutionProfile8（冻结 provider 删除 close），graph native-task-graph/v2 与 cursor v2；checkpoint19、Codex profile/resource codec2、全局execution格式2和内建目录v3保持。旧 SQLite/profile/cursor 和旧 close 字段在副作用前拒绝并保留现场；无兼容读取、迁移、回填或清理状态机。

修复自查发现的 UI 残留：丢弃 Task 的旧 pending 问题不再打开/保留可提交编辑器或提示正常合并交付；晚到合并明确显示冲突需人工处置。实际执行仍可 active/unknown，原停止身份另行显示。候选详情补历史 PR 和原生 worktree ID/branch，资源不存在时只读定位保持。控制台全包112项通过，不新增清理按钮或恢复权限。

## 最小验证

| 范围 | 实际结果与证据 |
| --- | --- |
| PS3-01–03 双候选资源、停止事务、未知/迟到、合并冲突、唯一交付 | [原资源定向](directed-resources-fixed/summary.json)：28项中27通过，1条旧断言误把必须保留的停止待办当作破坏性清理；原报告保持失败。该项改为只接受原执行停止项后，[修复定向](resources-final/summary.json)6项全过，包括弃任务晚合并冲突，以及 Orca 原终端停止/未知和 remote 原 ack 回查。未重跑27项未变路径或把旧失败重标通过 |
| PS3-04 CLI/图分页/基本关系/依赖/发布/配置 | [最终定向](cli-final/summary.json)：42项全过，退役命令/参数在项目打开前拒绝，cursor v1拒绝，页面变化失效、边界与终端 bidi保护保持 |
| PS3 UI 历史定位、丢弃任务旧问题与合并提示 | [控制台全包](console-final/summary.json)：112项通过，包括两项新 PS3 断言；历史资源/PR、采样时间、实际执行与停止身份分开 |
| L1 | [最终当前 L1](l1-final/summary.json)：passed/complete=true，12组全部通过，执行110.085秒。首次L1也通过125.686秒；UI修复后按需复验，没有修改范围/期限或删除固定用例 |
| PS3-05 L3 controlled / recovery.waiting-merge-restart | [场景报告](controlled-isolated/summary.json)、[场景收尾](controlled-isolated/scenarios/recovery.waiting-merge-restart/scenario.json)、[精选身份/交付事实](controlled-isolated/selected-proofs.json)：通过，准备7.980秒、执行164.934秒；重启保持原 Task/数据库/Workflow/Execution/session/workspace，session/dispatch各1次；合并后原唯一 Delivery、后继ready但未启动；test-owned cleanup_complete=true |
| 静态 / 构建 | 当前产品构建、受影响 go vet、文档链接与diff检查通过；E2E基础设施tag编译通过。生产无PR Close/worktree Remove/branch Delete入口，原生协议及测试自有清理保留 |

原资源定向的原子 stop 回执测试保留提交失败回滚、重放原结果及Task/Execution分离；先前新增夹具重复插入活动 Attempt、旧 provider.Close 测试/旧 AssertFullAccess 引用造成的失败均保留本地，并按当前契约修正。没有新增回退或降低实际身份检查。

第一次 L3 因已有 recovery 测试仍调用第二批退役的 AssertFullAccess 而编译失败，替换为现有 AssertFixedNativeLaunch。随后与资源/L1回归并发运行时，在task create的既定30秒期限失败，没有任务提交；原失败及cleanup_complete保留。隔离重验在原30秒/5分钟/清理期限下通过。并发负载是耗时的推测，未作确定根因结论，也没有调整期限。此场景使用controlled worker/provider和真实Git HTTP/SQLite，不是原生模型/远端/live认证；没有执行完整L2/Full或真实平台PR场景。

## 行数与证据范围

[同口径行数](line-summary.json)：internal+cmd物理Go行（含空白/注释，生产与_test分列，排除docs/logs/tests/integration）；第三批生产65196→63905，净减1291；测试33237→33409，净增172（审计回归已计入），已计入新增必要测试和测试清理工具的移动。

| 批次 | 生产净减 | 测试净减 | 各自实际比较基线 |
| --- | ---: | ---: | --- |
| 1 三窗/反馈 | 1417 | -102（净增） | 69c839be，原结果固定在172cea6c历史快照 |
| 2 固定原生配置 | 1211 | 1234 | 6c16b8dc，不计其他PR收益 |
| 3 资源/CLI | 1291 | -172（净增） | 8e46537f（前两批已合入） |
| 分批合计 | 3919 | 960 | 各批独立差值相加，不把其他主分支变化纳入 |

单候选保留资源原已存在、第二批认证删除和第一批共享阅读收益不重复计入本批。原始输出与重复/失败资料按主分支约定留本地；只提交精选当前/已说明的历史报告，保留原采样source_commit、dirty状态摘要、命令、时间、逐测试结果与原报告SHA256/本地位置。采样source_commit是实施基线而不是干净已提交实现；不将原采样追认为最后提交的全部场景重跑。最后仅移除无用局部名/注释及补历史定位展示，后者有最终console全包回归；无原生行为变化，不重复controlled/live。

## PR #238 质量审计、修复与合并前复验

本次审计复现并修复：

- 停止消费循环按host字符串排除非local，却遗漏已注册且支持精确停止的后端；没有worker观察器但仍持有本次owned handle时，也没有使用原handle。现在统一经原adapter/原请求执行，未知只查原请求；不增加远端取消能力。
- 停止回执硬编码OS身份并拒绝已结束Attempt，导致原资源已经确认退出仍无法结算。现在核验完整当前目标、agent、session generation、worktree/branch/base与既有操作格式；资源停止不覆盖succeeded/failed/rejected等业务历史。重启只按原CurrentAttempt/原binding查缺席，不按新PID或标题kill。
- 过早移除worker观察项会丢弃排队中的原输出/ack；现在保留原观察流到排空，明确确认的物理停止另存原operation ID/观察时间，不能被迟到业务事实抹掉。派发只计算仍活动/未知的执行；保留资源、排空观察或已退出问题不阻塞无关任务。获胜的冻结业务状态/Delivery仍保留，不以物理停止改写已交付快照。
- 客户端最终答案提交缺少closed/discarded检查，原问题ID和版本不变时仍可能创建外发。新增最后提交门，拒绝已退出Task并保留草稿；不依赖前一轮UI刷新或服务端拒绝来遮盖。

[有效停止失败复现](audit-stop-valid-before/summary.json)保留三处实际失败：支持的非local宿主被跳过、只有owned handle未停止、终态Attempt不能结算。[UI失败复现](audit-ui-before.json)仅摘原混合报告的有效UI失败；先前停止夹具因试图改写原冻结Profile、session generation/policy资源不完整而失败，保留本地，修正夹具后复现，不削弱生产身份检查。

| 必要复验 | 实际结果 |
| --- | --- |
| [停止/原子回执专项](audit-stop-final/summary.json) | 10项全通过；支持/不支持、非OS身份、owned-only、终态Attempt、原绑定重启只读absence、原请求不重发、迟到输出、外部branch/错误格式拒绝及回滚/重放 |
| [相关生命周期回归](audit-regression/summary.json) | 14项全通过；资源/PR保留、未知与迟到、Orca原terminal停止和remote原ack对账保持 |
| [console/CLI/图与DAG规则](audit-console-cli/summary.json) | 130项通过，包含新增最终提交拒绝；只读定位和退役CLI保持 |
| [L1](audit-l1/summary.json) | 12组passed/complete，123.448秒，不改变固定覆盖或期限 |
| [获胜/落选/多合并与迟到交付](audit-delivery/summary.json) | MC6父/子场景通过；最后限定已交付winner业务状态冻结，物理停止单独表达 |
| [受控重启交付](audit-controlled/summary.json)与[清理](audit-controlled/scenario.json) | recovery.waiting-merge-restart通过，原身份不重派、一次Delivery/后继ready，cleanup_complete=true；既定30秒/5分钟/清理期限保持 |

修复不改变adapter stop/release/ack协议，也不引入任何生产Close/Delete，控测不冒充native/live。原生接口继续由既有adapter负责，未知/不支持不假装停止。构建、受影响go vet、文档/diff通过；未跑Full或新的live。

采样范围：audit-l1/controlled在最后补充winner冻结状态限定之前执行；随后MC6定向对该限定复验，构建/vet再次通过。没有将原记录重标为最后提交所有场景重新执行。主分支2d0e811b仅整理验收目录/文档；本批精选记录随其规则迁入acceptance，原日志仍在原本地validation/临时路径，不移动或重写原时点事实。第三批及三批行数按审计后重算，新增最小回归测试净增已如实列出。

额外规划注册静态检查发现registry #49与共享Git ref名称不一致；origin/main 2d0e811b的49.json与本分支一致，本PR没有修改roadmap/编号脚本。这是现有全仓规划状态问题，未在本任务改写共享编号或外部DAG，不能将该额外检查列为通过。本批构建、测试、导航及副作用检查不依赖该注册项。
