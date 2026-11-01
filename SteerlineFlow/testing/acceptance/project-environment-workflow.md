# 项目 × 运行机 × 链路环境准备：审计与验收安排

2026-10-31。普通开发，无 Task ID。[Spec](../../spec/project-environment-workflow.spec.md) · [Plan](../../plans/project-environment-workflow.plan.md)。状态：代码与界面已实施，自动工程验收PEW-A01–A13全部通过；真实生成G09和正式本机验收V05及完整清理已核实。PEW-M01人工体验待走查。不进入 roadmap，不修改安装现场。2026-11-01用户后续要求提交功能PR；不部署或自动合并。

调查源码：`e1968de2d8031c834bb5f35dff8a37a0a41b6a62`，实施阶段的验证基于其上的未提交工作树，实际diff/源码状态由各次报告保存。第 1 节是文档阶段本聊天自审；后续提示词及接线按 AGENTS.md 要求由独立只读子代理 `prompt_review` 审核，见第 7 节，不混同两种审计。

## 1. Spec 审计与修订

| ID / 优先级 | 原问题及影响 | 文档修订与后续验证 |
| --- | --- | --- |
| AUD-01 / P1 | 首次生成要求未明确排除待生成输出；上下文又称包含固定提交，若把其所在 SHA 写入文件会形成自引用维护循环 | 明确只读上下文预览不依赖输出存在，生成文档/脚本可缺；文件不写自身提交、时间或摘要，由 repository.commit 与活动单独冻结来源。Spec 4.2/5，Plan P1/P2；A03/A12 必测首次启动 |
| AUD-02 / P1 | “规范执行上下文”包含完整 settings、args/transport 等字段，与不得导出秘密要求冲突；仅文字禁止秘密不足以约束直接序列化 | 明确后端完整上下文与公开字段投影分离，公开允许字段及未知字段缺口策略；违规前轮阻断导出，不篡改原报告。Spec 4.2，Plan P1/P2；A06 敏感字段反例 |
| AUD-03 / P1 | 文档新增环境绑定/脚本目录但没有固定 manifest 格式，也没规定生成后校验；错目标输出可能被标成功或用旧 parser 处理 | 明确 manifest2 字段、仓库根相对路径、禁止跨环境依赖和严格输出验证；坏结果保留但不可用，gaps 明示。Spec 4.2/5，Plan P1–P3；A03/A04/A06 |
| AUD-04 / P2 | 项目历史原实现先截20条；前端再筛环境会丢掉所选环境记录；上下文预览也未有可达协议落点 | 定义材料只读路由及 environment_key 历史查询，在截断前过滤；options 不携所有正文/报告，贯通应用/代理/allowlist。Spec 6，Plan P2/P4；A09 用其他环境超过20条反例 |
| AUD-05 / P1 | 新目标有效性验证若先于原请求重放，会使已删除环境的原活动无法对账；新增目标字段也未落到不可变存储约束 | 明确原信封优先，生成/验收独立输入，新增目标及来源只写一次；全量生命周期枚举不套 UI 筛选。Spec 6/7，Plan P2；A07–A09 |
| AUD-06 / P2 | 前轮事实选择未限制已结算，导出期间持续变化会使上下文难以固定；“尚未提交”常驻提示可能与用户实际提交矛盾 | 只取同键最近已结算有轮次活动；构造函数唯一、导出后变化明确刷新；正式材料按 Git 读取，不永久断言用户未提交。Spec 4.2/5，Plan P1/P4；A04/A06/A07 |
| AUD-07 / P1 | 真实验证仍使用“纯页面变更”措辞，实际已改固定提示词、文件布局和生成业务，仅受控验证无法证明实际模型读对目标或脚本可用 | 新增 A13 明确真实本机生成两个目标及新格式本机正式验收；独立正文审核、L1/L2、受控闭环及最小 live 分开。Spec 8，Plan P3/P5/验证表；不宣称所有原生路径通过 |

七项已在文档层修订；是否在实现中满足，要由下列验收验证。本轮未发现需要新增环境注册表、Runner 接口、安装器或通用文档平台的理由。用户后续已授权按此 Spec/Plan 实施；按完整环境键分目录、生成上下文经 Git 固定及前置操作提示均已纳入实现与验证。

## 2. 验收范围与准备

验证“选择项目下的运行机/链路→固定目标上下文→选择生成方案→生成材料→正常 Git 固定→用户准备→选择验收方案→原验收与门禁”的对象连续性及材料隔离。资格算法、三轮预算、TTY/完整 L2/原 PR发布/停止封存、外部 L4 合并与资源所有权沿现行规则。

自动环境以 Plan 第 4/5 节为执行安排：Go owner/原公开 Runner fixture、独立临时 Git/SQLite、受控应用 HTTP/浏览器及固定 Runner 的最小真实 Codex 范围。原始产物使用仓库外独占 scope、新输出目录，SHA/构建/命令/时间/退出码/清理在执行时固定；实际构建与测试结果见第 6 节；未执行部分单独保持待验。

## 3. 自动与独立审核项目

