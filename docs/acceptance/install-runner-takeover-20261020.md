# 2026-10-20 安装部署接管与任务库重建

用户要求先 rebase，再处理安装版部署报错；此前已明确允许清理任务和旧执行。本次为实际安装维护，不将安装环境用于回归测试。未操作 `C:\SteerlineRuntime`、业务 Git 分支或 PR。

## 基线与原因

- 当前工作区从 detached `d22b93d8` 建立 `codex/fix-runner-takeover-query`，rebase 到当时最新 `origin/main`：`f21aea1fa972f9a89ff5fd6515b0eba4edf229ce`。上次本地目录修复已被主干覆盖，冲突采用主干；原维护文档保留，临时 stash 已核对并清理。
- 安装中的 Runner `0.2.0-dev.2` / SHA256 `33851d039193ca1f2f6766ab15e55abb54bda118ed673f81f45ad2cc80ec6b49` 的 offline capabilities 没有 `service verify-takeover`。候选 `0.2.0-dev.3-r2` 在线预检向该旧服务查询，复现 `service_unavailable: The service query response is unavailable.` / exit 4，部署在停服和替换前拒绝。
- 通过原安装 CLI 正常 stop，exit 0，原实例 `cd52ed23c9b9640d9a509fce5417e253` 的资源结算为 true。相同候选随后沿现有离线核验取得 compatible=true、items=[]，没有删除 pin、跳过校验或增加旧协议回退代码。
- 新版安装后，三个项目因 SQLite schema 23 与当前 schema 25 不符而失败。该失败及其停机回执保留；此阶段 Runner 未启动，原 Runner 公开停止证明仍为零 Run/Check 且已结算，未将失败停机回执改标 clean。

## 实际处理

维护证据根目录：`C:\SteerlineInstall\maintenance\runner-takeover-20261020`。

原应用/Web/Runner 退出、项目本地关闭和锁释放后，持有安装启动/部署/仓库锁，完整归档三份旧状态目录。一次性维护脚本将已有任务定义导出为正式 task-batch/v1，不向产品增加旧 schema 读取或迁移路径，也不直接改旧库 schema。

新状态由正式启动创建为 schema 25，通过 `task validate` / `task create` 恢复原任务定义。全部 20 个任务的编号、标题、类型、优先级、完整契约及其固定 Git 引用，以及 8 条依赖逐项匹配导出；新批次、创建时间、版本及操作回执重新生成。历史执行、旧批次及审计事实留在归档，不伪装成新库原执行。三个项目均设为 manual + paused，零业务候选。

最终部署记录：`C:\SteerlineInstall\deployments\20261020-064100-e10bdc6e\deployment.json`。Runner 来源 `13ef12312944317e7b78d4de44977b92e65e70c2`，版本 `0.2.0-dev.3-r2`，SHA256 `482aafcffd8b61516f26e842361602741643f52e2d77d2be5ccea6461da4292f`。SteerLine 来源为上述 rebase 基线；本地维护文档使部署记录如实标 dirty。

## 验证结果

| 验证 | 命令 / 证据 | 结果 |
| --- | --- | --- |
| L1 部署围栏，隔离 fixture | `./scripts/test-deployment-stop.ps1` | 通过；strict settlement、兼容 takeover、固定 Runner 复用及失败拒绝 |
| L1 调用方与 CLI，隔离 fixture | `go test ./internal/runnerclient ./cmd/steerline -run 'TestRunnerCatalogAcceptsWorkflowsWithoutFixedPromptSteps\|Test.*Takeover' -count=1 -timeout=120s` | 通过，4.255s / 8.270s |
| 原停服与离线兼容 | `original-shutdown.json`、`offline-takeover.json`、`runner-stopped-before-rebuild.json` | 原服务正常退出且资源结算通过；零原 Run/Check；候选兼容 |
| 构建安装 | `build-deploy.log`、部署包 `runner-takeover-preflight.json` | exit 0；三份已安装 EXE 摘要与部署记录一致 |
| 当前库与任务 | `final-verification.py` / `final-verification.json`；各项目 `*-validate.stdout` / `*-create.stdout` | 三库 schema 25，integrity/foreign-key 检查通过；20 个任务及 8 条依赖恢复，契约完整匹配，全部 open；manual + paused，候选 0 |
| 实际启动 | `status-after.json`、`start-result.json` | backend/Web ready，三个项目 ready 且 online，degraded=false |
| 新服务在线预检 | `online-takeover-after.json` | 新原实例 `943c1eed5cfa99968be1cd35f30b12d9`，compatible=true、items=[]，已走在线路径 |
| 页面 | `Invoke-WebRequest http://127.0.0.1:18742` | HTTP 200 |

初次任务重建使用非 32 位十六进制 request_id，被正式创建接口明确拒绝，未写入任务；维护脚本改用固定合法 ID 后创建成功，原错误保存在 `SteerlineTest-create-invalid-request-id.stderr`。第一次完整核对早于第二批创建完成而失败，全部正式回执完成后的最终核对通过；不把前次未完成状态记为通过。

本次没有新增或修改产品实现；仅更新安装说明和维护记录。未运行 full、L2/L3、真实模型或真实 PR 执行，安装就绪与空 Runner 接管不代表这些业务链路通过。
