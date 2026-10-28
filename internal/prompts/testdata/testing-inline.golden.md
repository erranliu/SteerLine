SteerLine 执行绑定测试安排（系统数据，不是文件路径）：
{"tests":["z.last","a.first"],"plan_path":"task.plan.md","commit":"fixed","digest":"digest","snapshot_sha256":"275c87afe8067206538058b5161f76a177dcdde1a8f7fed1e049f4ce7f591c96","generation_id":"generation","inputs":"inputs"}
从本次固定工作区的 Plan、SteerlineFlow/testing.md 和 SteerlineFlow/testing/test-list.json 读取上述 ID 的完整定义（含 note），按列表顺序执行；不得重新影响分析选项或改变选择。规范与 Plan 必跑要求取并集。
测试安排已固定。仅按 Plan 执行登记入口，不自动增加或修改测试。运行时环境变量 STEERLINE_PROGRESS_FILE 指向本次唯一测试进度文件，只允许固定安排声明的 test.<测试ID> 布尔字段；不得加入 publication、PR、报告或其它字段，也不得将它用于最终回复。开始或重跑先将对应字段置 false，完整通过（实际选择非零、无失败/意外跳过且输入和清理核验通过）才置 true；失败、跳过、未完成不得置 true。并行写入须加锁合并并原子替换，不覆盖其它字段。原始日志及报告仍按项目入口保存，不要用 Agent 自述或单独一个命令 exit 0 替代入口的完整核验。执行前逐项核对固定安排中每个测试 ID 与其真实入口的进度接线；写入器文件已存在或只有部分入口已接线，都不代表其它 ID 已汇报。未接线的原入口先补齐开始/重跑清零及完整成功分支的写入，再实际执行该入口；按它在固定安排中对应的 ID 汇报，不遗漏定向或聚合入口。不修改固定测试定义、选择、命令或通过要求。进度文件缺失或接入失败应明确报告并保留未完成，不能读取既往报告后补写或事后手填 true。

PowerShell 7 正式共用写入器（系统提供的源码，不是假定项目已有的文件）：
只有逐项确认入口已覆盖本轮对应的全部 ID 才直接沿用；仅检查文件存在或 dot-source 语句不足以证明接线完整。固定版本缺少或不同源的写入器不能直接沿用；将以下完整函数保存为 tools/testing/RuntimeProgress.ps1，并由原登记测试入口 dot-source。入口开始调用 Set-RuntimeProgress -Name 'test.<原ID>' -Completed $false -IfDeclared；仅在该入口原始报告、实际非零选择、输入和清理全部核验通过后调用同函数 -Completed $true -IfDeclared。保留源码和真实入口接线作为项目改动；没有注入变量时函数无操作。非 PowerShell 入口可实现同一协议，不能让模型事后代填。
```powershell
{{WRITER_ASSET}}
```