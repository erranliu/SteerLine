# Task18 第二轮测试进度 0/6 修复

用户要求修复后连续两次真实复验通过再告知。范围是原登记测试、进度写入/采集/页面、真实发布和资源清理；完整 MagicPotion 专项、人工业务验收及 clean 发布 head 测试资格不由此推导。

## 原始故障与边界

原候选 `1af016d628b355e048bb101b47012557`、Runner Run `596b316d8e877daed36279206054f05a` 的原六项执行报告已独立通过，但页面为 0/6，公开接口诊断“进度字段与本次固定测试安排不一致”。公开 Runner `progress-file` 取回的 closed 原始字节为六个 false 测试字段及一个 publication 对象，版本 2、563 字节、SHA256 `d55158423d8617396f6a753ee034e0dead0e237049d611a11779973b4db9d9d6`。原项目固定 base 没有共用进度写入器，原提示却仅要求使用该工具，缺失时保留未完成；后续非测试步骤也未收到该文件的业务边界。严格解析拒绝混合对象符合契约，不能靠忽略字段或模型 final 填回通过。

本次只通过公开服务短暂查询已停止的原 Run，没有发送 task submit/reply/resume，没有改原工作区文件或报告。第一次诊断脚本的 PowerShell JSON 解析及缺少 stop envelope 失败保留；独立后续驱动完成原字节核验，并按实际 instance 单次 public service stop，确认 not_running / offline resources_settled=true。原业务 Run 始终 stopped。证据根：`C:/SteerlineDebug/wt-507e-progress-0of6/original-progress-public-02/`；原截图及报告仍位于 `C:/SteerlineDebug/wt-507e-task18-retest/round-07/`。

## 修复与验证安排

- 调用方实施/审计提示包含本仓正式 PowerShell 写入器源码，缺少工具的项目先接入原登记入口；只由原非零选择、报告、输入和清理验证决定完成。
- 发布及报告提示保留原发布/回复要求，同时明确测试文件只读。冻结字段、初始化、严格解析、Run 身份和版本守卫不变。
- 定向回归执行内联源码，覆盖项目无工具、重跑 false、其它字段保留和 publication 拒绝，并验证实施/发布/报告权限隔离及原用户提示保留。
- 两次真实远端进度复验和清理尚待完成；当前连续通过 0，不把原 Runner 链路两次通过或已有报告计入本次。

首轮完整 affected-owner 回归在 `validation-01/owners.log` 发现两个主干已有断言失败：HTTP consumer 边界尚未允许纯值配置/公开协议包，以及 launcher 列表仍遗漏已实现的 restart.bat。用主干基线 `0ab5e3a59077776fd2969a7941590b1467cf94ba` 的独立导出源码定向复现相同两项失败，证据 `baseline-two-failures.log/json`。维护为精确允许两个不持有执行资源的共享值包、继续检查它们的完整依赖闭包；launcher 检查增加 restart 文件及其真实命令/CRLF。没有放宽任何测试进度、Unity、Run 身份或真实复验门槛；原失败保留，后续验证使用新目录。

## 源码回归及真实复验代际

主干修复构建源 `ab8611c1b8b4b7dd58f0c376c34aa32409a044a7` 的受影响 owner、应用定向失败项及进度回归通过；固定 L1 complete/passed、L2 passed/cleanup_complete 均为 true，证据 `validation-02/`。受控 `testing.runtime-progress` L3 passed/cleanup_complete 通过，证据 `validation-04-progress-controlled/reports/`。前一次 `validation-03-progress-controlled` 被 PowerShell 拆分未加引号的 scenario 参数，在测试选择前失败；修正精确参数数组后另目录执行，没有修改测试或补造报告。

原 Task18/f3 代际与当前 main 的新项目环境适配准入不同。主干包新 `round-01` 在业务接纳前停于 `Runner result-file capability unavailable`，唯一 start request `8419176246953318637e8d4a72eaefcd` 最终 expired/rejected，公开页面未建立 Task18 Run；该 home 已按正式入口停止，offline resources_settled=true，保留 `round-01/pre-admission-cleanup/`。未改新 main 的环境门禁、原项目合同、配置或旧 home/pin，不把这次前置失败记作通过。

