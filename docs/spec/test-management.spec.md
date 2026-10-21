# Runner generate/1 对接与最简测试管理 Spec

日期：2026-10-19；2026-10-20 对齐 Runner 修复交接。状态：实现及自动验收已完成；Runner 内容接管误校验已修复，真实 A/B 接管与干净主干同构建复验均通过，人工体验后置。实际版本和各自验证范围见专属验收。Runner 对接和测试管理合并为一套 Spec/Plan。配套 [Plan](test-management.plan.md) 与[专属验收](../acceptance/test-management.md)。普通开发，不分配 Task ID，不修改 roadmap。

## 1. 目标、现状与范围

在“执行与待处理”的当前候选详情中，看到固定 Plan 选择了哪些测试，以及当前被测代码版本下的通过、失败和待验证情况。Plan 旁的测试安排由 SteerLine 调用 Runner `generate/1` 生成；SteerLine 不实现内置 Agent 运行时。

SteerLine 调查基线 `4c5409f398585b9fd3623d625d4e2d0563efec13`。实施固定 Runner PR #33 合并提交 `9586ac4dee53882ba4349710e94c950cf47fd8db` 的公开 generate/1 契约；生成与报告出口是不同前置，实际版本和未交付边界见验收。

| 当前实现 | 本次整合 |
| --- | --- |
| `runnerclient` / `runnercontract` 通过公开 CLI 调用固定 Runner；应用 home 共用一个服务 | 支持内容流程公开输入/schema；测试安排使用同一个服务的 `generate/1` |
| 当前 `RunInput` 要求 repository、Spec/Plan、publication，catalog 校验还面向开发流程 | 按 Runner 发布的流程分别验证，不给生成伪造开发材料，不放宽开发候选准入 |
| Task 的 PlanRef 已固定路径和 commit；common/acceptance refs 的校验面向 Markdown | 从 PlanRef 推导同目录测试 JSON，在同一 commit 读取并固定所选定义；不修改 Task 契约 |
| `tests/e2e/suites.json`、`cmd/test-feedback` 已负责场景选择和原始报告 | 复用 ID、选择逻辑和结果证据，补展示清单与最薄结果输出 |
| Web 已有候选详情、Runner 材料和结果读取；没有测试管理闭环 | 在原详情增加只读区域，沿现有 API / Query 接线 |

同一交付包括：`generate/1` 的必要客户端接入和生成用例、项目清单、Plan 测试安排、实际结果输出与公开回传、持久化投影、执行页展示。Runner 可以发布 `conversation/1`，本仓本批不为它实现消息、历史、中断或会话业务接口；规划台实际开发时再接入。内容流程不能被当作开发候选预设启动。

不做独立测试管理页、清单编辑器、页面运行按钮、自动调度/补测/修复、PR 门禁、完整规划台、完整工具流、通用测试框架插件或旧格式兼容。不修改开发流程、TaskDelivery 或后继释放条件。

## 2. 所有权与跨仓前置

| 所有者 | 职责 |
| --- | --- |
| SteerLine 测试业务用例 | 固定材料、构造生成指令、校验测试引用、保存文件、计划关联和结果汇总 |
| Runner | 原生执行、输出格式、输入/轮次来源、会话/权限/取消/恢复及资源结算；通过公开出口提供执行事实 |
| 项目测试入口 | 实际执行断言并输出结构化结果及原始证据 |
| Web | 展示服务端已提交投影，不读取项目文件、解析 Agent 自述或独立判断通过 |

共用现有 EXE pin、实例绑定、state-dir 和认证来源。生成请求从指定本机 Codex profile 复用可执行文件、模型和推理强度，固定 `workflow=generate/1`；按 Runner 正式 schema 构造最少 execution 设置，明确使用内容流程的 `read_only` 权限。不得原样复制开发 profile 的 sandbox/full_access、Git 发布认证、审计或提示词配置，不修改原 profile。这里只做用途明确的字段映射，不重建权限编译器。缺少适用 profile 或能力时明确不可用，不自动改投其他主机或 Agent。

跨仓前置如下；契约固定后可受控开发，真实交付还需实际构建与联验：

