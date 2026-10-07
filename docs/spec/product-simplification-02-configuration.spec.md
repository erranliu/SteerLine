# Spec：固定原生沙箱与自动审批，移除附加需求

目标：`PRODUCT-SIMPLIFICATION`；批次：2。状态：实现、最小验证及自查完成，待独立PR评审，未分配 Task ID；本批[实施Plan](product-simplification-02-configuration.plan.md)、[独立实现PR #236](https://github.com/erranliu/SteerLine/pull/236)。依据：2026-10-07 已接受并审计的取舍 4、5，以及本次确定的单一沙箱与自动审批方案。实现与真实验收见[记录](../validation/product-simplification-02-configuration/README.md)。

参考源码基线：`75474dcece26d807367e106a89a992f01f53fafd`。硬前置：[第一批](product-simplification-01-console.spec.md)合入；实际实施基线记录第一批合入后的提交。遵守第一批“目标与共同边界”，完整保留多后端、远端与多候选，不合并各后端的原生解释。

本 Spec 替代 [项目权限策略](../archive/spec/completed/execution-isolation-06-policy.spec.md)中的自由组合配置，以及 [执行需求注册](../archive/spec/completed/execution-isolation-08-resolution.spec.md)中的可选 `execution_requirements`、认证和工具匹配；原生权限落实、必要能力、执行注册、配置冻结和发布责任继续成立。历史 Spec 不回写。

## 单一执行方案

SteerLine 现阶段只提供一套默认执行方案，以文件修改、构建测试、Git 提交推送和创建 PR 等典型开发需求为目标。Windows Codex 固定使用“unelevated 沙箱 + 原生自动审批”；项目和执行方案不再选择权限模式。项目配置省略 `execution_policy`，采用固定产品默认值；旧 `execution_policy`、权限组合及用户提供的 Codex sandbox/approval 覆盖即使为空也在资源创建前拒绝。配置说明明确默认行为，不增加每任务确认步骤。

| 执行范围 | 固定要求 |
| --- | --- |
| Windows local Codex、Windows Orca 托管 Codex | `sandbox_mode=workspace-write`；`windows.sandbox=unelevated`；`approval_policy=on-request`；`approvals_reviewer=auto_review`；工具网络 allow |
| 其他 agent、非 Windows 或远端 | 保留现有 adapter 的原生配置与支持范围，不套用不适用的 Codex/Windows 参数，不要求证明与 Codex 权限语义等价 |

1. 删除 workspace/full_access、elevated/unelevated、人工/自动审批的产品选择，以及 read_scope/write_scope/git_metadata_write/network/escalation 自由组合。Windows local 与 Orca/Codex 统一生成上表参数，删除 local 对 unelevated 的一律拒绝和 elevated 默认。必要参数由现有 adapter 生成，不新增权限组合编译框架。
2. 普通工具操作在原生沙箱内运行，需要批准时交给原生 reviewer；不再沿用 `escalation deny`，也不把 `never` 当作自动审批。审核拒绝、不可用或启动失败时记录原生原因，使用现有反馈/环境阻断；不做通用审批器、人工审批队列、终端模拟确认、自动修复或 full_access/elevated 回退。获批操作可能跨越沙箱，固定目标、候选身份和发布交付核验继续成立。
3. Windows Codex 从原绑定候选工作区启动，通过现有工作区资源一次配置候选目录及必要 Git/工具根；保留当前临时目录约束和原生保护路径，不整体授权项目目录、共同 Git 目录或兄弟候选总目录。复用已有就绪和启动逻辑，不添加管理员初始化、系统账号/防火墙管理、全局配置改写、ACL 自动修复或通用缓存目录发现。典型开发操作超出常规沙箱时交给原生审核。
4. Orca 的受管命令链透传固定 Codex 参数，首轮、反馈与恢复保留这些设置。其他 agent、非 Windows 和远端沿用现有原生适配、目标连接和回执，不建立跨 agent 等价证明、平台权限矩阵或新的远端权限认证门槛。保留现有缺证阻断和目标身份核验，不把本机测试作为远端可用证明；本批不扩展尚未支持的组合。
5. 全局执行方案继续支持多套位置、agent、模型及推理强度配置，保持各后端现有默认语义。删除用户提供的 Codex 原始 `args`、native profile、Windows sandbox 和审批覆盖；Git/PR provider 的 executable/argv、Orca shell/连接设置仍由原接口负责。复用现有配置冻结、摘要和资源绑定保存生成的启动设置；配置编辑只影响新 Execution，反馈和重启使用原设置，不新建权限快照、证据数据库或认证生命周期。控制台显示该后端实际原生设置及就绪/失败原因；其他 agent 不标注为已经落实 Codex 的 unelevated/auto_review。

本批不处理任意权限组合、管理员安装、所有工具缓存与凭据需求、所有 agent/OS 组合或所有 Git 维护操作。典型需求之外的操作按原生结果成功或阻断，不以补齐特殊情况作为本批交付条件。

### 证据与原生语义

2026-10-07 对 Codex `0.159.2` 的隔离探针已验证 unelevated 文件操作、linked worktree 提交、本机 Git 推送和候选/保护路径边界。该探针使用 `never` 且补齐了夹具用户 ACL，只作为基础能力参考，不代替新的自动审核和 local/Orca 接线验收，不要求修改真实项目 ACL。Git 自动维护的 packed-refs 写入未通过，本批不扩展到全部 Git 维护能力。

Codex 自动审批使用 `on-request + auto_review` 和原生默认 reviewer，不提供自定义审核策略编辑器；自动审核不保证全部放行，也不能批准 Windows UAC。依据：[官方自动审批说明](https://learn.chatgpt.com/docs/sandboxing/auto-review)、[Windows 沙箱说明](https://learn.chatgpt.com/docs/windows/windows-sandbox)。

## 移除附加执行需求

删除项目 `execution_requirements`，包括 capabilities、tools/exact_version、credentials/site；旧字段即使为空也明确拒绝，不静默忽略。删除仅为这套声明存在的注册匹配、工具/凭据版本查询、附加认证载荷、执行时重复评估、持久快照/错误字段和产品展示。

`certify-backend` 命令及仅服务于附加需求报告的文件登记/读取/认证流程退役；底层被必要预检、原生协议或诊断实际消费的版本/能力事实继续由原所有者提供。按消费者证明删除，不把名称含 capability/requirements 的所有类型或注册入口整体清空。

必要准入复用现有后端/agent/协议注册、目标连接及仓库、共同 base、受管启动设置、readiness 与 worker 发布能力检查，不因采用固定方案增加通用权限认证。首轮、反馈和恢复沿用原资源与配置核验。中心/provider 的凭据不自动下发 worker；worker 仍负责 commit/push/create PR，中心仍只核验真实 PR/合并和唯一交付。

Go/Python 等普通项目依赖及业务测试要求由任务 Spec/Plan 与 worker 负责；不再由项目配置表达通用工具版本门槛。准备失败沿现有环境阻断或原会话问题路径表达，不新增通用检查语言、安装器或替代需求框架。删除附加需求不删除现有 mandatory readiness 探针，也不增加额外 SteerLine 准备模型轮次；原生自动审核自身的请求属于固定方案行为。

## 所有权、格式与删除范围

主要所有者：`internal/execution/backend` 的退役权限/附加需求值；`internal/execution` 的现有配置冻结与退役需求评估；`internal/app` 的配置/注册组装；各 `adapters` 的原生启动参数；model/storage 的实际受影响记录；product/console 的方案展示。保留现有固定内建目录和 codec 所有者，不建立第二注册表或统一原生万能配置。

实施 Plan 列出退役字段、方法、生产消费者和需保留的必要检查；不保留旧组合读取、摘要试算、字段别名、空需求路径或转发 wrapper。仅调整实际受影响的配置/codec/快照/数据库格式，复用现有版本机制；入口在外部副作用前拒绝旧开发格式并保留现场，不迁移、双写或回退。更新项目/全局示例、README、执行方案文档和 fixture；文档更新不代表接线已完成。

## 最小验收

| ID | 场景与通过条件 |
| --- | --- |
| PS2-01 | 无权限字段时采用固定方案；旧字段/原始 args/profile/权限覆盖拒绝；Windows local 与 Orca/Codex 均生成 workspace-write + unelevated + on-request + auto_review，无 elevated/full_access/never 回退；其他 agent/OS 不误用 Codex/Windows 参数 |
| PS2-02 | 现有配置冻结保存固定启动设置；修改配置只影响新候选，反馈与当前格式重启仍用原设置和资源；已有摘要/身份不一致检查继续成立，无新增权限证据系统 |
| PS2-03 | 附加需求字段、认证命令和无消费者流程确实删除；无此系统仍因必要权限、readiness、目标或发布能力缺失而拒绝，不下发中心凭据 |
| PS2-04 | 其他 agent/OS/远端的既有注册、原生参数、目标连接和回执路径保留，不误注入 Codex/Windows 参数，不新增权限等价认证；既有缺证拒绝与身份对账保持 |
| PS2-05 | Windows local 与 Orca/Codex 各以一个隔离候选验证典型开发：文件修改、一条项目构建或测试命令、Git 提交推送；确认固定设置生效和未经升级的兄弟候选写入被拒绝。各触发一条有界原生自动审核请求，记录真实决策；Orca 做一次恢复确认设置不丢失。记录是否出现 UAC，无管理员初始化或全量权限场景验收 |

PS2-01–04 使用现有纯规则、配置准入、临时 SQLite 和可控协议测试；审核拒绝/不可用沿既有失败路径，用一个受控回执验证即可。PS2-05 取得新接线和真实自动审核结果；Git 推送可用本机临时远端，PR 创建/交付路径未修改时复用已有测试，不执行完整 PR 链或穷举工具和平台。基础沙箱事实允许复用上述探针。其他 agent 和远端只做受影响路径的定向回归；没有真机时标注未实测，不要求本批补建环境或取得新的权限认证。

实施 Plan 按 [测试指南](../testing.md)和 [E2E 约定](../../tests/e2e/README.md)列最小测试计划，包含级别、组/场景 ID、环境、命令、时点、通过条件及证据位置；一次 L1 加定向回归。PS2-05 复用或登记最小原生场景，不以整组 live 代替；不默认 Full、完整 L2 或全部 live。证据根目录：`docs/validation/product-simplification-02-configuration/`。

## 交付边界

提交同口径净行数和退役消费者清单，注明自由组合、双模式/Windows 选项及附加需求的删除收益，不把原生权限核验全部删除计为精简。仅固定沙箱与审批方案及删除附加需求，不减少执行方案数量、后端/远端代码、多候选、通用必需生命周期接口或 worker 发布职责。本批更新 Spec 不是已完成上述实现和集成验收。
