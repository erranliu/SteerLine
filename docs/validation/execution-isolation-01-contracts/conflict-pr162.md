# PR162 主干冲突整合

日期：2026-09-30。PR164 整合主干 `930a4d6`（PR162）。保留独立 `orca_remote_worker` 生命周期、中心 Task/Dispatch 接线、当前查询与共享记录，以及主干删除的旧适配器/exec journal 入口。

远端后端直接使用公共 ProcessIdentity、ExecutionTarget、BackendKind、ExecutionProtocol 和 BackendErrorCode，删除同义本地定义；平台 Ref、Error 与 Workspace 等原生请求/返回结构仍由该后端拥有。两个身份/目标布局在搬迁前与公共定义逐项一致；既有 17 组共有字段与新主干一致，序列化/schema 不变。公共包的项目内传递依赖仍只有自身。

本机事件和 turn evidence 按新主干只处理 local Codex；Orca 按固定 Dispatch 的独立适配入口处理。已删除的旧监督 exec journal fixture 跟随主干删除；此前的原生消息来源与 Codex 选项隔离断言保留。旧 orcaProcess 随主干移除，停止后无需清理该观察器；调用方句柄不被 Close/Kill 的断言合入现有 `TestOrcaStopExecutionConfirmsFixedTerminalExit`，继续防止原审计发现的清理越界。

验证：

- `execution`、`codexworker`、`orcaworker`、`orca_remote_worker` 包测试通过。
- 相关入口测试编译、受影响包 `go vet`、Linux amd64 交叉编译通过。
- 项目 `-quick` 加既有固定身份/事务/回执用例，以及远端两轮续聊、问题回复与丢失回执对账用例通过；[实际分组与命令](conflict-pr162-summary.json)均命中，没有运行全量或真实 agent。
- 生产源码无冲突标记，公共同义类型与 PRClues 只有一个所有者；`git diff --cached --check` 和文档链接检查通过。

实际架构依赖和全部源码索引已同步为 18 个包、216 个生产 Go 文件。
