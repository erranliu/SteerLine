# Plan：通用代理启动配置

1. 增加上层 agent_launch 和参数编译器，冻结、验证、复制并显示模型/强度；Orca 只收到通用命令。
2. Orca 分别保存唯一终端与启动意图；只启动一次，接受后只读等待原代理身份；未知效果不重发。
3. 补最小回归，更新调试生成器和示例；独立验证本机/远端 high/xhigh 的实际启动和结果。
4. 验证后安装四档；保留 local/default 配置，核对运行中服务与冻结任务，安全部署二进制和配置；提 PR 并登记验收。

## 测试计划

| 验收项 | 最小验证、环境与时点 | 通过条件和证据 |
| --- | --- | --- |
| ATL-A1 | agentlaunch、execution Profile 选定单元组，配置/快照修改后 | 参数实际进入命令，high/xhigh 摘要不同；冻结、复制与错误拒绝；验收登记日志 |
| ATL-A2 | orcatransport 单元、两个目标受控三阶段，生命周期修改后 | 一终端一次启动，未知效果只读、身份不符不发任务、恢复不重发 |
| ATL-A3 | 独立 debug 包与 SteerlineTest，四档真实探针，安装前 | 本机/远端 high/xhigh 原生显示与紧凑结果正确；记录身份、回执、清理；不在安装目录测试 |
| ATL-A4 | 构建后检查安装配置、默认/local、已冻结任务与运行状态 | 新二进制正确解析四档，命令准确不同；不改任务定义/全局设置；记录部署事实 |

最小入口：go test ./internal/execution/agentlaunch ./internal/execution/adapters/orcatransport -count=1；go test ./internal/app ./internal/execution -run 'AgentLaunch|Profile' -count=1；go test ./tests/integration -run 'WorkerStageOrca' -count=1。不跑 full。真实入口保存独立包 evidence，结果登记[专属验收](../acceptance/agent-terminal-launch.md)。目录信任等原生准备是前置，不能豁免真实失败。