为复验用户指定的原现场，另用普通独立源码 clone，从原 Caller `57623186b63c16be2a5748665e63d52ae615adbf` 只追加同一修复及断言维护，干净受测源 `42c16465`。主干修复 `24daeeb8` 与原 Caller 修复 `440110fb` 的 stable patch ID 均为 `425e3082cff59b8e6dd39389e2d6576c4fa3d659`，原固定 Runner f3、模型、原合同及已批准 Unity 绑定保持。`original-caller-fix-equivalence.json` 保存来源证明；`validation-05-original-caller/` 及后续 `live-01/`、`live-02/` 单独登记。该原代际复验不证明 main 新环境适配门禁已通过，也不提供产品运行时回退路径。

关联：[Spec](../../spec/runtime-test-progress.spec.md)、[Plan](../../plans/runtime-test-progress.plan.md)、[接入说明](../../../docs/integration/runtime-progress.md)。

## 原代际 live-01 与后续候选 Runner

`live-01` 的原 Run `3808918f4de62554116d03ce354545d4` / Attempt `3e8a3e01bdba5f013a348fe3cabd09c7` 已独立核实原 L1 的 11 个 XML 成员、原 L2 两个各一成员的 XML batch、四个结构检查 exit0，以及原 plan、输入、case receipt、清理和实际 Unity 版本；没有重跑 Unity 冒充原报告。原登记入口已接入正式 writer：开始或重跑置 false，完整 managed 报告和原结果/输入/清理通过才置 true。远端 writer 仅多出空行，其余源码逐行等同正式 writer；父调试进程未写进度。

公开原始进度 version11 为恰好六个声明的 true 布尔字段，无 publication 对象；194 字节，SHA256 `71585b019da15301909af3407ce5d3bee8651b9c8612fec1beb19d4adfd4e7a7`。API 和独立目检截图均显示 6/6。前序 Incomplete / 结构失败报告保留。L1 与 L2 的 dirty codeIdentity 分别为 `afe:c2ad5a...` 和 `afe:c2e04f...`，不能宣称同一最终源码或 clean 发布 head 已通过。

原 f3 Runner 随后停在缺少原输入归属证明：原生 main 已 done、空 prompt、完整同根 hook 元数据，但漏掉 working 时现有独立原输入查询仅接受 thin 元数据。没有真实 PR，故本轮完整复验不通过，连续数仍为 0。原 Run/宿主均按公开控制停止，offline resources_settled=true，完整精确 owned native census0、truncated=false、omittedHosts 为空，本地 home/bin 进程0；原工作区/报告保留。证据为 scope 下 `live-01/checks-verified.json`、`verification-20261026T0915-inputs/`、`diagnostics/report-transfer-recovery-2026-10-26T09-10-56-017Z/` 和 `cleanup-blocked-20261026T0920/`。

独立 Runner 修复见 [SteerRunner PR62](https://github.com/erranliu/SteerRunner/pull/62)：复用原 terminal 围栏和唯一完整原输入证明，在稳定完整元数据下补证，不新增状态/协议/重放，不虚构 working。主线受测源码 `8eda1434` 正式 L1 702 顶层+29扩展通过；原代候选 `8e1ce9f9` 正式 L1 675顶层+29扩展通过，两个代际 owner 均通过，同形测试在旧 f3 失败。stable patch-id 均为 `6c2ff4585680000f87e68df9fef2686c980331b4`。

用户已明确允许未来新轮使用候选 Runner，原代候选 EXE SHA256 为 `30b3bc832a123427b9494675b242351df975e99994e5688121ef185b4055355c`。另存授权 `runner-pin-authorization-20261026T0930.json`，用原干净 Caller42c16465 及该 Runner 重新通过正式生成入口建立独立包 `candidate-caller-packages/dev-20261026-173725-42c16465`；未来 `candidate-01/02` 采用新普通 clone/home 和独立固定，原 home/pin 不变。原模型、环境 profile、合同、base、L1/L2/四结构及 extra 检查要求保持。仍需两次真实发布前后/closed 采集、原报告、UI 与资源清理完整合格；原 6/6 或 Runner-only 历史成功不计入。
