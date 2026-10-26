# Task18 第二轮测试进度 0/6 修复

用户要求修复后连续两次真实复验通过再告知。范围是原登记测试、进度写入/采集/页面、真实发布和资源清理；完整 MagicPotion 专项、人工业务验收及 clean 发布 head 测试资格不由此推导。

## 原始故障与边界

原候选 `1af016d628b355e048bb101b47012557`、Runner Run `596b316d8e877daed36279206054f05a` 的原六项执行报告已独立通过，但页面为 0/6，公开接口诊断“进度字段与本次固定测试安排不一致”。公开 Runner `progress-file` 取回的 closed 原始字节为六个 false 测试字段及一个 publication 对象，版本 2、563 字节、SHA256 `d55158423d8617396f6a753ee034e0dead0e237049d611a11779973b4db9d9d6`。原项目固定 base 没有共用进度写入器，原提示却仅要求使用该工具，缺失时保留未完成；后续非测试步骤也未收到该文件的业务边界。严格解析拒绝混合对象符合契约，不能靠忽略字段或模型 final 填回通过。

本次只通过公开服务短暂查询已停止的原 Run，没有发送 task submit/reply/resume，没有改原工作区文件或报告。第一次诊断脚本的 PowerShell JSON 解析及缺少 stop envelope 失败保留；独立后续驱动完成原字节核验，并按实际 instance 单次 public service stop，确认 not_running / offline resources_settled=true。原业务 Run 始终 stopped。证据根：`C:/SteerlineDebug/wt-507e-progress-0of6/original-progress-public-02/`；原截图及报告仍位于 `C:/SteerlineDebug/wt-507e-task18-retest/round-07/`。

## 修复与验证安排

- 调用方实施/审计提示包含本仓正式 PowerShell 写入器源码，缺少工具的项目先接入原登记入口；只由原非零选择、报告、输入和清理验证决定完成。
- 发布及报告提示保留原发布/回复要求，同时明确测试文件只读。冻结字段、初始化、严格解析、Run 身份和版本守卫不变。
- 定向回归执行内联源码，覆盖项目无工具、重跑 false、其它字段保留和 publication 拒绝，并验证实施/发布/报告权限隔离及原用户提示保留。
- 固定 L1/L2、两次真实远端复验和清理尚待完成；当前连续通过 0，不把原 Runner 链路两次通过或已有报告计入本次。

关联：[Spec](../../spec/runtime-test-progress.spec.md)、[Plan](../../plans/runtime-test-progress.plan.md)、[接入说明](../../../docs/integration/runtime-progress.md)。
