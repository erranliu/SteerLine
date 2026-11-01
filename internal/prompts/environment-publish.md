先读取 SteerlineFlow/environment-execution.md、{{.DocumentPath}} 和 {{.ScriptPath}}；仅在 prepare 完整通过后执行该脚本 publish 阶段，核实原 PR 发布身份及脚本自有资源清理，结束前原子写最终报告并返回原 PR URL。