1. **生成契约前置 R1**：已固定 Runner `9586ac4` 的 generate/1 公开 capabilities、输入/schema、结果来源及停止语义：submit v1、generate 输入、inline 材料及 read_only。本仓按该公开契约建设 fixture 和客户端，不从 Runner 私有实现拼接口，也不等待 conversation 的 UI/交互能力。
2. **报告出口前置 R2**：Runner PR #34（合并 `a259c3a`，实现 `7325a87`）已交付 `artifacts discover/collect/list/read` 与 `workspace-git`；R2 历史固定主干 `bf3ec58` 保留这些能力。正式契约见 [Runner 接入说明](https://github.com/erranliu/SteerRunner/blob/bf3ec581cb0b23a7358d95e5efa8ff5f607c2f55/docs/run-evidence-r2.md)。产物位于原工作区 `.steerrunner/artifacts/`；collect 是带原 request_id、expected_revision、relative_path 的写操作，read 按 Run/artifact ID 返回原字节 base64、SHA256 和归属。Git 查询须检查 verified 及可空 commit/clean。项目测试程序记录执行编号、顺序、实际选择和 Git 来源，SteerLine 解释测试定义、计划与通过条件；Runner 不理解测试 ID 或签发额外测试证明。SteerLine 已实现消费接入，不能再标为 Runner 尚缺接口；实际通过和恢复限制只在验收登记。
3. **联验前置 R3**：取得实现 R1/R2 的实际 Runner commit、EXE version/SHA、resource digest、自包含 schema 与同构建定向证据。受控开发不必等待 R3；真实接入、功能交付必须满足 R3，不能拿移动工作区或旧 EXE 冒充。

不导入 Runner Go 包，不读取其 SQLite、原生文件、私有日志或认证内容；不直接启动 Codex。新流程缺能力明确返回 unsupported，不影响原有合格开发功能，也不回退本地 Agent。

## 3. 两份简单文档

### 3.1 项目测试清单

固定路径 `docs/testing/test-list.json`，根对象只有 `tests` 数组。每项固定六个必需字段：

```json
{
  "tests": [
    {
      "id": "smoke.delivery",
      "name": "核心交付流程",
      "level": "L2",
      "environment": "controlled",
      "command": "go run ./cmd/test-feedback -level=L2 -env=controlled",
      "check": "从领取到交付完成核心闭环，实际选择完整且必要断言和清理全部通过。"
    }
  ]
}
```

示例只说明格式，不替代源码中实际命令核验。级别遵循[通用规范](../../SteerlineProjectGuideDoc/testing-architecture.md)，环境名称由项目定义；不在产品中硬编码本仓全部环境。可选 `note` 仅用于本项特殊约束，公共所有者/隔离/证据要求沿项目测试规范维护。

一项为可独立运行并报告的入口，可代表一组测试。只能报告整体结果的集合不拆成虚假的逐场景通过。已有 E2E ID 和命令选择与 `suites.json` 一致；清单不代替执行登记。没有内容的级别沿规范写明未登记，界面不推断覆盖。

解析拒绝未知字段、重复 JSON 键、重复/空 ID、非法级别或空必需内容。新增、删除、改级、改环境、改入口或改主要断言时，同步实现、执行登记、清单及受影响的 Plan；ID 不静默改作其他目的。计划内新增测试可先登记目标入口/通过条件，并在 Plan 安排落实；文件存在不表示入口已经实现或测试已经通过。

### 3.2 Plan 测试安排

2026-10-21 当前维护规则见[项目维护 Spec](project-maintenance.spec.md)：初始化/基线/局部共用材料规则，首次启动固定最新代码与同 commit 材料；缺失/无效或必要核对未完成阻断相关首次启动，有效空安排允许。后续候选保留唯一 Task 快照；不增加测试通过/PR/Delivery 门禁。原 TM 历史验收不因此重标，本次差异归 PM 专属验收。

`<name>.plan.md` 对应同目录 `<name>.tests.json`，固定结构：

```json
{"tests":["smoke.delivery"]}
```

只保存有序、唯一、可解析的 ID，不另加模型、结果、实时状态或重复测试定义。Plan 正文链接该文件，保留选择原因、验收映射、执行时点和新增能力安排。允许空列表，但 Plan 必须说明无需自动测试的原因；空列表展示“未安排自动测试”，不显示全部通过。

生成/修改 Plan 时同步生成安排。当前没有完整 Plan 生成入口，本批提供针对已有 Plan 的显式 CLI 用例，不建设规划台或文件监听。发现输入变化时提示重新生成；服务无法观察的仓库外部编辑由显式生成/材料固定时复核，不承诺后台自动同步。

不新增 `tests_ref` 或 Task 创建参数。候选准备时从 `PlanRef.Path` 推导测试安排路径，并在 `PlanRef.Commit` 读取它及固定路径清单；将安排、所选定义及摘要与原候选预留一并保存，把实际存在的文档附加到 Runner references。首次启动前缺必要文档直接拒绝预留；不把不存在的引用送给 Runner。JSON 走专用业务校验，不塞入 Markdown common/acceptance refs。相同 Task 的候选沿同一 Plan 版本，不因工作区移动文件改变计划。

