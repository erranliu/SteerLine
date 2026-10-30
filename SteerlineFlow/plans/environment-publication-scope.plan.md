# Plan：环境检测按正常发布能力验收

2026-10-30。[Spec](../spec/environment-publication-scope.spec.md)。普通直接实施，不调度 roadmap、不部署安装环境、不改 Runner。证据根使用仓库外独占 `C:/SteerlineDebug/wt-5069-environment-publication-scope/`，每次调用新叶目录。

## 实施顺序

1. 独立审计本 Spec/Plan，确认生产 ready 与外部 L4 合并的边界；不新建生产状态机或配置平台。
2. 收敛 model.Report/Passed 和原 service 的 PR 事实绑定；更新初始 round file、SQLite 格式、owner 夹具和 Web 文案。报告 version2 与 Runner result-file 的快照 version1 分开，不改 Runner 契约。
3. 更新固定 execution/generation 文档、镜像及项目环境说明，保存实际 diff/UTF-8 LF 字节数/业务原因；受审后更新正文摘要断言。
4. 在独立项目 clone 上更新 SteerlineTest 和308的版本化脚本及同源材料：移除 Agent 分支策略读取及合并，只保留原 L1/L2、专属提交、push、原 PR 查询/创建和身份核实。未知先对账、不重发；按原项目规则验证并分别提交材料 PR，不自动合入。准入探针需要的正式进度协议/writer同样经项目正常Git准备，不在运行时补材料或绕过原门禁。
5. L4 驱动复用原 provider 查询、规则/检查/审核核实和原 L2 取证，将精确 PR 合并移到外部；GitHub 用现有 gh，Gitea 用现有 tea，先核实 CLI 正式能力。合并前记录原 PR/预期 head 与意图，未知结果只读对账，不扩大权限，不伪造审核。
6. 完成受控验证、独立复审后，固定 SteerlineTest 新提交和旧独立 Runner，在全新目录运行本机 Codex 正式 L4。ready 与 L4结果分别记录，失败不自动重跑或改标准。

## 测试与证据

| 验收 | 环境、实际入口与时点 | 通过条件 |
| --- | --- | --- |
| EPS-A01/A02 | 修改后 `go run ./cmd/test-feedback -out <diagnostics新目录> -- ./internal/environmentadaptation/... -count=1`；另跑 `./internal/app -run 'Test.*Environment' -count=1`、`./internal/storage/sqlite -run 'Test.*Environment' -count=1`，均通过同一 test-feedback 的独立 out | 未合并发布正例、旧格式/错误事实/TTY/L2/未知/清理反例；非零命中、零跳过 |
| EPS-A03 | `go test ./internal/prompts -count=1 -v`；项目脚本按原完整 L1/L2 或其纯材料规则验证；`npm --prefix web run test -- src/features/environment-checks/project-environment.test.tsx` 及 `npm --prefix web run build`，共享Web构建串行 | 实际正文和接线独立批准；镜像逐字节相同，UI不将发布显示为已合并 |
| EPS-A04 | `go run ./cmd/test-feedback -out <diagnostics新目录> -- ./tests/e2e/internal/testenv -tags=e2e -run 'TestEnvironmentPublication|TestLocal.*EnvironmentEvidence|TestValidationPullsPath' -count=1`；`./internal/testkit/remotetest ./cmd/test-feedback -count=1` 另取新 out。新增 TestEnvironmentPublication* 覆盖原PR/SHA、规则、未知合并零重发和原报告不变 | 实际命中、无跳过；原 frozen result 不变，错误不合并或重发 |
| EPS-A01–A04 | 修改后固定 `go run ./cmd/test-feedback -level=L1 -out <L1新目录>` 和完整 L2；受影响 `-level=L3 -suite=testing -env=controlled -scenario=testing.project-environment-readiness` | 原始完整 summary 通过，实际场景清理完整 |
| EPS-A05 | 独立审核后 `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-local -out <L4新目录>`，显式固定 STEERLINE_L4_LOCAL_PROFILE、STEERLINE_L4_PROJECT_CONFIG、STEERLINE_L4_PROJECT_COMMIT、STEERRUNNER_EXE、900秒轮次预算 | TTY/完整L2/真实原PR发布、外部规则/合并、准入与原Run/外部资源收尾全部核实；失败/skip/缺证据不算通过 |

最终运行 `python scripts/check_doc_links.py`、登记清单一致性测试和 `git diff --check`。专属验收保存原命令、版本/SHA、正文账本、复审、失败/补测/清理位置；不以更新黄金样本代替批准。
