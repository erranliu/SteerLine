# 第一批精简：三窗与右下原地反馈

绑定 [Spec](../../spec/product-simplification-01-console.spec.md) / [Plan](../../spec/product-simplification-01-console.plan.md)，基线 `75474dcece26d807367e106a89a992f01f53fafd`。最终范围按2026-10-07用户修订：保留左列表、右上候选信息、右下运行/内嵌反馈。第二、三批未实施。

## 实现与保留

- `three_pane.go` / `interaction.go`：三窗及窄屏主界面，复用Unicode/多行编辑；Tab列表/选项/输入、Enter换行、F2明确提交、F8完整冻结背景。读详情/最终回复前保存并关闭编辑，不能提交被遮住的草稿。
- `recent_activity.go`、产品RecentActivity读取、SQLite同事务采样：仅当前候选/Attempt最新10条活动，命令事件不占名额，短标题/状态/说明有界；不保存工具全文/meta，不从终端回填。不恢复时间线、翻页或折叠。
- `final_replies.go`、FinalsOnly投影：保留完整业务最终回复和简单历史选择，排除当前/历史准备轮；有界UTF-8正文读完后核验SHA-256，项目/候选/修订/读取代次拒绝串线。
- `preferences.go`：UI记录格式2只存项目绑定、草稿、原请求对账；默认布局重开重置。旧/损坏文件保留并禁止新外发，原请求只查不重发；原子文件及多窗口锁保留。
- 多后端、远端实现、多候选、原生消息/结算/恢复、唯一交付、权限模式和补前置均保留。API仅删除无消费者的Raw/Capabilities/旧活动游标客户端入口，底层诊断/采集契约仍保留；新增RecentActivity有真实UI消费者。

## 验证

| 范围 | 当前证据 |
| --- | --- |
| PS1-01–04及隐形编辑器保护 | [最终定向报告](three-pane-release-no-skip/summary.json)：console/query/client通过 |
| 最新10条、同事务、来源/准备轮隔离 | [SQLite定向报告](three-pane-sqlite/summary.json)：通过 |
| 当前真实app/SQLite/受控worker接线 | [内嵌反馈报告](inline-integration-refreshed/summary.json)：两方案/两候选、切换草稿隔离、同一候选续跑通过；用户动作在未提交前显式刷新/核对，不重发未知请求 |
| PS1-05 Windows原生终端/PTY | [输入捕获](terminal-three-pane/capture.json) / [原始VT记录](terminal-three-pane/native-output.json)：80×24实际终端，中文两行、F8、P1→P2→P1及一次F2后仍绑定candidate-0/decision-0；完整最终回复末尾可滚动到达，测试进程退出PASS |
| L1 | [L1完整报告](l1-release/summary.json)：passed=true/complete=true；`l1-three-pane/`的功能/边界组通过但默认终端fixture skip使总报告不完整，随后改用显式测试标签；此前失败不能替代最终通过 |
| 静态 | 受影响包go vet、当前产品构建及git diff --check通过 |

终端fixture只注入公开product DTOs，不启动模型或发布PR。Windows PTY证明真实终端VT/UTF-8解析与实际程序渲染，不证明物理IME、桌面截图或远端后端原生能力。宽屏三窗、问题版本变化、焦点/窄屏尺寸由定向测试覆盖；没有把Go fake KeyMsg称为该PTY输入证据。

## 初次失败与范围修订

- 两栏初稿的三分钟五候选接线两次耗尽总准备期限，错误为context deadline exceeded；原始`integration/`、`integration-final/`报告保留。五分钟fixture的旧布局接线通过只作历史，最终版本改用最小两候选内嵌接线，本批未放宽既有五候选测试默认期限；随后合并main已存在的五分钟期限和follow-up执行修复。
- 新两候选接线初次在F2前未重新采样原问题，续跑等待超时；`inline-integration/`失败保留。最终测试在原请求尚未提交时模拟明确Esc/刷新/a核对同一草稿后F2；仍拒绝未知结果的新请求重试。
- 首次L1在同时运行多组服务fixture期间，app构造30秒期限超时，integration边界组15分钟超时；报告complete=false。app构造原用例随后单独通过（21.433秒），未修改生产或该期限。最终L1重新按固定入口执行，不删慢用例、不改runner选择或超时来制造通过。
- 旧桌面窗口截图实际被聊天窗口覆盖，未保存为终端截图或验收证据；改用明确隔离PTY，原桌面fixture停止标记已发送。

## 删除与行数

删除 `conversation.go`、`conversation_anchor.go`、`view_conversation.go`、`model_update_conversation.go`的完整阅读/折叠/锚点逻辑；`model_navigation.go`仅剩简单滚动。阅读偏好/正文分页/历史attempt选择字段和旧UI入口删除。原输入、幂等、失败、身份及正文断言迁移或补测；[退役测试映射](retired-tests.json)保留历史名称，不将删除断言当成新实现通过。

[同口径统计](line-summary.json)比较internal和cmd中的Go物理行数（含注释/空行，按_test.go区分，含未变更测试支持代码）；文档和tests/integration不计入该口径。当前实现净减1417行、console实现由6981至5548行；比较基线为主分支69c839be。保留三窗/摘要/内嵌输入的新增代码已计入，不重复计算共享退役代码，不以压缩排版计收益。

## 主分支同步

合并`69c839be`，两处冲突仅在移至product-design的活动/会话说明。保留main的新数据模型版本定位和目录整理，同时保留三窗、RecentActivity与内嵌反馈说明。当前Spec保持原文件名，已完成资料引用更新为archive路径；未恢复退役版本任务或旧格式兼容路径。

合并后[受影响产品/SQLite回归](post-merge/summary.json)、[app准入/只读投影回归](post-merge-app/summary.json)、产品构建、go vet与全库文档链接检查通过。L1报告保留其原始采样提交及工作树；合并后的源码差异另由这些定向证据覆盖，不改写旧报告。汇总见[最终检查](final-checks.json)。

## 合并前质量审计

独立逐项复查修改代码与Spec后，复现并修复四处问题；新增五个定向场景同时覆盖阻断输入可见性：

- 异步拒绝回执到达最终回复/只读详情或其他弹层时，不自动打开被遮住的编辑器；原草稿与回执保留，返回主界面按a可恢复。
- 窄屏只有列表可见时，F2先返回原草稿预览，再次F2才能提交；防止提交不可见答案。
- 新Decision/状态/问题版本出现及答案提交时重置右下滚动，避免新问题/回执继承旧活动阅读位置。
- 右上将当前候选、冻结配置、执行状态、环境阻断和PR排在可见区；小高度编辑时优先使用右侧空间显示问题、输入、预览与提示。

[初始失败复现](audit-before/summary.json)记录前三项真实失败，[窄屏失败复现](audit-narrow-before/summary.json)记录列表焦点F2问题。修复后[console/query/client全包回归](audit-after-final/summary.json)109项全部通过，无skip；[SQLite定向回归](audit-sql/summary.json)4项通过。go vet、产品构建、git diff --check及文档链接检查通过。此次修改只涉及console交互与布局，沿用既有L1/受控接线/PTY证据，没有将旧运行冒充当前新源码验证。第二批Spec的并行工作区修改未纳入审计修复提交。
