# 普通任务运行期间重检项目环境

2026-11-11。已有任务可以和重检并行。同一 home 内，相同 adapter/target 上的未结算检测仍拒绝下一次检测，包括跨项目和重启后。新候选沿原有资格及占用规则准入。

## 308 脚本隔离实测

从 merge-h-308-code 的 `AIRefactor` 提交 `5f765a52571f01a835fde8ff19acc0afd5475cd9` 核对 manifest、PowerShell 入口和声明依赖。实测使用内容一致的 Windows checkout 副本，换行格式为 CRLF。入口只查询工具和配置，以独立临时空项目执行 Unity 授权探针，并写入该次结果文件。

同一 Windows 用户和 Unity 2022.3.62f2 下，先启动代表任务执行的独立 Unity 批处理，确认其 Editor 方法进入执行，再按原调用格式运行 308 脚本。观察到探针的 Unity 日志在任务方法执行期间出现。两个进程均退出 0，检测十项全部通过并返回 `completed=true`、`ready=true`，其临时探针目录已清理。沙箱内初次尝试无法连接 Unity 授权 IPC，此结果来自正常用户环境中的独立复验。证据为 `C:/Users/lmas1/AppData/Local/Temp/steerline-env-overlap-Ccp6mo/overlap.json` 和同目录 `inputs.json`。

实测范围为本机隔离项目的真实 Unity 并行启动及授权。远端 Orca 上的原任务 17、18 和安装版未参与验证。

## 自动回归

受测基线为 `40d710fb89d21923c2a107399b582cd717c3709a`，含本次准入及测试改动。隔离回归使用真实 Git、SQLite 和受控 Runner 夹具。通过正常候选预留保存同目标 `running` Run 后，首个重检成功创建活动，第二个请求因同目标仍有未结算检测被拒绝。当前活动仍是首个重检，原候选的 Run、修订及运行状态保持原值。并发开始、跨项目占用与重启重建由同组已有断言覆盖。

初轮回归在夹具的任务图类型和验收标题处失败，修正夹具后以独立目录复测。通过结果位于 `C:/Users/lmas1/AppData/Local/Temp/steerline-env-recheck-01e93c2943d744ec8bbdd067ea7c517f/`。

| 命令 | 结果及报告 |
| --- | --- |
| `go run ./cmd/test-feedback -- ./internal/app -run '^TestProjectEnvironmentAdmission' -count=1` | 通过，`admission/summary.json` |
| `go run ./cmd/test-feedback -level=L1` | 通过，`L1/summary.json` |
| `go run ./cmd/test-feedback -level=L2` | 受控交付冒烟通过，`L2/summary.json` |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.environment` | 环境基础包与 Windows 应用层 32 个根测试通过，`go.environment/summary.json` |
