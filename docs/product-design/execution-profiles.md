# 全局执行配置

第一阶段使用本地 Codex和本机Orca。多后端、远端协议和多候选仍保留；远端缺证阻断保持，不以本机验证推断远端支持。

所有项目共享 `<STEERLINE_HOME>/execution.json`，全局文件格式2，每个方案仍有独立host/agent/model/reasoning_effort及实现codec。多套执行方案与默认方案选择保留。项目负责仓库、Git/PR provider、任务与publication_policy，不再提供权限模式或附加需求。

复制[全局示例](../../examples/execution.json)到execution.json，按机器环境修改；项目文件见[项目示例](../../examples/config.json)。Codex profile codec格式2只接受executable、model、reasoning_effort。旧args/profile/windows_sandbox及审批覆盖即使为空也拒绝；Git/PR executable/argv与Orca shell/连接设置继续保留。local的effort省略时仍为high；Orca的model/effort省略时保留原生默认语义。

```json
{
  "version": 2,
  "default_execution_profile": "local-luna",
  "execution_profiles": [{
    "id": "local-luna", "name": "本机 Codex", "host": "local", "agent": "codex",
    "implementation_configs": [{
      "implementation_key": "codex.worker-profile", "codec_version": 2,
      "payload": {"executable": "codex.exe", "model": "gpt-6-luna", "reasoning_effort": "medium"}
    }]
  }]
}
```

## Windows 本地 Codex 沙箱

Windows local及本机Orca托管Codex采用唯一原生方案：workspace-write、windows.sandbox=unelevated、approval_policy=on-request、approvals_reviewer=auto_review、工具网络allow。adapter生成这些设置；全局或项目不能覆盖。Orca/Codex受管命令使用独立原生进程与同一设置，首轮、反馈和恢复保持原绑定。

候选工作区及必要Git对象/refs/logs/自身worktree admin根由现有资源准备一次绑定；隐式TMP/TEMP及slash tmp写入仍排除，不授权项目总目录、共同Git总目录或兄弟候选目录。普通操作在原生沙箱内执行；需要批准时交给原生自动reviewer。审核拒绝、不可用或启动失败沿原反馈/环境阻断报告原因，不增加SteerLine审批器、人工审批队列、自动修复、elevated或full-access回退。自动审核不保证全部放行，也不能批准Windows UAC。

SteerLine不做管理员安装、系统账号/防火墙设置、全局配置改写、ACL修复或通用缓存发现。依据[原生自动审核](https://learn.chatgpt.com/docs/sandboxing/auto-review)与[Windows沙箱](https://learn.chatgpt.com/docs/windows/windows-sandbox)；本机验收见[第二批记录](../acceptance/archive/completed/product-simplification-02-configuration/README.md)。其他agent、非Windows和远端不套用不适用的参数，沿原adapter支持范围及目标/回执检查。

## 项目策略与配置受理

项目省略execution_policy与execution_requirements；两字段即使空对象或null也在资源创建前拒绝。publication_policy仍必须是worker，必要原生版本、协议/目标连接、仓库/base、readiness和发布能力检查仍由原所有者执行。

方案选择沿原入口：控制台s选择后Enter确认首候选，b选择方案新增候选；CLI使用control -action start_task/start_candidate -execution-profile。服务重新核对配置摘要、Task/候选资格与原目标。编辑配置只影响新的Execution；原候选从FixedExecutionProfile及PolicyResources恢复原codec/模型/原生设置/根，不以新配置补造授权。配置与实际工具结果分别展示；阻断保留原问题和草稿，未知结果只对账原请求。

当前原生契约绑定Codex 0.159.2及生成的app-server schema。格式版本由[数据模型](data-model.md#当前格式来源)统一定位；旧开发配置/数据只识别并拒绝，保留现场，不迁移、双写或回退。

## 项目发布责任

项目必须配置 `"publication_policy":{"publisher":"worker"}`。worker 统一负责 commit、push 和创建 PR；中心只查询、核验合并、提交唯一 TaskDelivery，并保留 PR/worktree/分支，由人处置。当前本地 Codex/本机 Orca 使用这个责任边界；保留远端设计沿用同一边界，但不纳入一期运行或验收。全局执行方案不再接受 publication_policy；host_service 和 execution_profile 均明确拒绝。

worker 的执行策略和本身凭据必须允许 Git 推送及 PR 创建；中心的 token_env 只用于查询，不自动下发给 worker。仅有 query 或成功创建会话不能证明发布能力。worker 的成功、URL 和 worker_done 均不代替 provider 的真实 merged 证据。

冻结快照保留 publisher、原 provider/查询命令、Git、单一 fetch/push URL、base 与认证引用。后续轮次、恢复、PR 查询/交付使用原快照；远程目标变化仍拒绝副作用。秘密值不入库或写入产物。当前持久格式明确拒绝旧开发数据，不迁移、回填或回退；版本见[数据模型](data-model.md#当前格式来源)。历史真机报告保留原始证据格式。

环境阻断是执行状态旁路，保留业务问题及本地草稿。阻断时新回答明确拒绝；未产生投递意图的已保存回答作废并保留审计，恢复后重新提交。已有未知投递只核对原回执。控制台 Ctrl+O 打开原终端，Ctrl+E 重新检查原环境；CLI 的 control -action open_terminal/recheck_environment -run-id <RunID> 使用同一精确 blocker/turn/attempt/version 目标。

会话和活动只显示 SQLite 已采集的规范记录。采集缺口显示诊断，原始文件仅供手动排障，不自动补画或补投输入。

## 附加执行需求与注册

execution_requirements、工具exact_version/credential site匹配、认证文件报告和certify-backend命令已退役。普通Go/Python等依赖和测试要求由任务Spec/Plan与worker负责，不建立替代检查语言或安装器。

内建后端/agent/协议目录、必要原生CapabilityReport、会话能力事实、目标连接、readiness、执行/资源摘要及worker发布核验继续存在。删除的是附加声明与文件认证流程；版本探针和native lifecycle仍不能冒充真实工具成功。远端缺少原生执行落实证据时按原原因拒绝，不新增权限等价认证或远端平台门槛。

PR provider 的 close 字段已删除，即使空值也由严格配置入口拒绝；中心只查询合并事实，不关闭业务 PR。测试环境只清理本次自有资源。