| ID | 方法 / 步骤与 Plan 映射 | 最小通过条件 / 阻塞范围 | 状态 |
| --- | --- | --- | --- |
| PEW-A01 | P1/P4；model/app/Web及浏览器，七方案四环境及两项目夹具 | 原键合并模型、区分入口/权限/目标；不硬编码数量；失败阻塞对象及界面交付 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A02 | P2/P4；生成真实输入断言及 UI 操作 | 本机生成方案为远端目标产出，模型变化不换目标；失败阻塞生成交付 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A03 | P1–P3；缺输入、坏目标/路径/manifest、首次预览与生成 | 必需规则缺失零 Runner 新提交；首次无输出可用，无自引用/inline 兜底；失败阻塞生成与验收 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A04 | P1/P2/P5；两项目两环境，材料/前轮/生成历史隔离 | 不互相覆盖、不借前轮，不跨项目；失败阻塞资格安全 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A05 | P1/P2/P5；变更 A/B 专属材料、共享依赖、模型/上下文 | 按有限依据正确失效，读取与候选事务同判定；失败阻塞派发 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A06 | P1–P4；坏输出/合法 gaps/正式材料对比，敏感字段导出反例 | 错目标输出不可用，合法导出不自动授予或撤销资格，允许字段投影不泄露原 settings；失败阻塞材料交付 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A07 | P2/P4；任务跳转返回、切环境/项目/实例、双击/迟到回执 | 模型选择各自归属，零串对象或重发，不自动换默认；失败阻塞操作交付 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A08 | P2/P3/P5；原预算/未知/占用/停止及固定门禁受控回归 | 缺 TTY/L2/PR/收尾等拒绝，开始撤旧资格，失败不回退；失败阻塞业务交付 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A09 | P2/P4/P5；真实 HTTP/代理链、先筛后限、移除目标后原请求对账/停止 | 超20条不丢所选历史；新材料读取只读，旧资源义务不漏；失败阻塞查询/生命周期交付 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A10 | P4；原 configuration-navigation 浏览器宽窄屏/键盘/故障缓存 | 环境选择优先、要求到结果可达，状态及来源准确，无模型级环境条目；失败阻塞 UI 交付 | 已验收（owner/受控/Web 范围，见第 6 节） |
| PEW-A11-T | P3；固定模板/同源资产/实际请求断言及字节账本 | 目标路径及来源正确、差异和字节齐全，黄金样本不代替批准；自动部分待验 | 已验收（最终正文测试，见第 7 节） |
| PEW-A11-R | P3；独立于实现作者的最终实际正文及接线审阅 | 固定提示词治理批准；未批准阻塞相关 live 使用及最终交付，不受人工体验后置安排豁免 | 已验收（最终独立批准，见第 7 节） |
| PEW-A12 | P5；扩展正式 readiness L3 场景 | 两环境正式生成→固定 Git→验收→只放行对应项目环境，完整清理；fixture 不冒充真实模型 | 已验收（正式 L3/02，含生成脚本摘要绑定） |
| PEW-A13-G | P5；计划新增真实生成 L4 | 固定 Runner 本机 Codex 给本机及远端两个目标生成有效新材料，原来源/输出/停止核实；远端不需 ready，不执行远端验收 | 已验收（最终10055字节规则，Sol/high，generation-09；原失败保留） |
| PEW-A13-V | P5；既有真实本机环境 L4 | 将 G 的本机材料正常 Git 固定后，完整 TTY/L2/原 PR发布、外部规则/精确合并、准入及清理；失败不得借历史结果通过 | 已验收（verification-05，首轮通过；PR #726精确合并、实际准入及清理核实） |

PEW-A11 在 Spec 中为一项，此处拆 T/R 分别记自动与独立审核；PEW-A13 拆 G/V 分别记生成和验收，不允许以其中一半替另一半通过。没有新生成场景或真实工具时，相关项仍为工程前置待验，不转为 M01。

固定 L1/L2、完整 Web/HTTP及生产构建属于必要回归，按 Plan 记录实际报告，不合并成产品用例替代断言。实际原生结论只覆盖新受测版本已执行的 Codex 路径；Orca 本机/远端 owner 与受控适配通过不等于原生已验收。

## 4. 后置人工体验

PEW-M01，方式：人工；参与人、窗口及受测构建待安排，状态：待验收。可在自动工程项满足后集中走查，不阻塞无关后续工作；已知自动失败不能按人工后置处理。

1. 从当前项目环境列表准确选择运行机/链路，指出当前准备对象和共用方案。
2. 首次无材料时找到目标上下文、Git 维护提示和生成入口，分清生成位置与材料目标。
3. 找到要求和用户准备责任，选择验收模型，解释完整 L2/真实 PR 副作用及重新验收撤旧资格。
4. 查看失败原因、当前/历史报告及项目全部活动，并返回原任务保留原方案。
5. 在窄屏和键盘操作下完成相同辨认及导航。体验走查不要求再次发起真实模型或 PR，真实执行证据属于 A13。

最小通过条件：无需口头补充就能回答“我在准备哪个项目/机器/链路、模型用于什么、下一步是什么、通过覆盖谁”。记录具体反馈和范围，不以一次笼统签收覆盖其他工程项。

## 5. 文档阶段检查（实施前）

