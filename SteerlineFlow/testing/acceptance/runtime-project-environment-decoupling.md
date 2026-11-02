# 运行环境与项目运行环境最小解耦：验收规划

2026-11-02，已实施，所选自动验证及独立代码审核通过。[Spec](../../spec/runtime-project-environment-decoupling.spec.md) · [Plan](../../plans/runtime-project-environment-decoupling.plan.md)。基线 `6f1edde1dbc9bb950d579a4cbe1dca67301c778b`。本记录不继承历史环境 ready 或真实链通过状态。

## 1. 必要行为与待验项

A01–A07 为 Spec 的 RPE-A01–RPE-A07 简写。

| 映射 | 验证行为 | 状态 |
| --- | --- | --- |
| A01/A03 | 从实际 Web HTTP 配置/探测入口依次验证零项目、项目加载失败、缺库和零环境材料；全局配置/目标/目录照常返回，项目访问探针零触发；指定 profile 不存在或目标/Agent 错配明确失败；双父 cwd 独立元数据 fixture 验证绝对程序取证/执行一致、相对程序拒绝、同名程序不会从项目目录命中，子进程 cwd 恒为 home，目录/context_key 不依赖父目录 | 已通过，见第5–6节 |
| A02/A07 | 同 home 的同目标多 Agent/模型共组，目标/adapter 不同分组；远端 selector 原值不自动合并，新选择使用目录 ID；项目非模型键/摘要保持，各上下文资格不合并；目标规则只有中性实现，无新持久对象或包装层，前端旧归组算法删除 | 已通过，见第5–6节 |
| A03/A06 | 实际查询输入/已观察身份改变不复用另一上下文成功；未观察到的远端重启沿原刷新/freshness，不承诺实时发现；远端 A 切 B 不选中 A 的 profile；Orca 目录查询不执行 transport/agent_launch，未验证执行参数及默认目录保留范围说明；取消/过期/失败/迟到结果正确，切项目不串资格 | 已通过，见第5–6节 |
| A04/A05 | A 项目缺材料仍显示目标且提示未接入，B 项目可独立检测；项目失败不改变全局探测；目录成功不放行未就绪项目，目录故障不改 Current/报告且原活动仍可停止 | 已通过，见第5–6节 |
| A05 | 原材料失效、Current 竞争、迟到成功、同目标跨项目占用、原请求/停止封存与候选事务反例仍通过 | 已通过，见第5–6节 |
| A06 | 全局执行配置页无项目可用；原项目环境路由仅文案最小变化，开始/停止/失效、任务定位/返回、未保存草稿、配置条件保存及待重启表达保持 | 已通过，见第5–6节 |
| 全部 | 计划所选 owner/Web、L3、固定 L1/L2 及结构清理完成；实际命令、源码和证据完整 | 已通过，见第5–6节 |

## 2. 执行与证据

实施使用 Plan 的独立 scope，记录每次完整源码 SHA/dirty 状态、命令、选择和原始输出绝对路径，失败与补测不覆盖。Web 浏览器运行前将 Playwright 输出指定为该次独占目录，例如：

```powershell
npm --prefix web run test:e2e -- configuration-navigation.spec.ts --output <独占绝对目录>/test-results --reporter=line
```

浏览器场景沿原受控服务器与 API 拦截，证据不代表真实 Runner/模型成功。无新增人工验收窗口；本轮仅固定元数据 cwd 及不依赖父目录的程序解析，不改原生 RPC、transport 调用或执行协议，因此不新增真实模型/PR L4。实现超出此边界时先更新 Plan 和本记录，再执行新增验证。

## 3. 方案编写阶段文档检查

2026-11-02，本轮 `python scripts/check_doc_links.py` 通过：640 个 Markdown、4014 个本地导航链接、0 错误；`git diff --check` 通过。静态核对所选四个既有测试 ID 的级别/环境及两个 L3 的 suite manifest，一致；七项验收 ID 齐全，三份新增文档 UTF-8、文件末尾换行及行尾空白检查通过。没有业务测试、原生检测、服务启动或部署结果。

## 4. 独立方案审核（2026-11-02）

独立只读子代理 `/root/review_decoupling` 对照 Spec/Plan、实际配置/目录/目标规则和 Web 启动链，确认三项 P2 及一项身份措辞问题；主代理核实后修正文档，未修改业务代码：

- 收窄 profile 承诺：Orca 只查询现有目录 RPC，不执行 transport/agent_launch；未验证 provider/权限/启动参数继续 context_unverified，避免为文档承诺新增探测链路。
- 收窄缓存承诺：只隔离实际查询输入与已观察身份；未观察远端重启沿原 freshness/显式刷新，不新增每次读前 RPC 或监听。
- 补隐式 cwd 边界：直接 Web 启动可继承项目工作目录；计划显式将所有元数据子进程 cwd 固定为 home，并增加双项目父 cwd 的独立进程 fixture，避免只检查显式项目端口依赖。
- 明确远端 selector 保留原值，不假称已规范化为注册 ID；新目录选择沿原 ID，原项目键和占用语义不变。另明确全局页删除被替代的前端归组算法，避免重复规则。

