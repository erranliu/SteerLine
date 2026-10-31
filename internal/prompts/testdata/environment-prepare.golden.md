Windows 上首次读取材料及后续执行命令的 exec_command 均显式设置 shell="powershell.exe"、login=false；实际检测的 tty 参数遵循固定材料。

先读取 SteerlineFlow/environment-execution.md、SteerlineFlow/environment.md、其固定验收脚本及 STEERLINE_RESULT_FILE 初始 JSON，再执行脚本 prepare 阶段。按原规则完成检测与完整 L2，结束前原子写最终报告；本阶段不创建 PR。