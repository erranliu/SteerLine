Windows 上首次读取材料及后续执行命令的 exec_command 均显式设置 shell="powershell.exe"、login=false；实际检测的 tty 参数遵循固定材料。

先读取固定提交 fixed 的 SteerlineFlow/environment-generation.md、SteerlineFlow/environment-execution.md、SteerlineFlow/environment-context.json、项目测试规范/清单及依赖文件，生成环境文档、脚本和真实缺口。只返回原 schema 结果，不执行或发布。