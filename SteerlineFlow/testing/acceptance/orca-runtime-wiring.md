# Orca 执行主机 runtime 接线验收

日期：2026-10-17。[Plan](../../plans/orca-runtime-wiring.plan.md)。普通开发；本记录仅归属本次修复。

原问题：安装候选 `f38901b9caaf46f2a617b06b03247b85` 的方案缺少 `agent_launch.runtime_executable`，Runner 在准备节点以 `remote_sandbox_authentication_unsupported` 拒绝，业务输入为零。原候选和安装现场保持原样。

| ID | 自动验收 | 状态与证据 |
| --- | --- | --- |
| ORW-A01 | 缺失/非法 helper 参数早拒绝；生成配置和 manifest 完整绑定 helper | 通过：四种参数反例均早拒绝；带模型包 `dev-20261017-074912-48c56719-dirty` 和无模型包 `dev-20261017-075316-48c56719-dirty` 均绑定 helper/SHA/native，后者无 model 字段。首包 `evidence/parameter-validation.log`、`default-model-generation.json` |
| ORW-A02 | Runner adapter/agentlaunch 定向回归 | 通过：`go test ./internal/execution/adapters/orcatransport ./internal/execution/agentlaunch -count=1 -timeout=3m`；新包 `evidence/runner-targeted.log` |
| ORW-A03 | 独立 Orca 调试包真实实现/报告/发布、产物、PR 与停止结算 | 通过：最终包 `dev-20261017-082031-48c56719-dirty`，实际三阶段到 pr_ready，PR #429 与 Git 字节独立核验，停止/服务/原终端结算均通过；见下文 |

人工项：无必需项。验证过程中不复制安装版数据库、认证或配置，不恢复安装版原候选。

## 固定依赖与失败记录

