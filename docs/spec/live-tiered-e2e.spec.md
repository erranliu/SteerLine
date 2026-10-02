# L2/L3 真实执行环境

## 目标与边界

在既有独立 E2E 包和项目所有权边界内加入真正调用模型的本地 Codex、Orca 本地 Codex 环境。固定模型 `gpt-6-luna`、reasoning effort `medium`。使用真实 SteerLine 服务、公开任务创建/领取入口、Git、数据库和原生执行器；L2 在用户授权的 `https://github.com/erranliu/SteerlineTest` 真实 push/create/merge PR；每条链路使用独有的临时 base 分支，不修改 main。L3 使用 loopback Git 和隔离 provider。

## 入口

- L2 默认顺序执行 controlled smoke，再执行 local-codex 和 orca-codex smoke。受控阶段失败（包括清理/证据失败）立即阻断真实阶段；不得预先创建真实会话或发请求。`-env=controlled` 只诊断门槛，不代表完整 L2 通过；L2 禁止绕过门槛直接 live。
- L3 明确选择 `-env=controlled|live` 和 `-suite`，不自动串联两个环境。支持 `-scenario` 精确选择同组同环境的单个或多个场景。未知、重复、零匹配和跨环境选择报错。
- L3 首期包含 delivery 和 recovery 两组。delivery 验证真实实现/发布/合并交付及依赖解锁；recovery 验证等待合并时重启保持原任务、数据库、执行和工作区，不再次发起模型回合，再完成原交付。真实组分别登记两条链路，各测试独立创建、运行和清理。

## 环境与证据

L2 smoke 两条真实链路显式采用完全访问：项目 execution_policy 为 host 读取/host 写入、git_metadata_write=false（host 写入已包含 Git）、network allow、escalation deny，编译为 danger-full-access 与 approval_policy=never。不配置 Windows sandbox 实现、不预检/复制 sandbox 账号、不运行 sandbox canary；无需管理员初始化。原生版本仍固定 0.159.2，冻结记录采用独立的 host-write 权限契约并记录无额外写根；实际 L2 断言核对该契约。其余 workspace 策略不会自动放宽，host 写入 + network deny 继续拒绝。L3 保留当前两种 sandbox 模式，本次只验收 L2 交付 smoke，不宣称它证明 sandbox 隔离。

每个项目持有独立 Codex home，从显式认证来源（默认当前 CODEX_HOME）复制 auth.json；仅 local-codex elevated 模式检查和复制已初始化的 Windows sandbox 两个凭据/标记文件。orca-codex unelevated 模式使用当前用户受限 token，不要求专用账号初始化。写入最小固定配置，不复制历史会话或项目配置。独立 worker 按 SteerLine execution_policy 启动，不继承父级 Codex App 会话的权限模式。GitHub 认证从本机 gh 读取并存于项目临时凭据文件，只以环境引用传递；凭据不进入产物。Orca 使用真实 CLI 创建/查询/删除测试资源；测试适配器为新建终端设置项目环境；当前 Orca 长文本注入若只停在明确的 Pasted Content 输入框，则对原终端完成一次 Enter，不重发文本。转发真实命令及回执，不模拟身份、权限、readiness 或结算。

elevated Windows 管理员初始化属于可复用的机器环境准备，在测试外完成。该模式的测试启动前以无 UI 的凭据解密和既有账号登录验证当前 sandbox 状态；缺失、版本不符或账号失效时在模型请求前失败，不自动提权修复或关闭 sandbox。每次运行继续隔离项目数据和 Codex 会话，复用初始化状态不等于复用业务环境。运行期间账号或系统策略改变可能仍触发原生修复，不能将启动前检查视为禁止原生 UAC 的保证。

提供显式的 PowerShell 初始化脚本；有效状态直接复用，失效状态通过 Windows RunAs 只提升固定版本的原生 sandbox setup 一次，由用户完成管理员授权。绑定提升前的原用户与显式 home，之后在普通权限下验证既有账号。支持仅检查模式，取消、超时和后验失败均报错；不调用模型，不自动重试，不修改认证或删除已有状态。超时不能保证已提升的原生命令被终止，禁止自动重新发起初始化。

local-codex 使用当前 host_service 发布路径；Orca 使用当前 worker 发布路径，通过任务内发布脚本真实 commit/push，L2 用 gh 创建真实 PR，L3 调用 loopback provider。保留正式远端身份。readiness 必须由真实模型执行原生工具完成。原生准备期限 4 分钟（覆盖资源绑定、模型探针与结算），与单次命令期限分开。原终端启动中可通过公开 recheck_environment 重新检查一次；不能更换会话或重发业务任务。

真实测试须同时取得业务闭环、冻结模型/effort 与原生 usage 证据。local 从绑定 thread/turn 的 app-server tokenUsage 通知提取；Orca 从本项目 Codex home 的原生 session JSONL 提取，核对 session、cwd、模型和 effort。累计 usage 取最终值，不逐事件相加；没有 usage 或为零不得通过。记录 input/cached/output/reasoning/total 与来源，数值代表 CLI 报告的 token 使用，不换算价格或推断账单。

失败也保存已取得的 usage、业务状态和资源清单，清理自建服务、终端、worktree 和仓库注册；L2 关闭本次未合并 PR 并删除本次远端分支，保留真实 PR 历史和链接。保留本项目原生日志，排除凭据文件并脱敏认证值。不能清理或无法确认所有权不得通过；禁止静默 skip、自动重发或改用模拟执行器。

## 最小验收

定向验证参数/清单选择、门槛失败不调用真实阶段、usage 身份匹配与累计去重；运行完整 L2 一次，L3 单独运行 recovery 的两条真实链路及受控组。不要求再次运行全量 Go 回归。
