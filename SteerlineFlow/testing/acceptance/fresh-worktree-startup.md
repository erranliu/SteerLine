# 新建 worktree 启动边界验收

验收对象为 [Spec](../../spec/fresh-worktree-startup.spec.md)。以下均为自动项，入口和触发时机见 [Plan §4](../../plans/fresh-worktree-startup.plan.md#4-回归任务-101-和交付)。Runner 负责工作区和流程证据，SteerLine 负责冻结、准入、派发和项目验收证据。

| 编号 | 行为 | 通过证据 |
| --- | --- | --- |
| FWS-A01 | SteerLine 正常启动任务 101 | 隔离使用 SteerlineTest `9d81d9c2652177faf6354d4e9f9f3aec79535d11` 及任务 101 的配置。证明 `SteerlineFlow/plans/native-101.tests.json` 不存在。原有序 ID 为 `N101-CONTENT`、`N101-SCOPE`、`demo.progress.roundtrip`、`demo.progress.retry`、`demo.progress.cleanup`。就绪候选、原 outbox 解码字节和 Agent 从新工作区实际读取的文件逐字节相同。记录摘要、首次读取及后续动作顺序，独立观察原进度渠道和结果，确认项目未新增安排文件。 |
| FWS-A02 | SteerLine 准入负例 | 固定材料/策略不可读、writer 不符、安排与清单不符、通道不受支持或安排超限分别在 SteerLine 阻断，零新 Run。候选变化或提交响应丢失时核对原 outbox 身份和字节，零盲目重派。 |
| FWS-A03 | 新建成功与原资源异常 | 在本机 Codex、本机 Orca 和受支持的远端 Orca 分别检查固定 commit、创建回执和执行起点。记录生产成功路径的 Git/Orca 调用，确认无创建后的额外启动 HEAD、diff 或 status 探针。目标分支变化且固定对象仍可用时成功，固定对象不可用、路径冲突及回执不符时失败。用测试侧独立读取取证实际起点。 |
| FWS-A04 | 安排在执行端缺失或不符 | 在受控目标分别注入写入失败和错误内容，保留正式通道诊断。Runner 可派发步骤，Agent 在依赖安排的操作前报告阻塞，进度不伪报成功，SteerLine 不将该次记录为通过。通过受控故障、实际读取尝试及后续动作轨迹证明顺序。 |
| FWS-A05 | Workflow 自治与恢复隔离 | 普通 Workflow 自主跨阶段，审计 Workflow 在受支持组合自主审计及修复。创建回执未知、已有工作区及原 Run 恢复单列，核对原有 Git 检查及原资源对账，确认零替代创建、零盲重发。 |

## 2026-11-04 实际结果

报告根为 `C:\SteerlineDebug\fresh-worktree-startup-20261104`。Runner 构建 `0.2.0-dev.fresh-start` 来自 `1df8e26d` 加本次 Runner 源码差异，EXE SHA-256 为 `02a0b68ad6f77a767011db02a1722aec3d28811c9c684d909c737ac585a902cb`。[配套 Runner PR](https://github.com/erranliu/SteerRunner/pull/101)

- FWS-A01 通过。SteerLine 读取任务 101 原提交 `9d81d9c2652177faf6354d4e9f9f3aec79535d11`，受控生成原五项顺序。启动准备采用「已就绪」记录的同一快照，序列化为 111 字节，SHA-256 为 `b9a819296f6f3c6b0e9a3ff9ce4059f89a111a9a7633ed73baa373d0475f6cc4`。Runner 正式 CLI 的受控原生调用读出相同字节。已登录的真实本机 Codex `0.166.0-alpha.6` 在独立 Run `2e3c83b4e2bb2224e6baa30a431a94c2` 实际计算出同一摘要。该轮只验证读取，已确认原 Run 停止和服务结算。证据见 `test-results/actual-101`、`test-results/public-101-retry` 和 `test-results/native-101-retry`。
- FWS-A02 定向覆盖固定规则、writer、定义、方案能力、候选版本和原请求，见 `test-results/owner`。SQLite 比较就绪快照与候选快照，并校验原 outbox 的安排字节。
- FWS-A03、A05 的固定对象、重写、资源绑定及旧 Run 回归见 `diagnostics/runner-targeted.log`。本机与远端 Orca 的真实 smoke 及原生审计交付未签收。
- FWS-A04 受控缺文件用例通过。阻断 result 目录创建后，Runner 仍发起回合，协议夹具记录读取失败并报告阻塞，独立 progress 已初始化。证据见 `test-results/public-101-retry`。
- 首轮公开测试在原结果文件封存通过后遇到 Windows 释放 EXE 的短暂清理冲突，复验通过。首轮真实 Codex 已计算出匹配摘要，停止回执为 unknown，隔离进程已确认退出。补验在原生回合完成后停止原 Run，正式结算通过。保留 `test-results/public-101` 和 `test-results/native-101`。
- SteerLine 正式 L1、L2 通过，见 `test-results/release-L1`、`test-results/release-L2`。早期受控 L2 在生成观察期间达到场景期限，原报告保留于 `test-results/L2`、`test-results/final-L2` 和 `test-results/cache-L2`。未修改主干的 L2 对照也通过，见 `test-results/main-L2`。最终实现把固定文件读取移到基线预检，生成完成后只做已固定输入的快照、大小和能力判断。
- L3 `testing.runtime-progress` 通过。正式启动在无旁置安排的受控项目中保留 outbox 的安排文件种子，原进度通道继续覆盖完成、重跑清零、异常快照、重启和停止。见 `test-results/release-L3`。

## 合入审计修订

writer 和项目策略的暂时读取失败保持材料待准备。下一次刷新重新读取同一固定 commit。两项回归均验证恢复后只发起一次生成并进入就绪。已确认的必需文件缺失、非普通文件和 writer 正文不符沿原无效状态处理。任务 101 原提交复验仍得到原有序五项、111 字节和相同 SHA-256。报告根为 `C:\SteerlineDebug\fresh-worktree-audit-20261104`，见 `owner` 与 `public-101`。正式 L1 和 L2 同样通过。

修复后的配对 Runner 构建 `0.2.0-dev.fresh-audit` 复验通过。真实本机 Codex Run `da64a3da77c9de7d440a924b8271a4b1` 从新 worktree 读出与就绪和启动安排一致的 SHA-256，原 Run 和隔离服务已结算，见 `native-101`。

## 提示词审核

SteerLine 的实施和审计提示将同名 Git 安排替换为读取 `STEERLINE_RESULT_FILE`，包括 Orca 的文件绑定定位、原字节 SHA-256、读取失败时报告阻塞及安排来源优先级。继续读取固定 Plan、清单和执行协议，复用原 writer。模板 377 → 540 字节，渲染样本 484 → 702 字节，组合样本 512 → 730 字节。[实际模板](../../../internal/prompts/testing-execute.md)

Runner 的通用说明把写入规则明确限定为调用任务要求写结果时适用，保留协调写入、同目录原子替换和停止后封存。正文 183 → 174 字节。独立审核复算了两仓字节数、检查最终正文和接线，SteerLine 提示词定向测试通过。