首次修订复核又确认一项 P2：父进程 LookPath/Stat 仍可能依赖项目 cwd。方案补为目录查询拒绝含目录的相对程序、裸命令只查绝对 PATH 目录项，并对同一绝对程序取证与启动；fixture 增加双父目录同名不同程序反例。总计四项 P2 及一项身份澄清已按上述最小范围修订，不引入通用 resolver 或修改项目执行路径契约。

最终修订版经同一独立子代理重新读取 Spec/Plan/本记录并对照代码复核：四项 P2 及 selector 澄清均闭合，未留必须修复项，未发现新增过度设计或结构债要求。复核仅证明方案边界、可实施性和测试映射；该阶段仅完成方案审核，业务当时待实施/待验，后续结果见第5–6节。

修订后再次执行文档检查：640 个 Markdown、4014 个本地链接、0 错误；git diff --check、四个既有测试 ID/两个 L3 manifest 映射、七项验收 ID 及三文档 UTF-8/末尾换行/行尾空白检查均通过。只修改文档，未启动服务、模型或原生探测。

## 5. 实施与验证（2026-11-02）

受测基线为上述完整 SHA 的 dirty 工作树；未提交、部署或操作安装/保留现场。实现把中性目标规则移到 executionconfig，Snapshot 读与保存均派生 environments，Web 直接消费并删除原归组算法；项目配置投影先于存储/材料判断，原项目 Key/Current/摘要/占用语义保留。metadata 查询显式固定 home cwd，程序只从绝对配置或绝对 PATH 项解析，取证/启动同源；Orca 原目录 RPC 与 Runner 请求/信封未改。

证据 scope：`C:/SteerlineDebug/wt-68cd-runtime-project-environment/`。下列叶目录均独立，完整命令/源码 dirty 状态在各 summary 或原日志；表中 owner 统计为顶层测试数，子测试结果亦保留。

