# 三窗主界面控制台体验验证

绑定 [Spec](../../spec/console-primary-workspace.spec.md) / [Plan](../../spec/console-primary-workspace.plan.md)。基线 `172cea6c6530b6693b742324a9c1d4f9aeb17b6d`，分支 `codex/console-primary-workspace`；2026-10-07，Windows。

## 实现与边界

- 三窗共用尺寸计算，左侧参考比例微调为 44%，增加独立待处理列；右上按目标、状态、关键原因、下一步与冻结方案优先显示，普通辅助信息后置。语义颜色/加粗遵守终端能力与 NO_COLOR，TERM=dumb 使用 ASCII 焦点框；失焦选中行、候选标记、选项与输入光标分别表达。
- 方案选择和候选放弃共用右下工作区，Tab 可回列表，切目标取消临时操作；开始锁定 Task/图/实例/控制版本与方案摘要。Enter 只在显式操作且核对信息可见时确认，y 不再确认；停服同样核验可见性。极小窗口不发送不可见操作，长说明提供 F8 原上下文阅读。
- 普通 Enter 到右下或打开问题，F2 唯一提交答案；详情/F8 独占按键，不能执行隐藏操作。草稿保存失败不切模式；拒绝回执不自动重新打开编辑器，用户显式 a 恢复原草稿。当前规范活动/新轮次优先于旧 final 与终态回执。
- u 在右下显示原只读核验摘要，r 按焦点重读报告或项目，版本/实例/连接状态变化标记过期。主界面只读一页 final 索引和最多 4096 字节正文前缀，展示最多 800 字符/6 行；完整正文及历史按 f/[ / ] 阅读并沿用原身份/大小/SHA-256 校验。通用 Summary 和 worker 自报测试不冒充 final 或独立验收事实。
- 产品/数据库/持久格式、服务/worker 协议、真实 PR 交付与恢复机制未变；没有新增配置迁移、主题系统、第二个编辑器、模型摘要或原生平台能力。

## 最终证据

| 范围 | 证据与结论 |
| --- | --- |
| CPW-01–04 与读取/请求回归 | [发布定向报告](targeted-delivery/summary.json)：console/query/client 共 124 项通过，passed/complete=true，无 skip；包括新 `TestCPW*`，覆盖过期方案、模式取消、隐藏操作、草稿保存失败、结果来源、核验过期、宽窄/ANSI/单色/ASCII 及长确认 |
| 真实 app/SQLite/受控 worker 接线 | [接线报告](integration-02/summary.json)：`TestPS1InlineConsoleWiring` 通过；两个候选、中文原地反馈、切换草稿隔离及同一候选新轮次成立，无真实模型调用 |
| CPW-05 当前终端构建 | [捕获](terminal/delivery-capture.json) / [发布 VT 记录](terminal/delivery-native-output.json)：实际 Windows ConPTY，80×24；Tab/Shift+Tab、Enter、F2、Esc、F6/F7、F8、中文两行，阅读中 F2/Enter 不产生控制请求；窄屏第一 F2 返回预览，第二 F2 只提交一次到 candidate-0/decision-0，测试 PASS |
| 固定 L1 | [原始报告](l1-01/summary.json)：passed=false，quick-boundary 集成组累计 15 分钟超时；其余 12 组通过。未完成的 TM04/TM06/TM08 按原场景单独补跑中，不将原报告改写为通过 |
| 静态 | go vet、产品构建、Markdown 链接、任务身份与差异格式检查通过；构建与源文件摘要见[最终检查记录](final-checks.json) |

终端 fixture 只使用公开 DTO 与隔离临时 home，固定 fixture 时钟避免人工键入耗时让固定快照过期；不会冻结生产时钟、修改服务准备期限或模拟终端按键。键盘由 ConPTY 实际输入，Bubble Tea 记录实际 KeyMsg 与完整渲染帧。ANSI 检查只为该子进程设置 TERM/CLICOLOR_FORCE 并清空 NO_COLOR；单色和 dumb 降级由定向测试覆盖，不修改用户系统环境或配置。

PTY 不证明物理 IME、桌面窗口截图、所有终端字体或远端能力；宽屏/短屏及版本竞争按 CPW 定向断言验证。不运行 Full、L2/L3/live，原因见 Plan。接线/L1 各自保留采样时点与工作树；随后显示/只读键盘收口以发布定向回归及当前终端构建覆盖，不改写较早报告的采样源码。

## 初次失败与修正

固定 L1 未完整通过：原集成组在 `TestTM04ExecutionWiring/original_request_ack_and_manual_recovery` 累计超时，TM06/TM08 尚未执行；没有该组已完成用例的断言失败。保留 panic/原日志，追加 `l1-integration-tail/` 定向运行，不更改 runner、选定用例或超时来制造 L1 通过。

- `development-failures.txt`、`targeted-01/`、`targeted-02/` 保留开发失败。旧断言要求“输入焦点”文字、主界面 session、55% 列表与自动完整 final；按新 Spec 更新了展示断言，保留原身份、草稿、请求和完整正文校验。新增待处理列后，列头/历史状态采用适合列宽的显示，修复对应断言；没有删除失败用例或放宽业务门槛。
- `integration-01/` 的失败是仍在匹配已退役的“输入焦点”文字；改为检查明确输入标记后，`integration-02/` 通过。
- 首次终端调用的 PowerShell 测试 flag 未完整引号，后续使用明确字符串参数。`terminal/first-attempt-capture.json` 保留固定 DTO 在真实时钟下过期导致的无输入失败；终端 fixture 改用受控时钟，业务新鲜度/版本规则仍由现有确定性断言验证。
- 早期 ConPTY 未将 urxvt 的 `ESC [12~` 转成 F2，不计为 F2 输入证据；发布记录使用 xterm/VT100 的 `ESC OQ`，捕获中明确包含 F8 阅读期间的 F2、窄屏预览 F2 与显式提交 F2。早期输出可能有工具截断，完整发布记录与捕获是最终终端证据。

删除整页方案选择/确认渲染、确认滚动遗留字段和异步自动重开草稿接线；完整问题/方案/报告仍复用现有只读入口。没有恢复退役的时间线、工具正文或阅读偏好。
