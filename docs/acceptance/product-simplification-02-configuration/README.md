# 第二批：固定原生方案与配置收口

绑定[Spec](../../spec/product-simplification-02-configuration.spec.md)/[Plan](../../spec/product-simplification-02-configuration.plan.md)，基线172cea6c，实施、自查与最小验证完成。保留多后端、远端协议、多候选、必要准入与worker发布职责。下列报告保留实际结果与范围，不以参数断言冒充原生验收。

Windows local和Orca/Codex固定workspace-write、unelevated、on-request、auto_review、network allow。项目严格拒绝execution_policy/execution_requirements，全局Codex input严格拒绝旧args/profile/windows_sandbox和审批覆盖，包括空值。原参数由adapter生成；现有资源封套保存启动设置和精确写根。

版本：SQLite15、服务checkpoint19、FixedExecutionProfile7、builtin-execution-v3、Codex profile/resource codec2；其余全局格式2、工作流worker-flow-v1、Orca/远端原生协议保持。旧格式拒绝并保留现场。

初次定向失败与修复保留在targeted-initial；接线初次失败只因两条旧可控CLI帮助未声明on-request，wiring-initial保留。五候选接线及原输入、结果、资源身份断言已通过，没有因配置精简取消核心候选覆盖。原生第一次使用桌面应用目录的0.160.1被必要版本检查拒绝；另一个已安装CLI的直接可执行文件为0.159.2，后续使用该匹配版本。

native-local记录本机原生开发/审核初次运行：文件修改、项目测试、commit、兄弟目录拒绝及一次真实自动review批准；本机裸库接收端packed-refs.lock写入拒绝导致该场景失败，不能作为推送通过。后续以隔离Git HTTP接收端验证典型开发推送，接收端在父级测试进程运行，worker只做真实Git HTTP传输，不扩大沙箱写根。

## 当前通过证据

| 范围 | 实际结果与证据 |
| --- | --- |
| PS2-01–04配置/冻结/必要检查 | [定向报告](targeted-fixed/summary.json)、[最终源码定向](source-final/summary.json)：通过，无退役native fixture默认skip |
| 原终端/重启及远端保留 | [接线修复报告](wiring-native-fixed/summary.json)、[远端adapter完整回归](remote-fixed/summary.json)：通过 |
| 原生资源设置防篡改 | [资源codec回归](resources-fixed/summary.json)：沙箱/审批变更、缺失、重复参数拒绝；完整身份与历史意图断言保持 |
| Windows local PS2-05 | [原生回执](native-local-http/receipt.json)、[原生事件](native-local-http/native-events.selected.jsonl)、[Go结果](native-local-http-run.jsonl)：文件修改、项目测试、commit/真实HTTP push、兄弟目录拒绝、一次有界自动审核通过 |
| Windows Orca PS2-05 | [原生回执](native-orca-verified/receipt.json)、[原生事件](native-orca-verified/native-events.selected.jsonl)、[Go结果](native-orca-verified-run.jsonl)：相同开发与审核事实，原序列化profile/资源恢复及production recovery port核验同一live terminal成功；已看到原prompt的turn_started，未重发 |
| L1 | [最终主分支集成报告](l1-main-sync/summary.json)：passed=true、complete=true，当前12组全过，execution113.9秒；[优化前固定L1](l1-release/summary.json)也完整通过1159.5秒，两者范围不同。前两次失败如实保留，不能当通过 |
| 静态/构建 | 当前产品构建、受影响包go vet、原生tag编译、差异格式、全库文档链接检查通过 |

原生审核是实际请求与决策，采用on-request/auto_review，未模拟人类菜单、修改ACL或使用elevated/full-access。UAC记录只覆盖本次native输出及未请求管理员初始化/提升启动，不声称全桌面/所有机器状态都不会出现UAC。其他agent和远端只做受影响路径回归，没有补建平台或新增权限认证，也未将本机日志当远端可用证明。PR创建/真实交付未改变，本批未执行完整真实PR链。

## 删除与保留消费者

| 删除的原消费者/所有者 | 当前保留的检查与接线 |
| --- | --- |
| backend/policy.go的读写/网络/escalation自由组合；Config/Profile/Service权限来源与摘要；Normalize、selection和LifecycleSetup的产品策略参数 | Codex adapter单一NativePermissions；Profile envelope/完整digest；现有PolicyResources的精确根/原生版本/生成参数与身份校验；readonly产品摘要显示实际原生设置 |
| backend/requirements.go、execution_requirements.go中的可选capability/tools/exact_version/credential/site匹配；Config/Profile/Execution评估与错误字段 | mandatory版本/协议/目标/仓库/base/readiness与发布能力检查；原生Lifecycle/Probe/会话CapabilityReport、工作区与未知结果原身份对账 |
| app/capability.go、certify-backend CLI/flag；ProbeRequirements/QueryTool目录hook、工具查找/凭据查询；文件认证报告登记读取及host验证端口 | 原生stop adapter准入、原进程handle/target/binding核验、真实退出/receipt确认；源事件能力与SQLite规范采集继续由原所有者消费 |
| Codex用户args/profile/windows_sandbox输入与elevated/完全访问/never映射、local unelevated拒绝 | executable/model/effort与各后端默认语义；Orca shell/argv/连接；Git/PR command argv；worker负责commit/push/create PR，中心凭据不自动传给worker |

