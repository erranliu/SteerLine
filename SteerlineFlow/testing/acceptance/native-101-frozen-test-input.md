# 任务 101 候选测试安排

2026-11-04，安装版 SteerLine `a589f618` 的 SteerlineTest 任务 101 在固定提交 `9d81d9c2652177faf6354d4e9f9f3aec79535d11` 显示「测试安排已就绪」。项目库记录了五项测试，固定 Git 树中没有 `SteerlineFlow/plans/native-101.tests.json`，启动在预留候选前失败。归档中的两项安排与当前五项不同。

只读核对原生成记录 `09b9674fd4658595339e4c6f2960796d` 得到 `N101-CONTENT`、`N101-SCOPE`、`demo.progress.roundtrip`、`demo.progress.retry`、`demo.progress.cleanup`。相同顺序的候选文件为 111 字节，SHA-256 为 `b9a819296f6f3c6b0e9a3ff9ce4059f89a111a9a7633ed73baa373d0475f6cc4`。

候选从原快照导出按序排列的 ID，随原请求持久化。SteerLine 在预留时校验文件字节、快照与原生成定义。Runner 在本机或 Orca 原工作区准备专用文件，在依赖该文件的实施和审计输入派发前核对原始字节。文件缺失或改变时阻断，恢复沿用原请求。结果文件的输出模式与测试进度文件仍使用各自原契约。

## 提示词变更

- 调用方正文由「先读取本次固定提交的 Plan、同名测试安排、项目文件和执行协议」改为「只读 `STEERLINE_RESULT_FILE` 指向的测试安排（Orca 按 Runner file bindings 定位），核对原文件 SHA-256；不可读或不符时停止并报告。执行协议中的安排来源以此文件为准」。随后仍读取固定提交的 Plan、项目文件和执行协议。去掉旧定义摘要，保留调用方快照标识和原测试入口进度规则。[模板](../../../internal/prompts/testing-execute.md)
- 模板 377 → 540 UTF-8 字节。渲染样本 484 → 702 字节，含原用户正文的组合样本 512 → 730 字节。增量用于定位本机和 Orca 的实际文件、校验文件原始字节并明确安排来源。完整执行协议与正式写入器正文未改。
- Runner 必读模式以 `STEERLINE_RESULT_FILE: frozen caller input for this Run. Read it without modifying it.` 取代同一位置的输出写入指令，183 → 86 UTF-8 字节。普通输出模式继续按原指令工作。

## 验收

| 验收项 | 环境与通过条件 |
| --- | --- |
| 101-A1 | 隔离 Git 与受控生成。远端固定树中没有旁置安排，生成后材料就绪，候选准备得到相同五项及顺序，安排正文不进入提示词。 |
| 101-A2 | 隔离 SQLite。预留拒绝缺少必读输入、选错模式或替换字节的请求，原请求重放保留相同文件种子。 |
| 101-A3 | Runner 定向。缺文件及初始化后字节改变时没有原生业务输入，匹配时发送一次；必读输入不进入结果收集。 |
| 101-A4 | 隔离本机 Runner 实际 EXE 与原生协议夹具。读取工作区里本轮文件，核对五项原始字节，核验任务和服务停止。 |

验证报告根为 `C:/SteerlineDebug/native-101-frozen-input-20261104/`。该场景验证启动材料和 Runner 派发，真实模型执行与 PR 另行记录。

101-A1 和 101-A2 通过，见 `diagnostics/steerline-owner/summary.json`。101-A3 通过，见 `diagnostics/runner-required-input.log`。101-A4 通过，见 `diagnostics/runner-public-101-final.log`。真实 Runner EXE SHA-256 为 `87ca57e80aab4dba8153373eec9756d19fa25db1fce5a61a2a82b61b49b47d29`。公开测试在受控原生回合期间从宿主读取工作区中的实际文件，同时复验原结果输出模式和服务结算。实际正文与字节数已由独立审阅者复算并核对接线。

正式 L1 和 L2 通过，报告位于 `test-results/L1/final/` 和 `test-results/L2/final/`。首轮分别发现测试夹具的结果输入类型和模拟 Runner 能力声明未更新，修正后在新目录复验。真实 Runner EXE 的首轮公开测试因旧夹具缺少显式开发权限而被拒绝，补齐权限后在独立现场复验。

L3 `testing.runtime-progress` 通过，报告位于 `test-results/L3/runtime-progress-final/`。首轮因夹具缺少项目环境准备被门禁拒绝，补上既有公开准备流程后复验。无旁置文件的候选接受原生成选择，受控进度经过完成、重跑清零、异常快照、重启和停止后仍按原候选关联。
