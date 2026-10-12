# SteerlineTest debug task plan

Read the bound Spec. Run sandbox-check.ps1 in the normal sandbox, then replace result.txt with the marker and the current branch from git branch --show-current, verify and commit only that file, then stop. Do not infer task/run IDs or reuse the preceding task's file. A separate report input formats existing facts without tools. Run publish.ps1 only after a separate explicit publication input, following the Spec's bounded automatic-review retry rule.

## 测试计划

在新生成的 Windows live 包运行 `scripts/test-codex-sandbox.ps1`：implement.execute 的普通沙箱探针要求 exit_code=0、passed=true，覆盖真实 PowerShell、task-owned TEMP/TMP、Git 自动维护关闭、项目检查及兄弟目录写入拒绝（[CSG-01、CSG-02、CSG-03](https://github.com/erranliu/SteerLine/blob/main/docs/acceptance/codex-sandbox-git.md)）。驱动核对 result.txt 的精确当前分支、候选仅修改该文件、原生实现/报告轮没有发布命令；publish 经真实 Git helper 核验 PR base/head/SHA 和原生审核结果（CSG-04）；两项合并后核对中心交付、后继就绪及不同 worktree/session（CSG-05）。模板/驱动修改后先跑无模型定向与 controlled 门槛，再跑 live；通过条件是两项完整交付及本次资源清理，拒绝、阻断或缺证据不得算通过。证据保存在本包 evidence/ 和 native journals，结果登记在专属验收文档；不使用安装环境。不选择 Full 或全部 L2/L3，此 fixture 的探针与双任务真实发布覆盖必要自动验收。
