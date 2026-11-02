# 全局开发者配置

「配置 → 开发者配置」管理当前应用 home 内所有项目共用的开发开关。定义与默认值集中在 `internal/developerconfig`；后续开关继续使用明确的布尔字段，默认关闭开发覆盖。此模块不读取文件、不探测环境，也不持有可变全局状态。

## 环境就绪阻塞

`execution.json` 中的配置：

```json
"developer": {
  "skip_environment_readiness": false
}
```

界面名称为「环境未就绪时阻止执行」，默认开启，对应 `skip_environment_readiness=false`。关闭后，未检测、未通过、已撤销或材料变化导致失效的项目环境资格不再阻止手动启动、新增候选和自动派发。环境检测仍可使用，其原始结果与状态不会被改成 ready；执行入口和环境摘要会显示阻塞已关闭。

保存复用全局配置的摘要条件写入和原子替换，冲突时保留输入，失败时不自动覆盖其他页面的配置。保存后需重启 SteerLine。沿用现有配置变更保护：保存与重启之间暂停新的执行受理，已运行的执行继续正常对账。页面区分草稿、已保存待重启和生效状态未知。

应用组成阶段解析配置并按值注入服务和 SQLite writer，生命周期内不热切换策略。最终候选预留仍在同一事务中校验任务、依赖、版本、执行模式和名额。候选中的 `environment_readiness_skipped` 只记录该次预留事实，由 writer 的可信策略确定；该字段不能授予免检资格，后续更新不能修改它。

运行机上未结算的环境适配占用保护、执行方案可用性、Runner 身份/状态、项目材料与测试安排等条件继续生效。重新开启阻塞并重启后，新候选恢复资格校验，原候选的对账与原请求重放保留其历史事实。

## 发布构建与移除

当前普通开发构建提供开发者能力。发布构建统一使用 `release` Go build tag：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\build.ps1 -Web -Release
```

此参数同时给 SteerLine 和 Web EXE 设置发布标签，Web 保留 `webui` 标签；不改变 Runner 的构建方式。直接构建时分别使用 `go build -tags release ./cmd/steerline` 与 `go build -tags webui,release ./cmd/steerline-web`。

发布构建中的能力常量为 false：Web 配置快照返回 `developer_available=false`，隐藏整类入口；配置校验和写入接口拒绝启用开发覆盖；应用配置加载也明确拒绝非默认开发覆盖；有效策略始终恢复正常门禁。发布前移除开发覆盖或将它们设为 false。不能通过环境变量、项目配置、请求字段或保存文件重新打开发布能力。

彻底退役时移除集中配置定义、开发者面板及应用注入点，并恢复最终预留的固定检查；历史标记仅为记录，不参与权限判断。本功能不新增数据库表、版本迁移或旧格式兼容路径。
