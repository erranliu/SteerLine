# Spec 项目执行权限策略

任务 ID：59；任务名称：项目执行权限策略；目标：`EXECUTION-ISOLATION`。状态：待实施。配套 [Plan](execution-isolation-06-policy.plan.md)。硬前置：[#58](execution-isolation-05-orca.spec.md) 合入，一个实现 PR 交付。

## 目标与范围

把项目授权与 Codex sandbox 实现解耦，local app-server 与本机 Orca Codex 使用同一有效项目策略。遵守[共同约束](execution-isolation-constraints.md)，本轮改变配置行为并实际接通参数、冻结、拒绝和展示；不只增加 DTO，也不借此扩大其他 agent 或 supervised 支持范围。

新增 `execution/policy.go`、`execution/snapshot.go` 与主包 `project_config.go`、`execution_snapshot.go` 的必要部分，权限编译归 `codexworker/permissions.go`。全局 Profile 的顶层 sandbox 不再是授权来源，Windows 实现选项归 `codexworker.Config`。

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

## 参数与实现配置

Codex CLI/app-server 均从有效策略生成文件、网络和非交互审批参数。禁止用默认用户配置、历史批准或参数顺序决定受管权限。所有启动和后续 turn 参数使用同一快照；thread 的权限不能宽于该策略，准备与业务轮也不能不同。

将主干已有的实现字段统一迁到 `codex.windows_sandbox`。Windows 受限 local 缺省 elevated 在摘要前规范化并冻结；workspace/Git 分散根 + unelevated 必须保持领取前拒绝，none/read-only 可显式选择 unelevated。Windows 本机 Orca Codex 要求显式 elevated 或 unelevated，但选择值不代表组合受支持，分散根无法落实时同样拒绝。非 Windows 或 host 写入未使用 Windows 沙箱的场景记录不适用，不注入伪设置。

删除旧顶层 windows_sandbox 和 sandbox 读取，更新当前格式及示例，不保留字段别名。本轮使用同一编译器承接 #57 已迁移的 Windows 默认/冲突检查，不再实现第二份 LocalSandboxArgs。故障不得自动换实现。model/effort 未配置时保留既有默认语义并记录观察范围，不能为了权限编译自行固定模型。

原始 args 中覆盖权限、附加目录、工作目录、受管 model/effort、子命令、配置文件或 profile 注入，以及绕过审批/sandbox 的参数，在副作用前拒绝。覆盖长短、等号和分离值写法；可以采用经过核实的最小安全参数集合，未知参数不能绕过管理。Codex 原生 profile 覆盖本轮拒绝，不保留旧字段别名或旧顶层 sandbox 读取。

静态配置和安全参数检查在任何外部调用前完成；随后可用共同约束允许的固定只读版本/help 查询核对 CLI 能力，不初始化沙箱或启动 worker。不得仅凭 help 出现一个通用 config 参数就认定具体网络/Windows 设置有效，必须绑定已核实版本的原生配置契约或 schema。不支持的版本/路径报告理由，不自动改策略或后端。原生设置证据与工具实际执行证据分别表达，成功编译不能冒充沙箱已就绪。

## 冻结与管理

领取事务核对项目策略摘要、所选 Profile 摘要和已受理的配置版本并冻结有效配置。资源绑定后追加工作区、Git/临时根、实际启动设置与原生身份证据，不在领取前预填未知路径。启动、恢复输入和后续 turn 只使用该快照，损坏或不匹配时阻止副作用。

本任务与 #60 都扩展同一个执行配置快照和组合摘要：后合入方基于前者实际结构增加字段，不能各保存一份完整快照、只验自己的摘要或丢掉对方的版本拒绝。权限摘要不能代替完整配置摘要，实际资源绑定继续通过现有原子附件提交。

项目权限来自中心显式加载/受理的配置；仓库修改、worker 输出或运行中读取变化不得自动受理为新的授权。使用已有项目配置加载与服务入口，不新建通用权限编辑器；显式重新加载对新 Execution 生效，已有 Execution 不变。

Query 和既有控制台显示项目要求、执行冻结值、额外资源、实现设置及编译/核验状态，复用 ReadProjectView 同一快照中的执行记录。配置不一致或快照不完整时保留可读信息并阻止新启动，不在展示代码另读数据库补字段。未观察到的字段标未核验，不隐藏在 model/host 名称里。更新 examples 和 execution-profiles 文档，清楚区分项目规则与全局方案。

## 并行与环境就绪交接

可与 [#60](execution-isolation-07-publish-policy.spec.md) 并行包内开发；主包配置、快照、摘要和 UI 接线串行集成，两项各自完整生效，不等 #61 才冻结。

[环境就绪 Spec](worker-environment-readiness.spec.md) 负责原会话工具探针、准备轮结算和宿主恢复。本任务交付受管权限配置、编译、冻结和冲突拒绝，该任务复用这些实现，不另扩顶层 sandbox 字段。准备功能可独立开展，最终权限接线在本任务合入后完成；本轮不实现它的 nonce/收据和宿主恢复状态机。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| PO1 | 显式策略校验、支持/拒绝和 args 冲突覆盖正确；缺失/旧配置在领取及副作用前拒绝 |
| PO2 | 两条 Codex 路径使用同一要求；有/无 model 均受管；保留 local elevated 默认与分散根 unelevated 拒绝，网络/审批/根目录设置有版本依据 |
| PO3 | 领取/绑定两阶段冻结；配置变更不影响原执行，摘要/资源不匹配禁止启动或恢复输入，展示未核验范围 |
| PO4 | 各完成一次 local/本机 Orca 受管工具读写及有效 Git 写目录检查；复用确定性映射核验明确 deny、不扩根及冲突拒绝，保存原身份/设置/真实工具结果 |

PO1–PO3 复用参数、领取和 UI fixture，补最小表驱动策略用例。PO4 在隔离 linked worktree 使用同一类工具检查，可在本轮临时文件与索引上读写，核验有效写入根后清理，不做业务提交/推送或请求额外权限；正向工具成功不能代替设置映射与拒绝检查。不要求全部权限组合。若缺少工具来源证据，先记录缺口，不把成功编译或 assistant 正文当作完成。