本轮仅修订 Spec、编写 Plan 和本专属记录、补两处导航。没有运行产品 L1/L2、Web、浏览器或任何真实生成/验收，不改上述产品验收状态。

文档检查（2026-10-31）：`python scripts/check_doc_links.py` 扫描630份 Markdown、3984个本地导航链接，0错误；`git diff --check` 通过，新增文件另做严格 UTF-8 解码、末尾换行与逐行尾随空白检查。Spec/Plan/验收的 PEW-A01–A13、PEW-M01 映射、五个实施步骤及七项审计修订另外检查；实际计数以最终复核为准。静态检查不代表产品或固定提示词独立审核通过。

## 6. 实施与实际验证（2026-10-31）

实现包括：完整环境键目录及 manifest2；独立生成/验收 payload；冻结材料目标与原请求；公开上下文白名单及同键已结算前轮；按选中环境读取材料及先筛选后截20条历史；不再以当前列表过滤旧活动停止义务；原资格/三轮/TTY/L2/PR/收尾保持。SQLite 当前格式升为37，不迁移或部署安装库。

页面先按运行机选择链路，显示当前对象/状态，再阅读要求；生成输入导出和生成模型位于折叠的生成操作，验收模型只在该环境验收区。没有生成/验收的自动代跑或安装器。用户原暂停/默认方案、任务返回、失败缓存及独立原活动停止保持。公共项目要求为 environment-common.md，根 environment.md 仅作人工导航；旧全局机器段不作为新请求输入或运行时回退。

证据 scope：`C:/SteerlineDebug/wt-68d4-project-environment-workflow/`。下表路径相对此根；每次失败与补测分目录，报告包含原命令、受测基线/工作树、退出码及清理信息。

| 范围 | 实际入口与证据 | 结果及边界 |
| --- | --- | --- |
| 后端 owner | `diagnostics/owner-03/summary.json`；environmentadaptation/model、service、app、SQLite 的 Test.*Environment | 96 tests / 4 packages通过，0失败/跳过；覆盖首次无输出、公开字段排除、目标独立、旧历史先筛后限、原预算/未知/停止/资格；模型/强度显示补齐另见diagnostics/readiness-models-01 |
| 固定 L1 | `L1/10/summary.json` | 最终10055字节规则、模型显示及PS5.1外部取证修正后通过；之前L1/03结论仅属当时9013字节版本 |
| 固定 L2 | `L2/06/summary.json` | 核心交付冒烟通过，完整停止/清理；未用此证明原生环境已就绪 |
| 正式准备闭环 L3 | `L3/02/summary.json` 及场景下 first/second-environment-proof.json | 两项目两环境：正常生成→Git固定→直接验收所生成文件→只放行对应组合→失效再拒绝；实际验收 ScriptDigest 与生成脚本一致，原轮次/结果/收尾核实 |
| 全部 Web/HTTP/生产构建 | `web-05/`，PowerShell7宿主运行原 scripts/test-web.ps1 | 完整脚本通过；客户端228项、HTTP owner与构建通过，最终折叠生成操作及顶部状态已纳入 |
| 真实渲染浏览器 | `browser-03/` 与 `diagnostics/browser-03.log` | configuration-navigation 2项通过，任务往返、环境作用域、失败缓存、键盘及375/1440宽度无溢出；截图/trace保留。人工可理解性仍为M01 |
| E2E读取者/执行器 | `diagnostics/edges-01/`（原失败）及 `edges-02/summary.json`（定向补验） | 首轮仅新版多目标夹具和L4可读清单登记失败，其余通过；修复后原失败及机器清单一致性通过 |
| 项目公共脚本 PS5.1 | `diagnostics/tty-01.log`、`publication-01.log` | TTY 5项、publication 2项通过；不把Root共享脚本的通过当作生成脚本已通过 |
| 真实材料生成 G | `L4/generation-01/` 至 `generation-09/` | 第1/2/4轮正式入口/目标/来源/停止核实通过，原gaps和可留存源码保留；产物审阅发现问题后修正上游规则并独立复审，未手改输出冒充验收成功。第3轮原活动失败但诊断保存不足；第5轮在建库前打开证据reader失败，未发起模型；修复取证时序后第6轮9809字节规则通过；最终10055字节规则由Sol/high的G09两目标通过，原请求/来源/停止完整留存。V05完整正向通过，原失败单列第8节 |

首轮默认沙箱执行有 Git config 权限拒绝，未计业务失败；后续使用独立 scope 的受管报告。`owner-01` 的导出测试误匹配测试目录名，改为准确字段断言；`owner-02` 的 generation 生命周期夹具缺新不可变目标，补齐后 `owner-03` 全部通过。L1/01 原源码守卫登记旧函数位置，一对一改为新实际赋值函数并经独立批准；后续 L1 通过，未放宽守卫。

Web 首轮新增测试的异步等待失败保留在 web-01，修复后228项通过；PS5.1包装器曾将成功 stderr回执判为NativeCommandError，原完整检查在PS7宿主通过，业务PS5.1要求未改变。L3/01未执行场景：PowerShell拆分未引用的带点scenario参数；改为显式引用后L3/02通过，不计前次为零命中通过。

