# 开发首轮提示词精简验收

SteerLine 提交前继续核验固定 Plan、项目测试资料和正式写入器。候选保留冻结测试定义与原文件字节。首轮测试上下文只提供一行安排 SHA-256，Agent 按项目 Plan 调用测试入口。

| 内容 | 原 UTF-8 字节数 | 新 UTF-8 字节数 |
| --- | ---: | ---: |
| 执行模板（含末尾换行） | 540 | 47 |
| 固定上下文黄金样本 | 702 | 95 |
| 原正文与固定上下文组合样本 | 730 | 123 |

独立审核核对了模板实际差异、原正文换行、候选准入和固定材料核验，并复跑提示词测试。候选和提示词定向测试、正式 L1、L2 均通过。定向 L3 testing.runtime-progress 在同一实现基线上通过。上述验证均为合并前结果。

配对的 SteerLine 和 Runner 分别以 2610963f 和 a8284d13 为基线。使用 SteerlineTest PR #771 固定提交 199803191462a128577af246a3ef70d27bc751ed 的真实远端 Orca Run eac715ac29d6d5c78da32c2b9e2f7389 通过。Agent 按 Plan 调用统一入口并报告退出 0，原实施回复已验证。Runner 独立观察到五项实际完成和 retry 的完成、清零、恢复。封存的 111 字节结果与提交的原安排一致。实际首轮为 573 B。

验收在发布阶段开始时停止，隔离服务和会话资源已结算。测试流程的脚本化接入覆盖 Native 101。原始证据位于本次独立目录 native101-scripted，包含 first-prompt.txt、native-evidence.jsonl、steerline-L1/summary.json、steerline-L2/summary.json 和 service-verification.json。