- 初始 Runner 源 `d4969d07e41406c8a85179b6a483d42ee367b1e9`，干净源码构建，SHA256 `928daa427b97dde4e391d742df992aa5ad74deb3801dc628a57813b364319c7f`。后续真实发布发现独立的 Runner broker 字段缺口，补丁与最终新构建另记，不混用通过状态。
- 执行主机 helper `C:/sr-runtime/2ba0/24/steerrunner-agent.exe`，实际 SHA256 `60035e89d301065f4acdba728f7f00d22499a9358f3d408ce7fa64f793a0ff7c`；native `C:/sr-runtime/2ba0/01/native/codex.exe`，0.159.2，SHA256 `52f75c649bebb8001102a1dd129c1ea6d02b0940321e6d7e82ee0526753bd58a`。只读核对后关闭专属诊断终端；未安装/替换执行主机程序或复制登录。
- 首包 `C:/SteerlineDebug/dev-20261017-074912-48c56719-dirty` 的首次宿主启动受工具 kill-on-close Job 拒绝，零 worker；改用现有测试惯用的 WMI 独立同用户进程启动。PowerShell 5 对 native stderr 的包装失败另留日志，未改变执行权限或配置。
- 首包应用候选 `8e3cfbe36f4a4e629f6a5983e7d5e425`、Runner Run `9b15c5ce75278e49d260ec7a017d909b` 已通过远端认证和确定性预检，并写入、核验 result.txt。但原调试 AGENTS 要求 `git config --local`，触及共享 `.git/config`，沙箱正确拒绝，未提交/发布。`failed-product-run.json` 保留失败。随后模板改为单次 commit 的 `git -c` 身份，没有放宽 Git 写根或权限。
- 首次聚合 stop 因尚未显式停止 Orca Run 返回 resources_settled=false，失败回执保留。使用本包 `debug-helper runner-cleanup -config <本包配置>` 明确停止原 Run，返回 confirmed=true；Runner 最终 service status 为 not_running、last_shutdown.resources_settled=true；原 worktree 的 Orca terminal list 为空。原失败宿主回执不改写，不把首次 stop 记作通过。
- 第二次 live 包 `C:/SteerlineDebug/dev-20261017-075525-48c56719-dirty` 固定 `gpt-6-luna/high`、默认 sandbox。候选 `ae7b62e8284648b6b432818d6188fcf4`、Runner `ef8c6197ea56a0f2cb8dd094936796fd` 同会话完成 implement.execute/report；提交 `07cee9d0d06d397645afe12636cfa12b012e7216` 仅修改 result.txt。Git 有既有 packed-refs.lock 清理警告，退出 0、提交/父提交/路径均核验；不声称零权限诊断。publish 因旧 AGENTS 要求直接执行 publish.ps1，被主机脚本策略拒绝，本次失败保留。独立查询对应 base/head 的 PR 为 `[]`。通过产品 stop_candidate 明确停止，Runner new_inputs_allowed=false / stop_status=stopped，随后本包 debug stop 返回 0、service status 记录 resources_settled=true；无策略修改或发布重试。模板随后改为 Runner 的普通 Git/gh 发布，不再调用脚本，不修改执行策略；新包另验。
- 第三次 live 包 `C:/SteerlineDebug/dev-20261017-080103-48c56719-dirty`，候选 `0ca68b1b8b8e44b5a4599fd56cf93302`、Runner `8529c84576597c8872b986ca46a3e670` 完成实施/报告，并由原 worker push 提交 `4d92bd78729ca6de64ab592d9803354804f05d43`、创建 [SteerlineTest PR #428](https://github.com/erranliu/SteerlineTest/pull/428)。独立 gh 查询确认 OPEN、base/head/提交匹配；worker 复用输入内提供的查询（包含 mergeCommit）时被受限 broker 403 拒绝，未记 publish 通过。诊断只读取原生会话的这条 PR 命令，确认 Runner whitelist 漏了 `mergeCommit { oid }`，由 Runner 修复并维护专属 PMM 验收。原包冻结不换 helper，新包另验。

## Runner 补丁首版与后续真机问题

本次修复属于两个 owner：SteerLine 修配置与调试材料；Runner 修 PR 只读字段白名单。Runner 在 `codex/pr-merge-metadata` 分支对基线 d4969d0 增加 `mergeCommit { oid }`，仍拒绝跨仓及 commit/history/repository 扩展；本机及执行主机定向测试通过。Runner 自身安排/结果在其 `SteerlineFlow/plans/pr-merge-metadata.plan.md` 与 `SteerlineFlow/testing/acceptance/pr-merge-metadata.md`，本仓不复制实现。

首版补丁包：`C:/SteerlineDebug/dev-20261017-081319-48c56719-dirty`。

- Runner SHA256 `868af3b5798c44d3cb8169fc8547df59d50643dc0b8d544506cb63591b953d97`，source 标为 d4969d0-dirty。
- 新 helper 独立路径 `C:/sr-runtime/steerline-orca-wiring-20261017/steerrunner-agent.exe`，SHA256 `5d072d5df6dc5fd85f395a550156cb52430168335dfb2fb3dbcde3e8c1f0e2b6`，本机/执行主机分别编译 SHA 完全一致；源码补丁 SHA256 `c94001dc1469e57a982faea9eeb925e3edd3236eb292e45bec623c18307e0f12`，Go 1.26.4。未替换旧 helper。
- 应用候选 `431ff043b6624d53a5c575be840072e7`，Runner `fa7d1d9d10d65154ef6ec9d6e19ae9b3`；本包独立 project `e29c55652b6caa4ee147fdb112f3bb09c5538b0337013aa1069bc51693940d4e`，默认 sandbox，gpt-6-luna/high。
- 前序第三包也已明确 stop_candidate，Runner stop_status=stopped，随后 debug stop 返回 0；原 PR 保留作失败证据。各包数据库、workspace/base/head 与请求均独立，不复用失败输入、不改写原失败。

该包真实 publish 又暴露 Runner 已有的同仓 head 限制：gh 使用合法 `erranliu:erranliu/sl-fa7d1d9d10d65154ef6ec9d6e19ae9b3`，broker 因冒号拒绝创建。实际 push 成功，但查询无匹配 PR，未记发布通过。原 Run 通过产品 stop_candidate 后确认 stopped，debug stop 返回 0、资源结算完成。

Runner 补丁第二版仅允许 qualifier 等于目标仓库 owner，继续拒绝其他 owner、空分支与多重冒号，加入四个定向正反例。新 helper 在 `C:/sr-runtime/steerline-orca-wiring-20261017-v2/`；两台机器分别运行定向回归并构建，SHA 再次完全相同。最终固定依赖：Runner `a5e05a64fb682cd260288ac9f0691d1f87bfd72877136bf0ac114dccbe295676`，helper `fdae6d687e49a57e5036c02d2a74274f64b5865356608e2e4058a98548ee3d0b`，源码补丁 `aef5503541fcf4f12435b0e64868b85ca64d59a58dbd5dd31dea0a941ce393f9`。最终产品验证另记，不把首版通过的组件项拼接成最终 live 通过。

## 最终独立产品实测通过

现场 `C:/SteerlineDebug/dev-20261017-082031-48c56719-dirty`；SteerLine 源 HEAD `48c56719` 加本次工作树，manifest 固定源码 dirty、生成器/模板与 EXE 摘要。Runner/helper 使用上述第二版固定构建，默认 sandbox，gpt-6-luna/high。

- 项目 `f13790cbacd6d304145a72267f60e1f611e7d79918c8c36004f8ba8849ece5f8`；候选 `72b243627a2b4461b95e10726e0ae3dc`，Runner Run `40fa14d710bd4883d765d368593cb363`；Attempt `2f0ca9e41bbe9d91f5298444dcadc51a`。
- 同一个 runtime `f8616747-34af-4543-8487-ec6b5d8de4ff`、terminal `term_16bf7655-ca3d-4bb2-9e12-c4140b8ac5cd`、incarnation `b55bac64-7fc6-45f5-9cc6-a29822352713` 完成 implement.execute → implement.report → publish → pr_ready。三个 result_committed 均 succeeded/verified，来源 `codex.app-server.original-turn.v1`，同一 session/Attempt、三个不同公开 turn ID，没有 publish.report。公开 events 46 条、has_more=false，见 `evidence/runner-events.json` 与 `artifact-verification.json`。
- 原 worker 创建 [SteerlineTest PR #429](https://github.com/erranliu/SteerlineTest/pull/429)，head `erranliu/sl-40fa14d710bd4883d765d368593cb363`，base `codex/debug/dev-20261017-082031-48c56719-dirty/base`，实际提交 `ad7305043690b54feab6678a93d27bd8128d9675`。worker 在原沙箱内的实际 `gh pr view` 包含 mergeCommit，查询成功；只读诊断摘录见 `publish-commands-diagnostic.txt`。本轮创建使用未限定 head；同 owner qualifier 的正/负边界由本机和远端定向测试覆盖，不冒称本轮模型选择了 qualified 写法。
- 从 GitHub fetch `refs/pull/429/head` 后独立校验 head、仓库、base/head 分支、唯一变更路径 result.txt。产物恰为 77 字节、无 BOM、两行 LF，内容为 marker 与精确当前 branch；SHA256 `2e90f2613d7371afc4df10d6922fd805ecc074acc2d619607d6685f0ba422416`。`python evidence/verify-artifact.py` passed=true；此核验不替 worker 实现/提交/push/创建 PR。
- 产品 stop_candidate 原请求及响应在 `stop-candidate-request.json` / `stop-candidate-response.json`。公开 Runner `stop_status=stopped`、new_inputs_allowed=false；随后 debug stop 返回 0，service status 为 not_running，原 EXE SHA/目录匹配且 last_shutdown.resources_settled=true；Orca 原 worktree terminal list totalCount=0。见 `stopped-runner-run.json`、`final-service-status.json`、`final-orca-terminals.json` 和产品 home/application-api/shutdown.json。
- 启动控制的原请求 `ece5331715b44b49980b190ed535acbb` 曾遇文件替换拒绝；只 GET 同一请求取得 applied，没有重发 resume_dispatch。完整回执保留在 `resume-original-receipt.json`。临时核验脚本最初误将非空 next_cursor 当作截断，按正式 has_more 字段修正后对同一已完成执行重读通过，没有重跑业务。

ORW-A01/A02/A03 全部通过；参数反例、无模型配置生成、两仓相关定向测试、PowerShell/JSON 语法、Markdown 链接和两仓 diff --check 均通过。未运行完整分级 L2/L3，不记 TaskDelivery/合并验收；PR 保留 OPEN 供检查，未合入任何默认主干。此前失败不抵消，也不合并为首轮通过率。未修改/使用安装环境进行测试，原安装候选未恢复。

## 提 PR 后独立复审

SteerLine PR #346 的独立复审发现 PowerShell 正则 `$` 接受末尾 LF，可能使非法 helper SHA 未在副作用前拒绝。改为 `\A…\z` 严格匹配并加入 `64hex+LF` 反例；五种参数反例均在查询 Runner 前拒绝。原 live 包的固定构建与结果保持原身份，本次参数边界修复使用定向反例验证。
