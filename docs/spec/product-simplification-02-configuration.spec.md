# Spec：收窄执行权限配置与移除附加需求

目标：`PRODUCT-SIMPLIFICATION`；批次：2。状态：待实施，未分配 Task ID；本批一份实施 Plan、一个独立实现 PR。依据：2026-10-07 已接受并审计的取舍 4、5。

参考源码基线：`75474dcece26d807367e106a89a992f01f53fafd`。硬前置：[第一批](product-simplification-01-console.spec.md)合入；实际实施基线记录第一批合入后的提交。遵守第一批“目标与共同边界”，完整保留多后端、远端与多候选，不合并各后端的原生解释。

本 Spec 替代 [项目权限策略](../archive/spec/completed/execution-isolation-06-policy.spec.md)中的自由组合配置，以及 [执行需求注册](../archive/spec/completed/execution-isolation-08-resolution.spec.md)中的可选 `execution_requirements`、认证和工具匹配；原生权限落实、必要能力、执行注册、配置冻结和发布责任继续成立。历史 Spec 不回写。

## 权限模式

项目仍显式授权权限，只接受以下 `execution_policy` 配置；缺失、null、未知模式或混用旧字段在资源创建前拒绝，不默认扩大权限。

```json
{"execution_policy":{"mode":"workspace"}}
```

| 模式 | 固定要求 |
| --- | --- |
| `workspace` | 目标用户权限下读取宿主；只向原绑定工作区及经过核验的必要 Git 元数据/工具根授予写入；工具网络 allow；escalation deny |
| `full_access` | 目标用户权限下不另限工具文件写入范围；工具网络 allow；escalation deny；显式选择，不作为 workspace 不可用时的回退 |

1. 删除项目对 read_scope/write_scope/git_metadata_write/network/escalation 的逐字段自由组合，不再提供 none/read-only、禁网或其他组合。模式表达要求，不保证每个后端/OS/版本都支持。内部由一个规范化函数展开为必要纯权限值用于适配/资源核验；用户模式及实际值相符，不保存两份可独立修改的权限权威。
2. 按所选后端、目标 OS、agent 和版本编译模式。当前已验证的 Windows local workspace 使用 elevated；本机 Orca workspace 保留其已验证的 elevated/unelevated 两种原生选项，不把 local 限制套到 Orca。local 省略时规范化 elevated，Orca workspace 继续要求显式选项；full_access 不接收 Windows sandbox 设置。非 Windows 不注入本机 Windows 参数，缺目标证据明确拒绝。
3. 远端模式由远端适配/协议解释，核验实际目标和版本上的设置及落实证据；中心不能用本机编译、工具查询或兄弟候选报告替代。当前远端缺证拒绝继续成立，不增加 blanket remote 禁用、不切换目标，也不以本批完成宣称远端权限缺口已解决。
4. 全局执行方案继续支持多套位置、agent、模型及推理强度配置，保持各后端现有默认模型/effort语义和独立选择。取消用户提供的 Codex 原始 `args` 与 native profile 覆盖；必要平台参数由 adapter 生成。Git/PR command provider 的固定 executable/argv 以及 Orca shell/连接设置仍是对应接口配置，不按 Codex args 一并删除，也不允许它们绕过受管 agent 权限。
5. 领取冻结所选 mode、规范权限、方案/完整配置摘要和原目标；资源绑定再保存实际工作区/Git根和原生证据。准备、首轮、反馈与恢复都使用原快照；配置编辑只影响新 Execution，不能改变已运行候选。控制台显示所选模式、实际沙箱和落实/就绪状态，而不是重新开放权限组合编辑器。

## 移除附加执行需求

删除项目 `execution_requirements`，包括 capabilities、tools/exact_version、credentials/site；旧字段即使为空也明确拒绝，不静默忽略。删除仅为这套声明存在的注册匹配、工具/凭据版本查询、附加认证载荷、执行时重复评估、持久快照/错误字段和产品展示。

