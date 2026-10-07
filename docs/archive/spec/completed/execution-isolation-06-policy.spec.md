# Spec 项目执行权限策略

任务 ID：59；任务名称：项目执行权限策略；目标：`EXECUTION-ISOLATION`。状态：已合入 PR #172；remote 成功场景仍受 #59 权限证据门槛限制。配套 [Plan](execution-isolation-06-policy.plan.md)。硬前置：[#58](execution-isolation-05-orca.spec.md) 合入，一个实现 PR 交付。

## 目标与范围

把项目授权与 Codex sandbox 实现解耦，local app-server 与本机 Orca Codex 使用同一有效项目策略。遵守[共同约束](execution-isolation-constraints.md)，本轮改变配置行为并实际接通参数、冻结、拒绝和展示；不只增加 DTO，也不借此扩大其他 agent 或 supervised 支持范围。

新增 execution/policy.go 的纯值 ExecutionPolicy 及校验，contracts 冻结记录引用它；execution/snapshot.go 如需要只接最小纯输入，不导入完整业务记录。主包 project_config/execution_snapshot 负责组合/摘要，Codex 层编译权限。全局顶层 sandbox 不再是授权来源，Windows 解释归实现。

现有固定 Profile、业务执行记录和环境证据归 contracts，本轮在其上扩字段并引用底层纯策略，严格解码/摘要/格式拒绝保留，不再定义同名完整快照。依赖固定为 contracts → execution；execution 不读 contracts/query/sqlite，记录组合归中心，不引入平台反向依赖。

## 项目配置契约

项目文件必须显式配置 `execution_policy`，不根据 host、Codex profile 或用户配置补造权限。第一版结构如下：

```json
{
  "execution_policy": {
    "filesystem": {
      "read_scope": "host",
      "write_scope": "workspace",
      "git_metadata_write": true
    },
    "network": "allow",
    "escalation": "deny"
  }
}
```

| 字段 | 第一版固定语义 |
| --- | --- |
| `read_scope` | 只接受 `host`：在目标 OS/工具既有权限下可读取宿主文件；不宣称只读或 workspace 模式限制读取到工作区 |
| `write_scope` | `none`、`workspace`、`host`；分别不授予持久写入、只授予绑定工作区及列明的运行临时根、在目标用户权限下不另限写入范围 |
| `git_metadata_write` | 必填 bool；仅 `workspace` 可为 true，授权 #55 解析的固定 Git 目录；其他模式要求 false |
| `network` | `allow` 或 `deny`，表达 worker 工具的访问边界，不涉及模型服务连接或中心/provider 的网络权限 |
| `escalation` | 第一版只接受 `deny`，worker 不请求扩大既定工具权限；不代表禁止宿主在原生入口进行沙箱安装或修复 |

策略描述要求，不保证所有组合可实现。编译器按所选版本和路径给出支持或具体拒绝；例如 host 写入与 network deny 无法同时落实时拒绝，不能映射到全访问后声称网络受限。workspace 临时根必须来自固定目标/工具的必要配置，单独记录并核验；不能把任意宿主目录或整个 SteerLine runtime 作为默认附加根。

本轮必须交付两条 Codex 路径的 `host` 读取、`workspace` 写入、Git 元数据写入、network allow、escalation deny 的实际支持。其余组合只有得到真实映射和必要证据才标为可用；已有其他 agent 若不能落实完整策略，领取前明确不可用，不经 opaque 启动放行。

远程链路已经接通，但原生 worker-start 未证明项目文件/网络/越权策略已受管。本轮不能仅凭 profile.sandbox 或平台成功启动宣称落实；远程适配必须返回对应目标/版本的策略支持与证据，无法落实时针对所请求策略领取前拒绝，保留链路及原身份，不 blanket 禁用 remote、不降级或改成中心执行。既有远程双轮 mock 不作为权限或双机认证。

## 参数与实现配置

Codex CLI/app-server 均从有效策略生成文件、网络和非交互审批参数。禁止用默认用户配置、历史批准或参数顺序决定受管权限。所有启动和后续 turn 参数使用同一快照；thread 的权限不能宽于该策略，准备与业务轮也不能不同。

将主干已有字段迁到 `codex.windows_sandbox`。Windows 受限 local 缺省 elevated 在摘要前规范化并冻结；local 的 workspace/Git 分散根 + unelevated 保持领取前拒绝，none/read-only 可显式选择 unelevated。这一限制只适用于已核实的 local app-server 路径，不能直接套到 Orca TUI。

Windows 本机 Orca Codex 保留显式 elevated/unelevated，当前 unelevated + workspace-write 已有同原 terminal 的工具、收据、准备/业务结算证据；用对应路径/版本的映射和证据判断支持，不能借项目抽象误禁该组合或把它推定为 local 也支持。非 Windows 或 host 写入未使用 Windows 沙箱时记录不适用，不注入伪设置。

