# Spec：环境检测按正常发布能力验收

2026-10-30。用户已确认实施；[Plan](../plans/environment-publication-scope.plan.md)。本规格替代[项目准备规格](project-environment-readiness.spec.md)及[原适配规格](project-environment-adaptation.spec.md)中要求检测 Agent 自行查询分支保护、完成审核和合并的条款。项目 × 环境门禁、TTY、完整 L2、三轮预算、原请求及停止封存规则保持。

## 目标与边界

检测 Agent 验证正常交付所需的工具、完整 L2、commit/push、创建并查询原 PR；分支规则核实、必要检查/审核、合并和远端测试资源清理由外部 L4 驱动负责。Runner 不知道环境检测，不扩其授权代理，也不允许改 PATH、凭据、Git 配置或安装工具。

生产 ready 表示项目在该环境已通过执行及 PR 发布能力验收，不承诺 Agent 能自行管理分支保护或合并。外部驱动的合并结果是 L4 的独立证据，不回写 Agent 的封存 result，不新增生产审核阶段、后台合并器或第二份资格状态。产品仍只消费普通 Run/result；外部验证失败使正式 L4 失败，不能篡改原 ready 或原报告。

## 唯一结果与通过条件

环境报告使用 version=2，保留原关联字段、checks、完整 l2、message；pr 仅保存 url/base/head/head_revision/cleanup_completed/outcome_unknown。删除 Agent 报告中的 checks_passed、reviews_passed、merged、merge_commit，禁止将未执行动作伪造通过。head_revision 是原验证提交的完整 SHA，URL/base/head/SHA 必须与 Runner 已核实的原 PR 事实一致，QueryError 为空且仓库、provider 与冻结 publication 目标匹配；PR 查询未知、错绑定、缺 SHA、未完成脚本自有资源清理均保持 unverified，禁止开启下一轮再建 PR。该约束同时覆盖 ready 和原 safePR 重试出口。

同一轮 checks（含实际 TTY）全部通过、完整 L2 全通过、原 PR 发布身份核实且结果已封存、原 Run 停止结算后才 ready。脚本不关闭或删除 Runner 仍持有的 PR/工作区/分支；cleanup_completed 仅表示脚本自身没有未结算的临时进程等资源，PR 交给正常外部交付/测试清理者。未知发布只对账原 PR，不重发。旧报告/状态格式不迁移、不兼容、不回退；当前 SQLite 格式随持久报告变更换代，安装环境不更新。

## 外部真实 L4

本机 Codex、本机 Orca、远端 Orca 共用上述生产判定。本机/远端 Orca 的原 L2 文件取证仍同一公开 repo/terminal 流程，仅 Codex 可直读原工作区。

外部驱动先核对封存报告、原 PR 的仓库/base/head/SHA，再核实项目源码分支实际规则、所需 checks/独立审核；不提供审核批准或绕过保护。之后使用既有 provider 身份合并这个精确 PR，绑定预期 head SHA，并只读核实合并提交。未知合并不重复写，保留原 PR 和资源。结果、原 L2、准入探针、Run 停止、PR 和专属 ref 清理全部核实才记正式 L4 通过。

## 验收

- EPS-A01：version2 仅验发布；未合并 PR 可以通过，旧报告/伪造合并字段拒绝；TTY/L2/身份/未知/清理反例均保持阻断。
- EPS-A02：生产资格使用原 Runner PR 事实校验，不采用外部合并结果补写 result；跨 PR/分支/SHA 及缺事实拒绝。
- EPS-A03：固定执行/生成规则、同源材料、Web 显示和两项目脚本使用同一契约，实际正文差异与前后字节数经独立审核。
- EPS-A04：L4 外部阶段只对原 PR 进行规则核实、合并和清理；缺审核/检查、SHA 变化、未知结果及清理未知明确失败，原报告不变。
- EPS-A05：固定受审 SteerlineTest 提交、本机 Codex profile 和独立 Runner 重跑真实 L4；原失败与补测分别留存。Orca 本机等待 Runner 修复，远端连接未恢复时不重跑或借本机结果宣称通过。
