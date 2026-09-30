# Spec 项目发布责任契约

任务 ID：60；任务名称：项目发布责任契约；目标：`EXECUTION-ISOLATION`。状态：待实施。配套 [Plan](execution-isolation-07-publish-policy.plan.md)。硬前置：[#58](execution-isolation-05-orca.spec.md) 合入，一个实现 PR 交付。

## 目标与范围

由项目明确选择发布责任，固定进 Execution 并用于指令与运行路径，移除 `localPublish`/host 自动决定责任的接线。遵守[共同约束](execution-isolation-constraints.md)，复用 #56 发布请求/provider 和 #57/#58 后端能力。

新增公共纯值 `execution/publication.go` 中的 `execution.PublicationPolicy`，由 `publication/policy.go` 做发布能力匹配；接通项目解析、领取快照、提示词、发布受理和展示。workerflow 不导入含 I/O 的 publication 包。本任务不要求尚无实现的发布组合可用，也不新增宿主远端发布、凭据下发或 PR 合并能力。

## 配置与支持矩阵

项目必须显式配置 `publication_policy`，例如：

```json
{
  "publication_policy": {
    "publisher": "host_service"
  }
}
```

`publisher` 只接受 `worker` 或 `host_service`，覆盖提交、推送与建 PR 三项责任。worker 模式由原 worker 完成并报告候选，中心查询真实身份；host_service 模式由 worker 在原轮次明确请求，中心受理固定请求并执行宿主发布。两者的最终交付都由中心核实真实 merged 状态并提交唯一 TaskDelivery。

| 路径与 provider | `worker` | `host_service` |
| --- | --- | --- |
| local Codex app-server + 现有 GitHub command 发布实现 | 本期拒绝 | 支持现有精确发布能力 |
| local + 其他 provider | 本期拒绝 | 未有对应发布实现则拒绝 |
| 本机 Orca native + 可核验 PR 的现有 provider | 支持现有 worker 发布能力 | 本期拒绝 |
| supervised 或其他尚不可启动组合 | 保持不可用 | 保持不可用 |

只把当前能力显式化，不能以有 query 方法推断有 create 能力。新增支持组合需另有实现及证据，不在本任务用通用命令或 fallback 补齐。同一项目换 Profile 后继续用原 publisher，匹配失败在领取前拒绝并显示原因，不自动改成另一模式。

本任务与 #59 无硬依赖：不支持的权限/网络组合仍由当前有效权限边界检查，不因发布策略放宽 sandbox 或给 worker 宿主认证。后合入的 #59 会与 publisher 合并校验，不能仅检查其中一个。

## 指令与执行

`workerflow/prompt.go` 使用固定任务契约和 execution.PublicationPolicy 生成共同工作指令。提交/发布责任由 policy 表达，Orca Run/Task/Dispatch 等说明由平台层单独附加，不再让纯提示构造按 host 判断协议。固定 Spec/Plan 引用与用户反馈语义保持。

host_service 继续要求绑定原 Execution/当前已结束轮次的显式发布请求，核验 workspace、分支、base、HEAD/index-tree 后保存意图。非当前轮 marker、assistant 普通正文和任意 URL 不能触发宿主 commit/push/create。worker 模式不受理宿主发布 marker，不把 worker 成功或链接当合并证据。

宿主发布原有 staged/unstaged 等条件只适用于该受控发布路径，不能扩展为通用任务完成门槛。两种模式共用精确 PR 身份核验、多候选/关闭未合并处理和真实合并交付，不重复建另一套结项流程。

## 冻结与并行

领取时冻结 publisher、发布及核验所用 provider 的 kind/base_url/repository/query/create/close 非秘密配置、Git remote 名与实际 URL、目标分支、工具和认证引用，并纳入完整组合摘要。认证秘密内容不入快照，可以沿原固定引用轮换；不能把当前 Config.PR 或服务共享 s.prs 用于不同冻结 provider 的执行。恢复与后续轮次按原快照构造 provider 和 Git 操作，目标事实不匹配时阻止副作用。

URL 或命令参数包含明文认证信息时在冻结前拒绝并脱敏诊断，认证改用既有非秘密引用；不能先保存带 token 的 remote URL 再隐藏 UI。冻结命令描述和引用，不复制凭据内容；SSH 的普通用户/主机身份与认证秘密分开处理。

修改项目文件或换默认 Profile 不改变已有执行；缺失/损坏/旧配置或不支持组合在副作用前拒绝，更新 examples 和既有配置说明。上述发布上下文必须本轮接通，不能等 #61 才防止 PR 查询或推送目标漂移。

Query/控制台从 ReadProjectView 同一快照显示所选 publisher、有效组合和执行冻结值，不另拼当前 provider 查询结果替换固定配置。与 [#59](execution-isolation-06-policy.spec.md) 可并行包内开发；共享配置、快照、prompt/UI 文件串行集成。两项扩展同一个 Execution 配置快照，后合入方保留全部字段和完整摘要校验，不能留两份完整快照或在 #61 再合并。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| PP1 | 必填 publisher、支持矩阵和启动拒绝生效；换 Profile 不自动改变责任 |
| PP2 | 提示由公共纯 policy 生成，无传递 I/O 依赖；worker/宿主各按责任运行，旧/错轮 marker 不触发发布 |
| PP3 | publisher/provider/remote/工具与引用冻结且可展示，配置变化/重启不重定向；未知发布只对账原请求 |
| PP4 | 两种路径共用精确 PR/真实合并和唯一交付；查询失败、closed-unmerged 不误结项 |

用既有 local 发布、native PR、领取与 provider fixture 补一组策略表驱动检查；不要求真实推送或人工合并，不扩大发布矩阵。