删除旧顶层 windows_sandbox 和 sandbox 读取，更新当前格式及示例，不保留字段别名。本轮使用同一编译器承接 #57 已迁移的 Windows 默认/冲突检查，不再实现第二份 LocalSandboxArgs。故障不得自动换实现。model/effort 未配置时保留既有默认语义并记录观察范围，不能为了权限编译自行固定模型。

本机 Orca 已有的 managed approval=never、strict-config、安全 args、Git 根、排除临时根和绑定转换为项目策略来源；本机 model/effort 可分别省略，远程继续要求显式二者。读取/恢复记录使用原快照/绑定，不用当前配置补造或改写旧收据。远程权限解释归远端协议适配，不注入本机 Windows 设置。

原始 args 中覆盖权限、附加目录、工作目录、受管 model/effort、子命令、配置文件或 profile 注入，以及绕过审批/sandbox 的参数，在副作用前拒绝。覆盖长短、等号和分离值写法；可以采用经过核实的最小安全参数集合，未知参数不能绕过管理。Codex 原生 profile 覆盖本轮拒绝，不保留旧字段别名或旧顶层 sandbox 读取。

静态配置和安全参数检查在任何外部调用前完成；随后可用共同约束允许的固定只读版本/help 查询核对 CLI 能力，不初始化沙箱或启动 worker。不得仅凭 help 出现一个通用 config 参数就认定具体网络/Windows 设置有效，必须绑定已核实版本的原生配置契约或 schema。不支持的版本/路径报告理由，不自动改策略或后端。原生设置证据与工具实际执行证据分别表达，成功编译不能冒充沙箱已就绪。

## 冻结与管理

领取事务核对项目策略摘要、所选 Profile 摘要和已受理的配置版本并冻结有效配置。资源绑定后追加工作区、Git/临时根、实际启动设置与原生身份证据，不在领取前预填未知路径。启动、恢复输入和后续 turn 只使用该快照，损坏或不匹配时阻止副作用。

本任务与 #60 都扩展同一个执行配置快照和组合摘要：后合入方基于前者实际结构增加字段，不能各保存一份完整快照、只验自己的摘要或丢掉对方的版本拒绝。权限摘要不能代替完整配置摘要，实际资源绑定继续通过现有原子附件提交。

项目权限来自中心显式加载/受理的配置；仓库修改、worker 输出或运行中读取变化不得自动受理为新的授权。使用已有项目配置加载与服务入口，不新建通用权限编辑器；显式重新加载对新 Execution 生效，已有 Execution 不变。

Query/控制台从 contracts.ProjectView 同一快照显示要求、冻结值、资源和核验状态；通过现有 query.Service 读端口与 NewQueryService 组合，SQLite 原始记录仍经中心 projectReadRecords 水合，不新增具体存储类型断言或展示端 SQL。配置不一致/部分错误阻止新启动，未观察值标未核验。更新项目/全局配置说明。

## 并行与环境就绪交接

可与 [#60](execution-isolation-07-publish-policy.spec.md) 并行包内开发；主包配置、快照、摘要和 UI 接线串行集成，两项各自完整生效，不等 #61 才冻结。

[环境就绪原范围](worker-environment-readiness.spec.md)已交付受管启动、冻结、原会话探针和宿主恢复，本任务接收其实现，并将配置权威切换为项目策略；不再建设一个 managed launch、nonce/收据或恢复状态机，不把既有就绪能力反向设为等待本任务才能交付。保留准备/业务分流、宿主动作与反馈隔离、重查并发和固定期限。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| PO1 | 显式策略校验、支持/拒绝和 args 冲突覆盖正确；缺失/旧配置在领取及副作用前拒绝 |
| PO2 | 同一项目要求分别编译到两条路径；保留 local 默认/unelevated 拒绝与 Orca 已证实的 unelevated 组合、有/无 model 受管和独立 effort |
| PO3 | 领取/绑定两阶段冻结；配置变更不影响原执行，摘要/资源不匹配禁止启动或恢复输入，展示未核验范围 |
| PO4 | 各完成一次 local/本机 Orca 受管工具读写及有效 Git 写目录检查；复用确定性映射核验明确 deny、不扩根及冲突拒绝，保存原身份/设置/真实工具结果 |

PO1–PO3 复用参数、领取、环境冻结和 UI fixture。PO4 在隔离 linked worktree 使用同类最小检查，不做业务提交/推送或请求额外权限；正向成功不代替映射与拒绝。复用已有原生工具解析/服务闭环材料须证明路径、版本、实际设置与新策略映射等价；不能改写旧收据的摘要或用旧验收直接宣告新策略接线完成，不重复完整 readiness 实验。
