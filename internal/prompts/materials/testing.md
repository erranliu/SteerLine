SteerLine 固定测试安排与进度协议：
安排由 Plan 同名 .tests.json 和同版本完整测试清单提供。
调用方快照标识用于关联内部冻结安排，由调用方核验；不对应工作区文件或进度文件摘要。执行方只核对固定 Git 文件中的安排、完整定义与 note，按进度协议处理布尔字段。
从本次固定工作区的 Plan、SteerlineFlow/testing.md 和 SteerlineFlow/testing/test-list.json 读取上述 ID 的完整定义（含 note），按列表顺序执行；不得重新影响分析选项或改变选择。规范与 Plan 必跑要求取并集。
测试安排已固定。仅按 Plan 执行登记入口，不自动增加或修改测试。运行时环境变量 STEERLINE_PROGRESS_FILE 指向本次唯一测试进度文件，只允许固定安排声明的 test.<测试ID> 布尔字段；不得加入 publication、PR、报告或其它字段，也不得将它用于最终回复。开始或重跑先将对应字段置 false，完整通过（实际选择非零、无失败/意外跳过且输入和清理核验通过）才置 true；失败、跳过、未完成不得置 true。并行写入须加锁合并并原子替换，不覆盖其它字段。原始日志及报告仍按项目入口保存，不要用 Agent 自述或单独一个命令 exit 0 替代入口的完整核验。执行前逐项核对固定安排中每个测试 ID 与其真实入口的进度接线；写入器文件已存在或只有部分入口已接线，都不代表其它 ID 已汇报。未接线的原入口先补齐开始/重跑清零及完整成功分支的写入，再实际执行该入口；按它在固定安排中对应的 ID 汇报，不遗漏定向或聚合入口。不修改固定测试定义、选择或通过要求。保留登记入口与测试语义；允许按入口文档准备并传入必需的环境、输入、输出定位参数，不得借参数缩窄测试选择。进度文件缺失或接入失败应明确报告并保留未完成，不能读取既往报告后补写或事后手填 true。正式 writer：tools/testing/RuntimeProgress.ps1。逐 ID 核对真实入口接线。


正式 PowerShell 写入器（支持 Windows PowerShell 5.1）为同版本 tools/testing/RuntimeProgress.ps1，由原登记测试入口 dot-source。只有逐项确认入口已覆盖本轮对应的全部 ID 才直接沿用；仅检查文件存在或 dot-source 语句不足以证明接线完整。入口开始调用 Set-RuntimeProgress -Name 'test.<原ID>' -Completed $false -IfDeclared；仅在该入口原始报告、实际非零选择、输入和清理全部核验通过后调用同函数 -Completed $true -IfDeclared。保留真实入口接线作为项目改动；没有注入变量时函数无操作。非 PowerShell 入口可实现同一协议，不能让模型事后代填。

非执行步骤：STEERLINE_PROGRESS_FILE 是本轮测试入口专用的 test.<测试ID> 布尔对象。本步骤不得写入、覆盖或追加此文件，不得加入 publication、PR 或报告字段，不得替测试入口补填通过。保持原步骤的回复和发布契约，只报告本步骤已有事实；尚未发生的发布不需要 PR URL，不得因此阻塞当前步骤或向用户索取未来结果。
