Windows 上首次读取材料及后续执行命令的 exec_command 均显式设置 shell="powershell.exe"、login=false；实际检测的 tty 参数遵循固定材料。

先读取 SteerlineFlow/environment-execution.md、SteerlineFlow/environment.md 和同一脚本；仅在 prepare 完整通过后执行 publish 阶段，核实原 PR 发布身份及脚本自有资源清理，结束前原子写最终报告并返回原 PR URL。