真实G固定独立Runner副本的SHA-256为 `326c467d0ce650aca28c54a4f1b68dec616739f4f8ebc6ad699c6ab04bf1dcb4`，版本 `0.2.0-dev.4`。本机Codex与远端目标分别读取显式profile，实际生成只在本机进行。原SteerlineTest源码 `91371fda325f04ce18592da67412a7f8af98ce52`；补齐新公共要求的独立测试源为 `789816e010f741eb92a079b7c1111a9cae51bb2b`，原项目测试/发布标准与PR acceptance保留。没有改安装目录、原业务checkout或业务主分支。

## 7. 固定提示词正文与独立审核

实际完整正文/接线diff为证据根下 `diagnostics/prompt-body.diff`，逐文件UTF-8字节账本为 `diagnostics/prompt-byte-ledger.json`。它们与当前可读源文件共同受审，不以摘要或黄金样本替代批准。

| 正文/代表请求 | 修改前→最终UTF-8字节 | 业务原因 |
| --- | --- | --- |
| environment-generate短模板 | 301→440 | 原全局context/doc定位改为所选环境；明确.ps1/.sh后缀及生成模型不改变材料目标 |
| environment-prepare短模板 | 273→260 | 引用本次固定文档和脚本，不读全局入口 |
| environment-publish短模板 | 260→262 | 同一目标脚本完成原发布核实，不改变PR标准 |
| generation完整规则 | 7476→10055 | 替代旧单份材料/manifest1说明，补目标绑定、新目录与manifest2、公开快照无.git、普通/验证base分离、生成入口接口适配及运行时root/base/PS5.1接线要求，明确初始空pr.base/head的填充来源及PowerShell零项/单项checks/l2的数组形状；禁止改测试标准/修环境/伪造通过保持 |
| execution完整规则 | 5478→5501 | TTY失败入口的固定旧路径改为本次文档声明路径，原失败/未知/停止规则保持 |
| generate实际代表请求 | 294→812 | 带完整环境键的三份材料定位，额外长度来自确定路径而非内联长材料 |
| prepare实际代表请求 | 272→447 | 固定环境键目录文档和脚本定位 |
| publish实际代表请求 | 259→449 | 固定同一环境文档和脚本定位 |

主要实际正文差异包括：`SteerlineFlow/environment-context.json` 改为 `SteerlineFlow/environments/<完整键>/environment-context.json`；机器段 `version=1 + scripts/verify-environment.ps1` 改为 `version=2 + environment_key + 同键固定脚本路径`；TTY失败入口不再硬编码全局脚本。真实G反馈另使规则明确固定文件快照本身无.git、manifest.validation_base是执行时唯一验证目标、仓库根不得用错误父目录层数推断，以及初始空pr.base/head必须由该base和实际分支填充。完整diff包含全部原文，不只保存这些说明片段。

独立只读子代理 `prompt_review` 已分别审阅正文/接线、精确源码守卫位置更新及真实G反馈修订，最终批准10055字节generation规则和保持不变的其他正文。最终generation SHA-256：`9cc9000977a36f1b0c7b477f6e8c94aff43452c129acb48dcb2b50530da2799c`；execution：`63468d146069a9ea0959c0eb454f07933db1576874d412cd2c24a54d5001c055`。根项目镜像逐字节相同，固定正文UTF-8/LF核实；`diagnostics/prompts-final-04/summary.json`记录全部正文测试通过。真实G/V结果与独立审核分开，不借审核冒充运行成功。

## 8. 真实生成与验收详细结果

本节按实际范围区分格式生成、脚本运行及外部PR核验，保留每次原报告，补测不覆盖原失败。用户后续明确回复“授权”，覆盖向GitHub erranliu/SteerlineTest上传生成材料、推送本轮专属refs/验证head、创建验证PR、仅向验证base精确合并及清理；不授权业务main写入或安装部署。

| 生成记录（L4下目录） | 原结果及处理 |
| --- | --- |
| generation-01 | 正式生成入口两目标通过；原输入不完整，gaps保留，未当作脚本已可验收 |
| generation-02 | 两目标通过，留存提交bf2fb9f7f818236917c815a1ecd93bba644353a7；后审发现root多算一层和base写死，修上游规则并独立审核 |
| generation-03 | 原活动failed、停止核实，但早期测试未保存具体失败原因；不宣称后续通过已解释该未知原因 |
| generation-04 | 两目标通过，留存提交e7bfec869bed3a3c2909379aee4ef1d842bc10d6；后审发现依赖初始空pr.base，修规则为读取本环境manifest.validation_base并填充 |
| generation-05 | 证据reader早于服务建库打开失败，未调用模型；移至StartService之后 |
| generation-06 | 9809字节规则两目标通过；完整原请求/来源/停止保留，正常Git留存提交75832394f3a6ee06ffd1d4006ffbe54870080b69，生成源093a10e740501dd1d468d1fc6dc4a51500976d06；原本机脚本15738字节、SHA e02517f7a1265c1c972e6fefc6dd1dace045494804ca89e13abd2cee2a4c7658。V3发现单项L2被序列化为对象，修上游规则至10055字节 |
| generation-07 | 最终规则下，本机材料生成；远端原final在单个JSON对象后多出两个字符，原生结构化结果failed。系统拒绝，原文/诊断及清理confirmed=true保留，未裁掉尾部冒充成功 |
| generation-08 | 原生结构化结果成功，但本机manifest把本环境environment-context.json列入验收依赖；严格业务校验拒绝，未采用输出，停止与原文保留 |
| generation-09 | 根据只读模型目录改用默认gpt-6.1-sol/high生成，目标键、Runner、最终10055字节规则和项目标准不变；本机及远端两目标生成均通过，原请求/固定来源/完整输出/stopped核实及Runner清理confirmed=true；留存提交ea92a2457c0f0e93d8886cda0f4dfecea65dcba6，本机脚本27739字节、SHA-256为1de6572ddbba3270842103926b3cc9586fa3a9374c7674af8d59e905bf5ff96b，PS5.1语法通过。未授予ready，未运行远端验收 |

