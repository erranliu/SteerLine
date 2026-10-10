# SteerlineTest debug task plan

Read the bound Spec. Run sandbox-check.ps1 in the normal sandbox, then create or update result.txt with the fixed task/run identities, verify and commit only that file in implement.execute. Report existing facts in implement.report. Run publish.ps1 only in publish, following the Spec's bounded automatic-review retry rule.

## 测试计划

implement.execute 在 Windows 普通沙箱运行 `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./sandbox-check.ps1`，要求 exit_code=0、passed=true；覆盖真实 PowerShell、task-owned TEMP/TMP、Git 自动维护关闭、项目文件检查及驱动提供的兄弟目录写入拒绝。检查 result.txt 的 task/run 身份及候选仅修改该文件。publish 的脚本通过真实 Git helper 推送并核验 PR base/head/SHA，必要自动审核记录原生结果；合并后由中心确认交付和后继就绪。证据保存在本包 evidence/ 和 native journals，不使用安装环境。无业务代码测试或 full；探针与真实发布覆盖本 fixture 的必要自动验收。