仅为退役选择/附加声明/认证文件流程的测试删除；[退役映射](retired-tests.json)列历史用例。配置文件字节重查、冻结防篡改/不串候选、必要能力/发布拒绝、原资源重启/输入/结果/stop确认保留在当前路径。旧native探针专属测试由PS2-05替代，共享native codec/resource helper保留，测试未引入第二状态机或测试专用生产入口。

[同口径行数](line-summary.json)仅统计internal+cmd的Go物理行（含空行和注释，按_test.go分开），不计文档、日志或tests/integration。当前生产实现65554→64307，净减1211行；测试33733→32495，净减1234行，已计入新增PS2安全、冻结及原生验证。收益来自删除自由组合与附加需求，不把共享原生预检计为删除收益，不以移动文件或压缩排版计收益。

## 失败与范围限制

`l1/`的唯一失败为远端旧夹具仍声明退役类型，已修复并独立通过；其他固定组通过。`l1-final/`接线包累计15分钟时到期，末尾用例未完成；没有调整runner期限/覆盖或删掉用例，随后静默隔离重跑固定L1。

Orca初次后台TTY未形成模型轮次，保留input_accepted而不冒充送达，原终端已关闭；后续采用UI挂载与生产原screen资格核验。隔离home的目录信任配置使用实际Windows路径键，未自动回答信任菜单。通用tui-idle等待未识别已经显示的输入区，最终沿产品既有精确资格核验；native journal task_complete才作为轮次结束，工具间短暂空闲不能代替。恢复夹具缺少Task ID时被严格生产校验拒绝，补齐夹具身份后通过；未降低生产校验。

原始完整native journal保留本地；提交的selected JSONL排除system/developer/base instructions、reasoning、账号标识/限额，保留此新会话的原身份、固定上下文、tool input/output、自动审核与完成事实。筛选不修改决策或生成成功事件。Windows local的本机裸库packed-refs接收端失败单独保留；典型HTTP push通过不代表所有Git维护操作可用。

本次native创建的Orca worktree按原回执ID、路径、repo核实后通过CLI关闭所有本次terminal并移除；[清理结果](orca-cleanup.json)记录已核实七个现场，另一个早期失败现场通过同一范围核验单独清理。原生复制的认证文件已删除；没有删除用户原有仓库/终端、修改全局配置或进行管理员初始化。

## 主分支资料同步

同步ae32f300的验证资料清理；保留其临时输出默认值、历史固定Git链接与Orca testdata。旧格式拒绝测试从固定172cea6c快照读取两份历史JSON，仅用于确认旧格式不受理，产品没有旧版读取路径。同步后历史格式拒绝及runner/Orca样本定向通过，产品重新构建、go vet、tag编译与文档链接通过；不把先前L1报告改写为合并后运行。两处文档冲突采用第二批行为并接上主分支的新导航，没有还原已清理资料。

提交范围只保留当前通过报告摘要、当前原生回执及selected事件、净行数与退役映射；raw、重复失败与完整日志仍保留本地（受主分支validation目录ignore控制），不重建历史产物目录。L1、原生及定向摘要保留原采样源码/工作树，源码与环境必须匹配方可复用。行数比较使用最终同步后的main 6c16b8dc为基线，不将其他PR的控制台/runner变化计为本批收益。

当前提交的runner报告为精选原始摘要：保留实际源码/环境、命令、分配场景、逐测试结果与通过状态；只省略大段工作区状态及重复慢测试列表。原报告SHA256与本地留存位置记录在evidence_selection，未修改历史运行来源或结果。

最终集成6c16b8dc：保留最新三窗交互、README聚合导航、测试并行与精简范围；退役固定测试按原位置替换为PS2必要能力、不可变资源根/冻结身份与固定原生方案场景。runner和console包级测试通过（12.008/1.285秒），当前L1全部12组通过（准备0.608秒、执行113.919秒）；产品构建/go vet/文档链接及diff通过。剔除与本批无关的空白/import变更，未改控制台产品交互。行数重新按6c16b8dc计算；原生和长链定向证据保持原采样来源，没有重标为本次集成后的live运行。