`certify-backend` 命令及仅服务于附加需求报告的文件登记/读取/认证流程退役；底层被必要预检、原生协议或诊断实际消费的版本/能力事实继续由原所有者提供。按消费者证明删除，不把名称含 capability/requirements 的所有类型或注册入口整体清空。

必要准入仍检查后端/agent/协议注册、目标连接及仓库、共同 base、受管参数、实际权限、环境 readiness 与 worker 发布能力。首轮、反馈和恢复继续核验原资源与冻结授权。中心/provider 的凭据不自动下发 worker；worker 仍负责 commit/push/create PR，中心仍只核验真实 PR/合并和唯一交付。

Go/Python 等普通项目依赖及业务测试要求由任务 Spec/Plan 与 worker 负责；不再由项目配置表达通用工具版本门槛。准备失败沿现有环境阻断或原会话问题路径表达，不新增通用检查语言、安装器或替代需求框架。删除附加需求不删除现有 mandatory readiness 探针，也不增加额外模型轮次作为配置简化代价。

## 所有权、格式与删除范围

主要所有者：`internal/execution/backend` 的纯权限/附加需求值；`internal/execution` 的配置冻结与需求评估；`internal/app` 的配置/注册组装；各 `adapters` 的原生权限解释；model/storage 的受影响记录；product/console 的模式展示。保留现有固定内建目录和 codec 所有者，不建立第二注册表或统一原生万能配置。

实施 Plan 列出退役字段、方法、生产消费者和需要保留的必要证据；规范化后不保留旧组合读取、摘要试算、字段别名、空需求路径或转发 wrapper。升级实际受影响的配置/codec/快照/数据库版本，入口在外部副作用前拒绝旧开发格式并保留现场；不迁移、双写或回退。更新项目/全局示例、README、执行方案文档和测试 fixture；不要把仅写文档的新模式当成已接通。

## 最小验收

| ID | 场景与通过条件 |
| --- | --- |
| PS2-01 | 两种模式及各后端实际映射正确；缺失/旧字段/原始 args/未知模式拒绝；local 与 Orca 的 Windows 边界分别成立，不隐式扩大权限 |
| PS2-02 | 原候选冻结 mode/实际权限/资源；修改全局或项目配置只影响新候选，反馈与当前格式重启仍用原值；摘要/资源不一致拒绝 |
| PS2-03 | 附加需求字段、认证命令和无消费者流程确实删除；无此系统仍因必要权限、readiness、目标或发布能力缺失而拒绝，不下发中心凭据 |
| PS2-04 | 远端协议/注册路径保留，缺原目标证据明确拒绝；不能用本机检查或其他候选报告放行，协议回执身份与只读对账保持 |
| PS2-05 | local 与本机 Orca 各验证一个实际受影响的模式启动/工具调用及冻结值，证明生成参数生效；不为本批穷举模式组合或执行完整 PR 链 |

PS2-01–04使用现有纯规则、配置准入、临时 SQLite 和可控协议测试；PS2-05选择隔离目标最小原生证据，当前原生映射完全未改且可证明证据等价的部分允许复用，不能复用旧摘要冒充新接线。远端无可用真机环境时记录缺口，以受控协议/拒绝验证保留路径，不宣称远端认证完成。

实施 Plan 按 [测试指南](../testing.md)和 [E2E 约定](../../tests/e2e/README.md)列最小测试计划，包含级别、组/场景 ID、环境、命令、时点、通过条件及证据位置；一次 L1 加定向回归。现有 L2/L3 场景没有覆盖所选权限模式时，不用整组 live 代替 PS2-05，按约定登记必要场景或复用明确的独立原生验收入口。不默认 Full、完整 L2 或全部 live。证据根目录：`docs/validation/product-simplification-02-configuration/`。

## 交付边界

提交同口径净行数和退役消费者清单；权限映射本已受限，不虚报大幅收益。仅收窄权限配置及删除附加需求，不减少方案数量、后端/远端代码、多候选、通用必需生命周期接口或 worker 发布职责。