G的原输出均未手改。最终规则补充要求PowerShell以@(...)接收集合，即使零项/单项checks/l2仍为JSON数组；原严格解析器保持。该修订已独立批准，实际diff/字节账本和全部正文测试为diagnostics/prompts-final-04/summary.json。只读模型目录来源保存在diagnostics/generator-model-catalog.json，生成方案为inputs/generator-local-sol.json；原inputs/local-profile.json的Luna/high仍用于正式验收，完整环境键均为7cb8d70015db8ef641ee9ce081f9571ea45e0f33275599d24d9d922a342d7ad7。模型变化不改变目标对象。

| 验收记录 | 原结果及处理 |
| --- | --- |
| verification-01 | 命令在创建进程前被自动审批拒绝，理由是原授权未明确覆盖GitHub外部发布；没有报告或本轮refs/PR。随后用户明确授权，未绕过拒绝 |
| verification-02 | 多余material-generation方案缺access_mode，项目启动校验失败，尚未产生环境活动/PR。移除V用不到的方案及Codex发现前置，只加载原目标profile；未改原目标或生成脚本。原Runner停止核实、零PR/零活动及两条refs原提交匹配后精确lease删除，补充remote-ref-cleanup-followup.json，原failed报告保持 |
| verification-03 | G6原脚本真实TTY和8项环境要求passed，完整L2 demo.l2.smoke实际passed；l2为对象而非数组，Agent正确blocked且严格ParseReport拒绝，原Run stopped、result closed，资格unverified，未创建PR。不是完整正向通过 |
| verification-04 | G09原脚本首轮产品ready，15项要求/完整L2及原PR #725身份核实；外部取证因Get-FileHash不可用而失败，未合并，故完整L4仍failed。Runner停止核实，精确确认PR/3条refs归属及SHA后关闭725并删除本轮refs，remote-ref-cleanup-followup.json记录补充清理，原失败保留 |
| verification-05 | 使用同一G09原脚本、原Luna/high及修正后的外部取证，完整正向passed。首轮15项环境要求与完整L2 passed，原Run stopped/result closed，外部独立L2/分支规则/PR核验通过，PR #726仅向本轮专属验证base精确合并；准备前拒绝、ready后实际Run准入、停止及全部执行/远端资源清理均核实 |

V3的原caller记录和封存报告为environment-unverified.json、environment-sealed-result.json；provider-no-pr-followup.json核实零PR，application-stop-followup.json为正常停服，runner-verify-stopped-followup.json核实Runner已停止。旧测试只等Settled会对永久unverified一直等待；保存证据并正常停服后，只终止已核对路径/PID的测试进程，父报告仍failed。remote-ref-cleanup-followup.json确认两条refs精确删除、Runner head从未推送，原scratch留作失败证据。新测试遇到Run stopped、result closed且永久unverified会保存原记录并明确失败，不改变产品资格、报告或停止事实。

后续V使用G留存clone、原真实项目的main作为只读分支规则来源，避免将G本地fixture的pew-generation-source错作远端保护策略分支。实际验证仍由原驱动建立独占验证base，业务main不写入；最多三轮各一个验证PR，按原规则核验后仅向专属验证base精确合并，最后清理本轮拥有的PR和refs。M01仍独立待人工体验。

最终代码与界面补齐：验收方案显式展示模型/思考强度，不依赖自定义方案名称；环境列表继续按非模型上下文分组。diagnostics/readiness-models-01通过；web-05全部228项、HTTP/构建通过；browser-03两场景通过，375/1440截图含模型显示，已视觉核对。最终固定回归L1/10、L2/06均通过。文档静态复核631份Markdown、3986个本地导航链接，0错误；9个新增文本文件UTF-8/末尾换行/尾随空白及git diff --check通过。

外部取证修正只将摘要计算由Get-FileHash替换为.NET SHA-256读取原始文件字节，保持原哈希语义、完整L2验证及原工作区边界；没有改项目脚本、原报告或PR标准。diagnostics/local-evidence-01（e2e构建）及local-evidence-02（正常构建）均通过，覆盖模块自动加载关闭、UTF-8/CRLF原始字节、原工作区只读及Orca不可借本机读取。失败外部取证时，测试现在保留该home已停止的Runner原工作区以便继续取证；成功现场正常清理，未将停止事实或失败结果改写为通过。

