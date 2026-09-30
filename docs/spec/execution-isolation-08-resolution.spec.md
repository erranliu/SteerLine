# Spec 执行需求与配置注册收口

任务 ID：61；任务名称：执行需求与配置注册收口；目标：`EXECUTION-ISOLATION`。状态：待实施。配套 [Plan](execution-isolation-08-resolution.plan.md)。硬前置：[#59](execution-isolation-06-policy.spec.md)、[#60](execution-isolation-07-publish-policy.spec.md) 合入，一个实现 PR 交付。

## 目标与范围

接通项目必要能力、工具和认证引用的需求匹配，将配置组合与实现构造集中到注册入口，完成执行配置隔离。遵守[共同约束](execution-isolation-constraints.md)。权限与发布责任在前置已经完整生效，本轮不替它们补欠缺的参数或冻结，也不新建执行模式、安装器或插件系统。

新增 `execution/requirements.go`，收敛 `execution_registry.go`、`execution_profiles.go`、`execution_snapshot.go`、`execution_service.go`，删除无消费者的 mode/反馈路由。更新当前代码架构和执行配置说明。

## 项目需求契约

项目可配置 `execution_requirements`，缺省等同空附加需求；worker-flow 的既有必需能力、#59 权限与 #60 publisher 检查始终强制执行，空需求不能绕过它们。

```json
{
  "execution_requirements": {
    "capabilities": ["workspace_inspection"],
    "tools": [{"id": "git"}],
    "credentials": [{"id": "pr.token_env", "site": "service"}]
  }
}
```

| 需求 | 第一版语义 |
| --- | --- |
| `capabilities` | 使用公共已注册能力 ID；required 项必须有绑定目标/agent/协议/版本的 verified 报告，未知 ID、unsupported 或 not_tested 拒绝 |
| `tools` | 支持注册检查器 `git`、`go`、`python`，声明 id 及可选 `exact_version`；省略版本只要求工具存在，不能假定三个工具全部必需 |
| `credentials` | 对象包含 `id` 与 `site=service|target`；检查已注册 provider/agent 的非秘密引用是否可解析，只证明来源存在，不证明登录或远端授权 |

工具检查用固定只读版本查询，Git 使用冻结 Git executable，go/python 使用所选实现固定工具映射或目标 PATH 解析；不能让项目附加任意命令作为检查。exact_version 与工具检查器归一后的版本字符串精确比较，缺少解析器或解析失败明确 unknown，不建设版本范围语言。检查必须在将执行该工具的目标和等价非秘密环境下进行；远端工具不能由中心机器的查询推定。

认证引用至少支持冻结 provider 的 `pr.token_env`（site=service）；命令 provider 不使用 token_env 时不能拿宿主其他登录状态冒充这一引用存在。其他引用只由已有 agent/provider 检查器注册，未知 ID 或不支持 site 拒绝。target 认证必须由目标查询，不能用 service 环境变量代替。示例适用于使用 token_env 的 provider；无此附加要求的项目写空列表，不自动要求所有项目具有某个 token。

认证检查返回 present/missing/unknown 和非秘密来源，unknown/missing 不满足必需项，不保存秘密内容或其摘要。无法在目标查询的需求明确拒绝，不启动隐藏 agent、修 ACL、安装工具或登录。工具存在不证明 worker 沙箱能使用它，权限落实/就绪状态独立展示。

附加 capabilities 消费当前经过认证的报告，不能将编译支持或 mock 运行升级成 verified。worker-flow 现有必需预检与附加认证分别处理：若补充持续会话等语义 ID，绑定已有对应原生证据，缺证则显示未核验，不用 start 推导反馈或把原受管 CLI 能力预检伪装成 sandbox 就绪。不要求全部后端支持一切接口，也不为本轮重新认证全部原生能力。

## 配置与注册

项目拥有仓库、任务、PR provider、ExecutionPolicy、execution.PublicationPolicy 和附加需求；全局 Profile 拥有选择的目标、agent、model/effort 与具体实现配置。Codex 和 Orca Config 由各自包解析/校验；中心只在注册组装处选择具体实现，公共契约不增加各家配置字段。

内建注册键区分 backend、agent 和协议，具体执行目标单独绑定。先做静态配置检查，再做共同约束允许的固定短命只读能力/工具查询，不创建 worker 现场；重复/未知键或不支持组合给出原因，不静默换默认模型、Profile、协议或后端。初版是固定内建组装，不开放用户插件或动态加载代码。

准备、反馈、观察、停止、恢复对账和交付流程消费共同请求/事件/能力。中心可以记录 native 诊断，但业务条件不依赖 `host == local`、原生 method、terminal 文本或 Codex UUID；具体选择留在注册处，各路径差异由报告和类型化回执表达。

## 快照与恢复

组合快照继承 #59 的权限和 #60 的 publisher/provider/remote/工具/认证引用，再加入附加需求及受理项目版本、Profile、实现配置版本与完整摘要；所有 slice/map 必须独立复制。领取事务再核对同一候选摘要，查验后配置变化不能造成 stale claim。工具/认证评估作为带目标/site/版本/时间的非秘密证据保存，不把时效性报告伪装成永久授权。

资源绑定单独保存实际 worktree/原生身份/命令与根目录，配置摘要与资源身份分别校验。Profile 或项目更改只影响新执行；重启构造适配器使用原快照和原资源，不用当前全局文件补造丢失字段。当前检查发现工具/认证丢失时报告新的阻断事实，不能重写历史快照或自动换方案。

## 系列收口

按当前 `worker_mode.go`、`workerflow/mode.go` 和 workerflow 调用点集中注册，删除仅为硬编码实现选择存在的业务路由；保留纯启动协调语义，将 PendingIdentity 等原生 ID 解释归对应适配器。主干已有事务、Runtime 和模式协调收口不重新建设，TaskDelivery、并发校验与历史来源不能因清理消失。

完整检查 import 和生产调用链，列出每类项目规则、共同操作、native 配置/证据和业务写入的所有者。可以保留组装处的具体类型判断，不能把 UI/业务分支换成名称不同的后端枚举来伪装解耦。

沿用统一 ProjectView 与事件 payload 批量读取，配置展示不新增逐条查询或将不同读取版本合并。连接新鲜度在使用时复核，不能凭采样时 alive 或缓存 Ready 放行新动作；最终资格仍由原 Command/领取事务核验。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| ER1 | 需求匹配正确，目标/版本不符、工具/认证未知或缺失拒绝；空附加需求不绕过强制能力/策略 |
| ER2 | 后端构造和原生配置解释集中在注册/实现，公共包与纯规则无中心/平台 I/O 依赖，生产状态/反馈/PR 判定不新增后端分支 |
| ER3 | 完整配置摘要与资源绑定核验、领取竞争、复制隔离和原快照恢复通过；两执行的配置/正文/身份不串线 |
| ER4 | local 与 native 现有接线通过，supervised 不启用；测试内增加一个公共契约 fake 实现无需改业务状态机；无重复实现或失效路由 |

用最小需求表驱动及已有领取/两轮/原生 fixture，运行现有快速入口一次核对最小闭环。无新增启动行为则不重复 #59 真机或人工合并链；mock 新后端仅用于契约证明，不作为生产注册。
