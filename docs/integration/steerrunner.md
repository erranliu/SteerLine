# SteerRunner 依赖与运行

计费职责调整见[Spec](../../SteerlineFlow/spec/billing-ownership.spec.md)、[实施 Plan](../../SteerlineFlow/plans/billing-ownership.plan.md)和[隔离验收](../../SteerlineFlow/testing/acceptance/billing-ownership.md)。Runner 只提供用量事实；SteerLine 管理价表、计算和展示估算。安装版尚未部署此次变更。

## 无服务原生目录查询

2026-11-05 Web 已接入固定 EXE 的 `native-catalog --input - --json`，按离线 capabilities.native_catalog 的 `steerrunner-native-catalog/v1`、schema、构建 SHA256/resource_digest 判断资格。stdin 是查询对象，目录调用不带 state-dir/实例/Run 绑定，也不启动服务。支持登记环境、本机/远端 Orca Agent/模型、本机 Codex 模型目录；原生连接及精确元数据进程归 Runner，Web 保留产品支持判断、配置保存和在途合并。30 秒取数、1 MiB、五秒清理，失败旧结果明确过期。策略预检消费已有 model_input_contract.workflow.project_policies.slots。具体实现、固定构建、原生资格与独立审核见[验收](../../SteerlineFlow/testing/acceptance/runner-boundary-consolidation.md)。