V05证据根为L4/verification-05：summary.json与scenario.json均passed，cleanup_complete=true；environment-proof.json记录首轮15项checks全passed、固定script_digest=1de6572ddbba3270842103926b3cc9586fa3a9374c7674af8d59e905bf5ff96b，与G09原始Git blob一致。source.json的G留存提交为ea92a2457c0f0e93d8886cda0f4dfecea65dcba6，V正常Git冻结提交为afc022404e0115a9ca8521e27659059a9172d09d。没有手改G原脚本。

provider-pr-proof.json核实PR https://github.com/erranliu/SteerlineTest/pull/726，head=e88a21e2f0d16216c0aefdcc5506a6db0c27f1f5，merge=ee82c6a3dcbea974851b09b7ad913bd768d6138f；base仅为codex/e2e/c82965f4cd744c0196eae205bdf78a82/testing.project-environment-local/validation/testing.project-environment-local/base。original-l2-summary.json保留同一原Run工作区的完整原L2及摘要校验，完整选中demo.l2.smoke，未运行替代测试。

admission-proof.json记录before_rejected=true、after_run_id=39a46de4fa7cc5dd429d93663ef22e49、stopped=true；另有实际候选和原Run的启动/停止快照。remote-ref-cleanup.json对本轮source、validation、环境Run head、准入Run head四个拥有的ref核实清理；308-runner-cleanup.json confirmed=true、retained_for_remote_evidence=false，场景清理无错误。较早失败轮次的本地现场/证据按原记录保留，不混同活动或远端资源残留。

因此A13-G与A13-V均通过，自动工程验收完成；真实结论仅覆盖本轮Sol本机生成和Luna本机验收，不宣称Orca本机/远端原生正向验收通过。M01保持后置人工体验。功能代码随后按2026-11-01用户要求提交PR；仍未部署安装现场，业务main未写入。

## 9. PR提交前整合验证（2026-11-01）

用户要求“提pr”后，创建codex/project-environment-workflow分支并整合最新main 967af588（包含生成提交恢复修复#415）。唯一冲突位于验收README的顶部导航，保留两项记录；没有业务代码冲突。整合后的实现提交为7481b3cc4ecc29e1a436df5694089ee4fddaa5a2。

该干净代码提交重新执行固定L1和L2，均passed且清理通过，证据分别为原scope下pr-submit-20261101/L1/01/summary.json、pr-submit-20261101/L2/01/summary.json。文档检查632份Markdown、3987个本地链接，0错误；git diff --check通过。前节Web、浏览器、owner、受控L3和真实G09/V05保留各自冻结源码及原报告，不改写为本次整合版本的重新运行结果。固定提示词最终正文、字节数与独立批准内容未改变。

随后仅补充本节整合记录提交；本次发布范围为功能分支和PR，不部署安装环境、不自动合并main，M01仍待人工体验。


## 10. 环境要求与调用协议解耦实施（2026-11-01）

用户授权按修订 Spec/Plan 实施，本轮不实现整个项目初始化适配。基线 752b1ed1df4e745c663fb2453ec029798d435d62；新证据范围为 `C:/SteerlineDebug/wt-68d4-project-environment-workflow/requirements-decoupling/`。前节 G09/V05 仍是旧格式历史，本轮不能沿用。

项目入口 version1 仅定位公共材料及唯一正式测试清单；环境文档 manifest3 唯一声明 required_checks/gaps，生成结果 gaps 严格投影同一文档。材料查询和验收直接读取固定 Git 文档，空生成历史不影响缺口阻断；目标事实只在原活动/请求冻结。新环境或旧失效产物按固定源码调查，有效材料按有限引用更新；调查实际整树普通 blob 总字节保持原1 MiB限制。项目初始化入口仍不提供，未部署安装现场。测试安排、候选、记录及 roadmap 规范消费者共用入口，不保留固定项目政策/清单回退。

### 实际正文与批准

业务原因是去掉 SteerLine 内部项目工具/版本/材料目录政策及 Git context 提交前置，保留通用生成和执行/报告协议。TTY 失败入口的严格原报告守卫从生成规则移至执行协议，仍完整保留；语言实现方式由项目负责。完整差异、每份 UTF-8 字节及 SHA256 见本范围 `prompt-review/final-01/actual-text.diff`、`bytes.json`，新指导/协议全文也保存在该目录，不用摘要或黄金样本替代正文审核。

| 正文 | 原字节 → 本版字节 |
| --- | --- |
| 环境生成规则及同源镜像 | 10055 → 3862 |
| 环境执行规则及同源镜像 | 5501 → 6683 |
| 测试执行规则及同源镜像 | 3229 → 3256（本仓库正式镜像新增 3256） |
| 项目适配指导 | 0 → 7983（设计上一稿 7892） |
| 环境生成短模板 / 实际代表请求 | 440 → 675 / 812 → 1018 |
| 测试生成短模板 / 代表请求 | 262 → 266 / 258 → 328 |
| 测试执行短模板 / reference / step 样本 | 423 → 377 / 464 → 484 / 492 → 512 |
| roadmap 文档短模板 / 代表请求 | 351 → 398 / 343 → 390 |

