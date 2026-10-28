

PowerShell 7 正式共用写入器（系统提供的源码，不是假定项目已有的文件）：
只有逐项确认入口已覆盖本轮对应的全部 ID 才直接沿用；仅检查文件存在或 dot-source 语句不足以证明接线完整。固定版本缺少或不同源的写入器不能直接沿用；将以下完整函数保存为 tools/testing/RuntimeProgress.ps1，并由原登记测试入口 dot-source。入口开始调用 Set-RuntimeProgress -Name 'test.<原ID>' -Completed $false -IfDeclared；仅在该入口原始报告、实际非零选择、输入和清理全部核验通过后调用同函数 -Completed $true -IfDeclared。保留源码和真实入口接线作为项目改动；没有注入变量时函数无操作。非 PowerShell 入口可实现同一协议，不能让模型事后代填。
```powershell
{{.Writer}}
```
