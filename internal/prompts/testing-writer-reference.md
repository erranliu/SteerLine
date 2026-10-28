
正式写入器已与本次固定提交逐字核实（仅允许 CRLF/LF 差异）：tools/testing/RuntimeProgress.ps1。读取并复用此文件；逐 ID 核对真实入口接线，入口开始/重跑清零，完整通过才写 true，使用 -IfDeclared。非 PowerShell 入口可实现同一协议。保留真实入口接线为项目改动。