测试 JSON 缺失时展示“Plan 未关联测试安排”；存在但损坏、引用缺失或清单不完整时保存诊断，展示“测试安排无效”。这些情况阻断相关 Task 首次启动，不改已冻结候选、DAG 或交付；不伪造总项数或全部通过。首版不回填历史候选：没有已固定快照时显示未关联。已有固定计划需要修改时仍遵循原任务材料规则，材料确认入口归统一维护。JSON 只有 ID，不能机械证明 Plan 正文语义已经同步；由正常文档审阅负责，不增加模型二次审计。

## 4. generate/1 生成用例

当前入口调整：统一维护复用原请求/profile/公开 Runner/取消接管；单 Plan/单次结果先保存为待审阅输出，CLI/Web 显式单文件应用后才写文件。新增 `tests apply`，不保留自动保存旁路，不建批量/多文件事务。完整契约见[维护 Spec](project-maintenance.spec.md#生成控制与产品入口)。

提供项目级 CLI `steerline tests generate -project <id> -plan <repo-relative.plan.md> -execution-profile <local-profile> -request-id <id>`；用 `steerline tests generation -project <id> -request-id <id>` 查原操作，`steerline tests stop-generation -project <id> -request-id <new-id> -generation-id <original-id>` 显式停止。请求号为 32 位小写十六进制；参数与 CLI 帮助及测试保持一致。

CLI 走应用服务与原项目用例，浏览器维护动作走原 `/commands`。调用方指定请求号，服务持久化原材料、profile execution、Runner 信封/请求映射、目标文件及目标原内容条件后才提交。复用现有项目请求/store 和共享客户端；允许薄业务模块，不建立第二套 outbox 框架、常驻 Agent 服务或执行 actor。

首版使用项目既定的 `docs/testing.md` 作为规范，不增加角色发现或路径配置系统。输入为 Plan、该规范和清单的内容快照，不自动搜寻 Spec。生成指令要求按 Plan 的测试安排选择入口，不增加无关全量要求；输出 schema 只允许 `tests` 唯一 ID 数组。Runner 校验格式，SteerLine 再对冻结清单验证 ID；非空但选错/漏选仍属于正常 Plan 审阅范围，不宣称程序已经证明语义覆盖。模型返回空列表时不自动覆盖原文件，返回结果供作者核对；明确无需测试的作者可手工写入空列表并在 Plan 说明；应用模型空结果还需显式空安排确认。不得用生成模型分析运行日志来决定测试通过。

生成结果只供审阅。查询返回目标 diff/review_digest；显式 `tests apply` 或 Web 应用按输出、当前 HEAD/分支、实际完整输入和目标原字节重算，先持久单文件意图，最后原子替换。取消/应用串行，丢回执与重启只核实，未取消且原条件成立时可显式继续原应用；原 generate 重放不写文件或重发模型。条件变化后需要重新审阅，适用输出可复用，无须新模型调用。Plan 链接由作者维护，用户正常提交/合入后核对远端材料；正式启动采用同一最新 commit 的引用和测试。

接受不等于执行成功；结果成功和资源结算分别展示。未知提交、停止、迟到接受及重启沿[生命周期规则](../product-design/worker-lifecycle.md)处理：只查原请求，零自动重发、零自动 resume；停止义务必须覆盖未绑定/迟到 Run。新生成对象纳入 home 共享 Runner 的停服/重置结算，不因其没有 Candidate 而漏掉。无 Task 领取、开发候选、DAG 或 TaskDelivery 副作用。

## 5. 测试结果与版本绑定

每个登记入口由真实执行器或薄包装输出以下四字段；多个结果用数组承载，不要求底层每个单元测试登记：

```json
{"test_id":"smoke.delivery","status":"failed","message":"交付断言失败","log":"本次公开证据引用"}
```

`status` 只有 `passed`、`failed`、`incomplete`。正常完成且所选范围/通过条件/必要清理和证据齐全才通过；实际断言失败为失败；环境不满足、中断或缺证据为未完成。零退出码、零匹配、意外跳过、编译成功、模型自述不能代替通过。通过项的 message 可简短，失败/未完成须说明原因；log 指向真实证据而非随意本机路径。

项目入口→Runner 从原工作区收集公开报告→SteerLine 已绑定候选的消费→保存→产品查询是唯一结果链路。SteerLine 不接受任意候选上报接口或扫描私有目录。报告使用一个最小公共封套记录本次执行 ID、顺序、实际选择、所选定义摘要、被测 commit、测试前后 clean 状态及原始日志引用，四字段结果放在封套内；业务封套由本项目测试规范维护，R2 仅规定通用产物契约。复用 test-feedback 既有命令、summary 和版本信息；只补缺少的关联字段。运行内输出显式指定为 `.steerrunner/artifacts/<本次执行ID>/` 下的新目录，项目维护其忽略规则，避免产物本身使 clean 失效。Runner 不修改项目 ignore。封套由测试程序输出，Runner 提供原 Run 归属及不可变产物，SteerLine 保存计划/候选关联并校验定义和环境/选择。不要求每个结果重复填写元数据或建设防篡改证明体系。

首版只把测试前后均为干净且同一 Git commit 的结果计入当前通过；测试输出写入仓库外或既定忽略目录。脏工作区运行、前后 commit 变化或无法确认 clean 状态时保留实际结果，但不计当前通过，显示未完成/待复验，不计算脏文件指纹或建设快照库。当前 commit/clean 来自 Runner 已有公开工作区事实；若字段不足，仅补通用 Git 观察投影，不能用 PR SHA 或调用方本地目录猜测。当前源码来源未知时，过去结果只作最后观察，不继续显示当前通过。此规则不声称观察采样能排除测试期间短暂修改后还原，首版不建设持续源码监视或防篡改机制。

每次补测独立保存。以原 Run+测试运行/记录身份幂等，原报告相同回传不重复，冲突回传拒绝；查询时按来源定义的运行顺序取最新，不能按到达时间让迟到旧通过覆盖新失败。清单语义、计划或代码变化后的记录保留历史，不计入当前通过。无报告不生成一个假的 failed；本地超时也不伪装测试断言失败。

本仓首批只适配 `cmd/test-feedback` 的 L1 和现有 L2/L3/L4 入口；按实际 level/suite/scenario/environment 映射登记 ID，L1 使用一个整体 ID。已有参数、执行逻辑和退出码保持，`-out` 等证据路径参数不改变测试定义；定向子集或 gate-only 不能被映射成完整登记项通过。测试清单覆盖该范围，已有 Web/脚本测试继续在原规范列明未接入本功能，不在本批为所有入口建设包装。其他项目由自身入口实现同一报告格式。测试规范维护格式与同步规则，测试安排不写运行结果。

## 6. 执行页

当前候选详情增加“计划测试”：计划总项数、当前通过/失败/待验证数。待验证汇总包含无有效结果、未完成和待复验；行上分别标明原因。各项计数互斥且总和等于计划数，空列表不算全绿。

每行显示名称、级别/环境和状态，展开命令、check、message 及日志引用；历史结果折叠显示。跨版本旧通过标待复验；当前同版本最新失败/未完成不能被早先通过掩盖。离线/来源未刷新显示最后观察与待核实，不把缓存解释为当前事实。通过数只表示所选入口，不能表述为全项目或全部级别通过。

沿 `product/api/query`→HTTP allowlist→Web typed API/Query 接线，查询范围包含 home/项目/实例/候选；切换、迟到响应与隐藏区域轮询沿既有规则。不从浏览器执行命令或打开任意 file:// 路径；本机/远端证据使用服务核验的公开引用或摘要。

## 7. 验收要求

通过条件唯一编号，实际结果记录在[专属验收](../acceptance/test-management.md)，测试命令与证据映射见 Plan。

| ID | 自动验收范围与通过条件 |
| --- | --- |
| TM-A01 | generate 公开输入/能力支持，无假开发字段；本机 profile 到只读生成配置的映射正确，开发 Task 准入及配置隔离保持，缺能力无回退 |
| TM-A02 | 清单/安排严格校验、已有入口映射、从 PlanRef 同提交推导与固定所选定义正确；空/缺失/无效安排分别展示；缺失/无效阻断相关首次启动 |
| TM-A03 | 生成使用原持久请求和完整固定材料，先待审阅；显式应用核对 review_digest 和 revision，丢回执只核实，原应用继续零重生成 |
| TM-A04 | 取消/迟到/未知/重启只对账，未绑定生成也纳入停止和结算；持久化失败零提交，零 Task/DAG/交付副作用 |
| TM-A05 | 实际测试报告转换与公开产物回传接通；状态/环境/选择/日志校验正确，零匹配/跳过/缺证据及前后 Git 状态不合格不冒充当前通过 |
| TM-A06 | 补测不可变、幂等/冲突/乱序正确；候选、计划、定义、源码和项目隔离，历史通过不覆盖更新失败或未知来源 |
| TM-A07 | 正式 API 和执行页汇总/详情/证据/历史正确；空/缺安排、待复验、离线、切换与迟到响应不串结果，不新增调度或测试通过门禁 |
| TM-A08 | 固定真实 Runner/Codex 通过正式 CLI 完成一次 generate→审阅→显式应用；通过、失败测试经公开结果→正式 API→真实浏览器执行页闭环可见，并核验本包停止/资源结算 |

TM-M01 为可后置人工体验：执行页密度、中文文案、折叠和日志可读性；不阻塞无关开发，不能豁免 TM-A01–A08 的真实失败或跨仓前置。