生成两份完整协议总计 15556 → 10545 字节；调查另读完整7983字节指导，合计18528字节，项目实际源码/政策另按固定快照记录；正常更新不加载指导。验收仅加载6683字节执行协议和正式运行依据，不重复生成规则/指导。新增指导承担实际调查到项目接口的二级职责，不成为另一份项目要求。

`/root/prompt_review` 独立只读核验实际正文、接线、资产、样本和守卫后批准当前定版。审核发现并修复：测试安排新生成缺同源校验、roadmap 短模板仍指旧政策、调查源码未计整树限额。新请求严格阻断不同源协议，原冻结请求重放不重构；调查按同提交普通 blob 实际大小计限额。固定模板 LF、含用户 CRLF 样本保留；三套镜像逐字节相同。批准覆盖当前正文/所审接线，不代替工程或真实验收。

### 当前工程证据

本范围 diagnostics/core-02、core-03、protocol-04、storage-02、app-02、app-03、source-bounds-01、contracts-final-01、catalogue-01 均通过，分别覆盖领域生命周期、协议同源、持久化不可变、自定义来源、冷启动 gaps、旧失效材料调查、真实源码限额及路由。默认固定 L1/01、L2/01 通过，L2 清理完成；web-02 完整构建/客户端/HTTP及 browser-01 两场景通过，补充环境界面16项通过。最终新增自定义目录受控场景及真实 G/V 尚在推进，完成后另记。

原失败完整保留：core-01/app-01 为未同步入口/检查集合的旧夹具，prompts-01 为本地写入 CRLF 触发正文摘要，e2e-owners-01 为 driver DTO 与内部快照字段错配，storage-01 为旧报告/验证器夹具未声明入口；均修对应来源/夹具后在新目录验证。web-01 发现 unknown 条件渲染不能作为 ReactNode，显式判断非空后完整 web-02 通过。e2e-owners-02 的两项目维护查询超时，其他 owner 通过；最终协议守卫版本 isolation-03 定向复验146.237秒通过，未延长期限或改写原失败，原超时的唯一根因未确认。


补充工程收口：实现固定为 e6a5bb20751915e94995942396fc3b7d57c81ab6；最终两个测试夹具修复为 ca944cd868ca195cbd42c46144e8533ca1fd5999、0087b2cf（完整提交可由 Git 解析）。L1/02、L2/02 在干净实现提交运行且通过，L2 清理完整。Web 最终 web-04 的31文件/229用例、构建和HTTP通过；web-03 曾因已有用例在异步历史返回前立即查询停止按钮而失败，改 await findByRole 等待原内容，原停止命令/禁用和副作用断言不变；补充独立审阅通过。

L3/readiness-01 默认目录闭环通过；readiness-02 因夹具未使用导入编译失败；readiness-03 在自定义目录下揭出 scriptedEnvironment 仍读旧固定清单，原启动结果未知并最终超时，原失败和清理保持。ca944cd8 将该模拟端改为同一 BaseRevision 的入口/清单读取，不提供回退，独立批准后 L3/readiness-04 两项目/自定义目录生成、采用、完整受控验收、候选准入、失效隔离及全部清理通过，未延长预算。模拟证据不声明原生环境或真实 PR。

另补充 diagnostics/e2e-final-01 共25项原始证据/真实夹具 owner 通过。移除 context 步骤后，真实 G 的采用夹具仍假定输出父目录已存在；0087b2cf 仅在校验目标及 manifest 后为原字节落盘创建独立 clone 内本键目录，已独立批准，不修改模型输出/标准。已启动的旧编译 G 轮次保持原结果，修复版本另目录复验。

本范围 L4/generation-01：首个本机原生内容结果通过格式/来源/目标/投影校验，之后旧编译夹具在新目录落盘失败；停止及 Runner 清理 confirmed=true，原生成/请求/输出完整保留。未执行远端生成或本机验收，不记 G 完整通过。采用0087b2cf修复后新目录重新测试，不手改原生成字节。


### 最终真实 G/V 与后续修复

本范围 G02 两目标实际生成通过（L4/generation-02，1358.967秒，cleanup_complete=true）。两个目标均为 manifest3、gaps=[]；原请求/来源/内容结果、停止和清理证据齐全，未授予 ready，未执行远端验收。原生成 Git 留存提交 `0ec457fe91366d1cb11c28e3f72f711ad74669eb`；本机脚本27758字节，SHA256 `482420f4aa328f879cd8e58482ca1bca6eb83c5a0d95391149fcfeca602a25e6`，PowerShell5.1静态语法通过；远端脚本30717字节。两目标各自检查来自项目政策，未恢复内部固定项目工具清单。

G02 原留存不动。为准入探针，在独立 verification-source-02 clone 正常 Git 采用同版本固定测试协议，提交 `44236738f72b82b0aa14b067b3679b5e86a1de06` 仅改变 `SteerlineFlow/testing/runtime-progress.md`，原生成文档、脚本 Git blob 及 writer 均不变，见 inputs/verification-source-protocol-adoption.json。正常 Windows CRLF 检出与原 blob 已分别核验，不把检出转换当作生成修改。V 的既有夹具另将 validation_base 绑定本轮专属分支并准备禁止实现的 Task 探针，经正常 Git 固定；脚本始终为 G02 原字节。