| 范围 / 映射 | 实际入口与结果 | 证据（相对 scope） |
| --- | --- | --- |
| P1 owner；A02/A07 | test-feedback 执行 executionconfig、environmentadaptation/...、storage/sqlite；126 项通过，36.2 秒 | `diagnostics/20261102034103398-core/summary.json` |
| 项目 owner；A04/A05 | test-feedback 执行 internal/app -run Environment -count=1；41 项通过，264.1 秒，含缺材料/源失败/关闭存储仍保留目标 | `diagnostics/20261102034543563-app-owner-2/summary.json` |
| Web owner；A01/A03/A04 | test-feedback 执行 web/server/... -count=1；58 项通过，9.8 秒，含双父 cwd、绝对 PATH、相对程序拒绝、绑定错误及元数据失败保留分组；nativequery 无独立测试文件，由实际调用点进程 fixture 覆盖 | `diagnostics/20261102035303261-web-owner-3/summary.json` |
| Web；A06 | scripts/test-web.ps1：构建、31 文件/229 项前端测试及 HTTP/client 回归通过 | `diagnostics/20261102034816120-web-checks/`；其 server-tests.log 指向 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261102T034935.173465500Z/summary.json` |
| 浏览器；A06 | configuration-navigation.spec.ts 两场景通过，7.6 秒；桌面/375/900 宽度、任务往返、错误缓存、草稿与零项目入口；已查看窄屏截图 | `diagnostics/20261102035303261-browser-2/test-results/` |
| 固定 L1；全部 | l1.minimum complete=true/passed=true，执行 16.3 秒 | `test-results/L1/20261102034816120/summary.json` |
| 固定 L2；A05 | smoke.delivery passed=true/cleanup_complete=true，执行 126.0 秒 | `test-results/L2/20261102035016477/summary.json` |
| 定向 L3；A04/A05 | testing.project-environment-readiness 单场景 passed=true/cleanup_complete=true，263.7 秒；同批共享场景因缺项目资格前提失败，整体不算通过 | `test-results/L3/20261102035411054/summary.json` 及对应 scenario.json |

失败和修正保留：首轮 app owner 因抽离后无用 import 编译失败，修正后通过（`diagnostics/20261102034253064-app-owner/`）；前两轮 Web owner 暴露原夹具缺必填 access_mode、以及仍使用当前 run schema 已不接受的 github_auth，分别补必填项并以合法可选字段验证原值保留，第三轮通过（原目录 `20261102034216788-web-owner`、`20261102034543563-web-owner-2`）。没有放宽生产 schema。首轮浏览器 1 失败/1 通过：原断言仍找“验收所用方案/提交验收”，与基线页面的“检测”文案不符；更新断言后两场景通过，原截图/trace 在 `diagnostics/20261102035016477-browser/`。初次前端构建因未安装依赖找不到 tsc，按现有锁文件 npm ci 后正式 Web 检查通过，依赖版本/锁文件未变。一次组合 L3 命令因 PowerShell 逗号参数解析失败、未启动测试；引用参数后另开目录执行。

本轮固定提示词、模型可见材料、Runner 输入/结果契约及其黄金样本/CI 均无正文变化，不涉及新增模型指令或项目入口接线。真实模型、原生 Orca 执行、外部 PR 和部署未选择，不将元数据 fixture/受控结果作为其成功证据。独立代码审核及后置修复见第6节。

共享 L3 的原失败已结算且 cleanup_complete=true：`runtime.shared-runner-projects` 在两项目均无 Current 时直接 start_task，公开回执为 environment_not_ready，尚未进入共享执行。核对基线测试没有调用公开项目检测，CreateTasks 只创建任务/材料；测试现补为两项目先各自 WriteControlledEnvironment，再通过既有 ValidateDeclaredControlledEnvironment 取得原 Run 停止/结果封存证据后开始业务。同步原测试 ID 的机器/可读清单说明，生产 Qualify 和执行门禁未改。共享场景独立复验及固定回归结果见第5.1节和第6节。

### 5.1 最终自动验证补记

共享场景修正前提后的正式复验 `test-results/L3/20261102040007020-shared/summary.json`：passed=true、cleanup_complete=true，执行 220.9 秒。两项目各自公开环境 proof、独立请求/Run/仓库/PR 绑定、整体重启前后零新增业务输入和结算证据均在对应 scenario 子目录。原组合 L3 失败报告保留；只将其中项目资格场景的独立成功与本次共享场景成功分别计入验收，不把原组合整体改成通过。

最后测试修改后的固定 L1 为 `test-results/L1/20261102040039058-final/summary.json`（complete=true/passed=true，执行 9.3 秒）；固定 L2 为 `test-results/L2/20261102040117097-final/summary.json`（passed=true/cleanup_complete=true，执行 104.0 秒）。此为独立审核前的正式自动结果；新子代理 `/root/implementation_review` 的审核及后置修复复验见第6节。

最终静态检查暂计 640 个 Markdown、4016 个本地链接、0 错误，git diff --check 通过。`internal/prompts`、Runner contract/client 及 CI 没有实际正文差异；environment-invoke.md 为 412 → 412 UTF-8 字节且逐字节相同。业务依据为仅调整中性目标归属与元数据观察边界，无需新增模型规则；原请求冻结/项目入口/结果信封对应 owner 与固定 L1/L2 已验证，独立实现审核亦须核对该边界。

## 6. 独立实现审核与修复复核

新的独立只读子代理 `/root/implementation_review` 对照实际工作区差异（含新生产文件）、Spec/Plan、测试及正式报告完成审核，确认一项 P2：无 profile 的远端目录查询允许空 environment，可能按 Name==空字符串匹配唯一未命名注册项。现已在非枚举 agents/models 请求中于读取配置和发起 RPC 前拒绝空远端 selector；新增未命名注册项负例，且明确 environments 枚举仍返回该 ID，不增加身份解析层或 RPC。

子代理已重新读取修复和测试，确认该 P2 闭合，未留必须修复项；中性 Target 保留原表示，项目 Key/摘要/Current/Qualify/占用/停止无放宽，Snapshot 读与保存都投影分组且前端删除重复算法。其亦核对原 fixture/schema 与共享场景前提修正，未发现掩盖生产回归或新增配置/资格/框架的问题。审核者没有修改文件或另跑业务测试。

修复后的 `diagnostics/20261102040715746-review-web/summary.json` 为完整 Web owner 58 项通过（17.6 秒）；`test-results/L1/20261102040715746-review/summary.json` complete=true/passed=true（执行16.3秒）。修复仅影响远端输入校验，前端/项目执行/停止协议未变，不重复无影响的浏览器与 L3。最终 L2 `test-results/L2/20261102040801730-review/summary.json` passed=true/cleanup_complete=true；所选验证与独立审核至此全部完成。

最终结论：RPE-A01–A07 的所选 owner、Web、受控浏览器、L1/L2 和两项 L3 均已验证，独立审核问题已修复并复核闭合。目录观察与项目资格保持分层，无新增全局资格或持久对象；只读探测不被当作真实模型/原生执行成功。文档/清单静态检查通过；用户随后授权将实现、Spec/Plan 与本验收记录一起提交、推送并创建 PR，实际提交与 PR 身份以 Git/平台记录为准，未部署。