| 本范围 V 轮次 | 原结果、修复及清理 |
| --- | --- |
| verification-01 | 原测试源码的测试协议副本未同源，Task材料准备正确拒绝，未启动环境Run/候选/测试生成。补测试侧 preflight，在创建现场/refs前检查准入探针协议和writer；生成夹具同步准备当前测试协议，不新增生产环境要求。原失败保留，零活动/无PR/Runner停止及两个refs原SHA lease清理见 remote-ref-cleanup-followup.json。 |
| verification-02 | 23个阅读引用中的重复运行辅助路径使固定Runner正文达到1574/1000B，在发送Agent前阻断；调用方447B无额度，未执行环境脚本。通过原公开environment_stop受理停止，原Run stopped/result closed、零候选/无PR。保留全部manifest依赖的读取/哈希/缺失守卫和完整Git工作区，仅不把每个helper再列为阅读引用，引用降为5；标准与预算均未放宽。两个自有refs精确lease删除，Runner head从未推送，跟进证据保留。 |
| verification-03 | 测试生成在CLI启动前被明确拒绝，未产生环境Run/候选，未分配原RunID；原根因未完全确认。固定CLI后续只读启动正常，两个自有refs精确lease清理，原失败不改写。 |
| verification-04 | 原测试生成submit结果未知且request_not_found，不重发。通过原正式取消请求和正常停服收尾；公开离线verify-takeover在原namespace/同固定构建上compatible=true、instance_id空、items=[]，证明原请求未被接受，零环境Run/候选；原取消、失败、离线报告及两个refs精确清理证据保留。 |
| verification-05 | 在最终缓存/引用接线上完整正向通过（413.709秒，cleanup_complete=true），首轮六项检查及完整L2 passed，原Run stopped/result closed、原PR身份和独立L2/规则核验通过，实际准入Run启动/停止与全部资源清理完成。 |

重复读取接线也已修复：材料评估与测试生成现在复用原同请求 `commit:path` immutable blob 缓存；不增加缓存层、不跨请求或提交复用、不扩大共享5秒tick/Runner预算。新反例及 diagnostics/shared-read-cache-01 通过；diagnostics/runtime-dependency-refs-01 验证运行helper不重复阅读引用，但改变仍使依据失效、缺失仍阻断，自定义政策/清单引用保持。V03/V04 原CLI错误不能据此推断唯一根因已全部确认；新的实际运行结果独立记录。

V05 使用相同本机脚本摘要 `482420f4…a25e6`，检查 powershell、git、python、github.auth、source.clean、terminal.tty 全passed；全部正式L2 `demo.l2.smoke` 为真实非零通过，原始摘要与明细在 original-l2-summary.json。原环境Run为 `087e598cd9d05b36554c8c9e72be64e5`；验证PR [SteerlineTest#740](https://github.com/erranliu/SteerlineTest/pull/740)，head=`b3bd09d3b524655e5397168a2feefbb3c7686939`、merge=`60c6992a40fc177b8657be364eb6c165a81cc1bd`，base仅为 `codex/e2e/904ee323a47d4d7f9d32f1d62a823f24/testing.project-environment-local/validation/testing.project-environment-local/base`。外部合并未回写原封存报告。

admission-proof.json 记录准备前拒绝、ready后实际Run `5b70fa2a771df024987859c61a1f639b` 启动且停止；remote-ref-cleanup.json 核实source/validation/环境Run/准入Run四个本轮refs均清理，308-runner-cleanup.json confirmed=true且未保留活动工作区。较早失败的本地现场作为证据保留，不当作活动资源残留或新版本通过。

所有新增正文/接线/测试资产和最后规范性文档均由 `/root/prompt_review` 独立只读审核；发现的协议前置、重复引用、缓存及默认目录/Spec状态残留已修复并复审批准。指南7983字节/hash不变。最终固定L1/L2收尾结果另在下段登记；M01人工体验仍后置。整个项目初始化适配未实现、未部署安装环境、未合入业务main或自动合并功能PR。本轮已转到独立分支 codex/environment-requirements-decoupling；此前PR416已合并，不修改其历史正文。


### 最终固定回归与交付

干净源码提交 `39e84418`（包含全部已审代码/规范，功能最终接线为 bbd69edd）运行固定 L1/05、L2/03，均通过，L2 TestDeliverySmoke 134.70秒且cleanup_complete=true。没有跳过、零命中或未知清理算作通过。最终完整正文/实际接线差异和21份字节账本位于本范围 prompt-review/final-02/actual-text-and-wiring.diff、bytes.json；actual-reading-references.json 直接从 V02/V05 原请求提取23→5引用，对应相同19项运行依赖，实际调用方 prepare 正文均447字节，未增加或藏入规则。

当前全部受影响自动验证完成：最终固定L1/L2、owner、Web229项及构建/HTTP、浏览器两场景、两项目/自定义目录受控L3、真实G02两目标生成及V05同字节本机闭环均通过。各报告保持各自冻结源码/原始选择，不改写前述失败或将旧版本结果当作新提交重跑。最后仅补本段记录及状态导航，按纯文档检查处理。M01人工体验继续后置；项目初始化适配功能未实现，未部署安装现场。
