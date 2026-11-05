# SteerLine 与 SteerRunner 职责边界收口 Spec

S1–S4 已实施，审计日期为 2026-11-05，SteerLine 基线为 `57e3c52e`，SteerRunner 基线为 `e7876c0d`。步骤见 [Plan](../plans/runner-boundary-consolidation.plan.md)，实际验证范围及结果见[专属验收](../testing/acceptance/runner-boundary-consolidation.md)。Runner 配套文档为其仓库 `docs/spec/native-catalog-boundary.spec.md` 和同名 Plan，公开新增接口的生产定义归该 Spec。

## 1. 审计结论与修订范围

执行已通过固定 EXE 与公开 CLI 迁出，Task/DAG、候选与唯一 TaskDelivery 已有明确所有者。本轮进一步集中原生协议知识、消除策略槽位双份定义，并收口两类内容生成请求的共同绑定核验。

| 当前交叉 | 审计结论与本期修正 |
| --- | --- |
| Web 自行实现 Codex model/list、Orca 本机与远端 RPC/配对 | 原生查询归 Runner，Web 使用固定 EXE 的无服务目录入口；按 Orca、Codex 两阶段迁移 |
| app.projectPolicySlots 硬编码三项名称和路径 | Runner 已经通过 model_input_contract 发布同源 slots；直接消费已有字段，预检继续归 SteerLine |
| Roadmap 记录转换为 testmanagement.Generation 参与停服核验 | 把共同 Run/request/停止证明的纯值和比较规则归 runnerbinding，各业务服务继续持有自己的记录和更新事务 |
| Web 缓存键读取私有配对/runtime 信息 | 迁移对应路径后每次查询重新获取，不以旧五分钟缓存报告新鲜；保留在途合并与过期结果展示 |
| 环境检测 useNativeOrcaExecutor 修改执行参数 | 属于另一个执行请求构造问题，本轮记录为后续审计线索，按当前原生执行 Spec 处理 |

本 Spec 按 S1–S4 四阶段验收。S1 先迁 Orca，S2 迁 Codex；S3 策略目录消费和 S4 内容绑定收口均可独立推进。新接口能力按实际固定 Runner 构建判断，旧 Run 继续使用原冻结事实和现行停止通道。

## 2. 职责与一致性边界

| Owner | 权威事实和行为 |
| --- | --- |
| SteerLine Task / taskrunner | 固定任务范围、DAG、候选参与关系、候选创建与原请求、用户授权、交付事务和后继资格 |
| SteerLine 项目/配置功能 | 产品支持组合、方案与默认选择、项目材料预检、项目×环境资格、配置保存与页面查询 |
| Runner | 全部 LLM 执行、原生适配、工作区创建、Workflow、会话及资源管理、原生目录协议和 PR 执行事实 |
| SteerLine runnerbinding | 原请求/Run/namespace/关联/版本及停止证明的纯校验 |
| Runner CLI / runnercontract | 公开输入/响应/schema、固定构建身份，协议值与调用方事实之间的边界 |

Runner 提供属于原 Run 的 PR 事实，SteerLine 在候选当前身份及任务版本下消费这些事实。唯一 TaskDelivery、候选收尾及后继释放继续在同一业务事务中确认。Roadmap 文档发布的人工确认与业务 Git/provider 编排按其原用例执行，其存在不能成为把 Task 交付判定迁给 Runner 的依据。

当前未发布版本按 [AGENTS.md](../../AGENTS.md)执行，新增格式使用单一当前契约。现有状态的重启、原请求对账、当前构建停止义务与结算继续按 [Worker 生命周期](../../docs/product-design/worker-lifecycle.md)及[现行 Runner v8 接入](../testing/acceptance/runner-v8-install-reset.md)核对。各阶段改变的是职责归属及公开查询入口。

## 3. S1 · Orca 原生目录归 Runner

### 3.1 调用链与接入资格

浏览器沿现有配置发现 HTTP API 提交 profile、host、adapter、environment、agent 和刷新意图。Web 从自己的配置快照解析固定 Runner pin 及允许的查询目标，调用 `native-catalog --input - --json`。Web 的 metadata 子进程 owner 继续管理固定 Runner CLI 的寿命与管道，Runner 内部负责原生连接及元数据资源。

Web 不引入常驻 runnerclient.Client 或后端 app 依赖。复用当前 `web/server/nativequery` 与 `web/server/executionconfig` 的离线 pin/公开元数据校验方式，增加有限 stdin JSON 查询能力。公开数据值放在 runnercontract；守卫继续限制 Web 只消费纯配置/协议及元数据传输。

调用前核对配置 pin 的路径、版本、SHA256 及离线 capability/resource_digest，调用结束再次核对 pin 与响应 source。native_catalog 仅按当前固定 EXE 已实现的组合启用。缺能力、错误 schema、构建身份变化或响应不匹配时返回具体诊断，该目录功能不可用。native_catalog 的可用性与 run_configuration 的原有解码/验证分别记录，不能因为可选目录字段错误而让整个配置快照加载失败。目录错误不阻断其它配置页面读取、已有候选观察和停止。

### 3.2 跨仓查询约定

Runner 接口为 `steerrunner-native-catalog/v1`，使用原 `steerrunner-cli/v1` Response，request_id=null，无 state-dir/实例绑定/Run 选择器。请求对象最多 64 KiB，kind 为 environments/agents/models。environments 仅含 kind，其余携带 adapter、公开 ExecutionTarget 形状的 target，models 还含 agent。本机 Codex models 可带由服务器配置解析的 executable。原生凭据、目录、pipe、endpoint、RPC 方法没有浏览器或 CLI 请求字段。

data 包含原选择器、contract、source、state、complete、observed_at、context_unverified 和对应目录数组。source 包含实际 Runner 构建身份及可证明的原生版本/runtime。state=ready 仅在 complete=true 时作为完整结果，loading/complete=false 只用于展示原生进行中的目录。environments 返回稳定 ID/名称，agents 返回原始 ID/来源，models 返回原始 ID、标签、原生 provider/default/effort 及来源。保留原生 fetched_at 和 listing_in_progress，接收时间单独记录。

输入错误 exit 2，未实现组合 exit 3，native_catalog_unavailable/native_catalog_timeout/native_catalog_cleanup_unverified 为 exit 4，内部故障 exit 1。失败 data=null，由 Web 映射到原配置查询的 unsupported/unavailable 与安全诊断。完整空数组和查询失败分开处理，畸形响应、重复身份、分页/限额/清理失败不能映射为成功空目录。

版本/离线声明核对及目录取数共享 Web 的 30 秒查询预算，原生响应累计及最终响应各自不超过 1 MiB，清理额外上限五秒。Web 的外层 I/O/等待需给 Runner 清理留出该五秒，超过上限后只清理本次所属 CLI 进程树并报告清理未核实。客户端取消、页面卸载、最后一个订阅退出及 Web 关闭均按所有权取消和排空。

### 3.3 目标与支持范围

Orca 本机与远端共用 Runner 现有协议能力层。environments 可返回已登记目标，即使目标离线。agents/models 使用明确目标，不能回退到本机或另一 Agent。原生目录不以 Web 的当前执行支持矩阵预过滤，已注册但未映射的 Agent 在页面展示发现事实与不可执行原因。Runner 原生协议不足时如实提示。

保存及执行配置仍取 Web 产品支持范围、固定 Runner 静态支持组合、输入 schema 和现行纯校验的交集。发现成功不自动选择方案、改写模型/effort、不新增项目资格。现有显式手动填写规则继续适用，不用目录成功推导认证、额度或测试通过。

### 3.4 缓存与删除要求

删除迁移路径对 orca-runtime.json/orca-environments.json 的读取、配对解密、WebSocket/IPC/RPC 实现及仅供这些代码使用的测试夹具。S1 后仍被本机 Codex 使用的元数据进程代码保留实际消费者。

Web 保留总并发最多两个、同一配置快照及目标的在途请求合并、最后订阅退出取消和晚到结果围栏。迁移的 Orca 路径每次新查询调用 Runner，取消跨请求五分钟新鲜缓存。页面允许展示原保留结果，但失败或上下文变化时明确过期，不将其赋给新选择。刷新表示绕过 Web 复用并重新请求，原生内部缓存时间仍按返回事实展示。原生凭据身份不再成为 Web 缓存键。

## 4. S2 · Codex 模型目录归 Runner

待固定 EXE 发布本机 Codex models 后，沿 S1 同一入口接入。Web 从匹配 profile 读取既有 executable，默认入口沿现行平台规则，浏览器不能提供新路径、args 或命令。Runner 以实际查询宿主的 CODEX_HOME/已有登录执行短命 model/list，响应始终声明未验证具体业务会话/账号上下文。

删除 Web 的 Codex initialize/model/list、分页及原生模型解码实现。固定 Runner EXE 的调用、配置保存和进程 owner 仍由 Web 的 metadata 通道承担。若 nativequery 仍被离线 capabilities/version 及新查询使用，按真实用途保留。S2 迁移后对应 Codex 目录采用与 S1 相同的在途合并、刷新和过期展示规则。

S1/S2 分阶段交付期间，尚未迁移的原实现仅服务其既有查询组合。已迁移组合固定走新入口，Runner 能力缺失或故障直接报告，该组合的旧原生路径在阶段完成时删除。

## 5. S3 · 消费现有项目策略目录

Runner 已公开 `capabilities.data.model_input_contract.workflow.project_policies.slots`，每项为 `{slot,path}`。沿现有固定构建 Catalog 解码该部分，由 runnercontract 承载纯值。对 slots 数组类型、非空项、重复 slot/path、相对路径及越界路径严格校验，目录顺序决定预检和所选 slot 的确定顺序。Runner 的其它 model_input_contract 字段作为公开说明消费，本期不把它们转换成新的行为规则。

app.projectPolicySlots 从冻结 runtimeCatalog 读取这些条目，在固定候选 commit 上复用现有批量 Git 文件读取。缺失的可选文件不选中，存在但不可读/非常规文件/错误 Git 响应阻断。确认无适用文件时显式 `[]`，非法或缺失目录不能当作空目录。正常受支持目录应包含可用条目，空 slots 定义按不完整契约阻断新开发请求。slots 解码错误以该功能的不可用原因保存，不能在 Catalog 通用加载时阻断原 Run 观察/停止或无关内容功能。

「已就绪」材料核验和实际准备请求消费同一固定 Runner 目录及提交。对账原请求时继续用已持久原字节，配置或目录刷新不重写既有 outbox。策略路径和阶段仍由 Runner 单一定义维护，SteerLine 只按目录读取；阶段路由继续由 Runner Workflow 执行。已有项目策略字节、固定短指令及文件通道按[提示词维护](../../docs/prompt-governance.md)核对。

删除 SteerLine 的三项槽位/路径常量和路径拼装。保留现有基线样本证明选项顺序、显式空数组和实际提交正文一致。若接线或样本确有变化，提供完整正文差异、UTF-8 字节数前后值、业务原因、验证并取得独立审核，不能以更新样本替代批准。

## 6. S4 · 内容请求共同绑定规则归位

本阶段只覆盖测试安排生成与 Roadmap generate/1 记录。原有生成记录、结果解析、确认、材料/manifest 验证、额度、排队和 Store/CAS 继续由各业务 owner 更新。候选 runnerbridge 和环境检测保持各自现有生命周期。

在 runnerbinding 中用窄的调用方事实表达 state-dir/build 来源、原 submit request、已知 RunID/input_digest、已观察 revision、预期 workflow 和关联键值。纯函数核对公开 Inspection/Run 与这些事实，返回明确比较结果。request-id 观察要求原 submit Command 与 Run 的关联成立；RunID、input_digest、项目/对象关联、workflow 和 revision 的既有比较均须保留。构建来源继续按当前 namespace/重启/升级规则解释，不因抽取公共值而新增逐 Run 的构建恢复门槛。不得因为共用函数而接受原本会被某个 owner 拒绝的观察。

将 `roadmapBinding` 的跨领域记录转换改为中性事实投影。原启动前 inspectOriginalBeforeStart 继续拥有传输及冷读证据含义，其结果交给相同纯校验。在线 request_not_found 继续未知，只有现有冷读租约证明下的缺失才可沿原规则结束取消记录。

共同停止规则只抽纯决策/构造所需部分。业务 owner 读取新 revision、决定是否需要 stop、分配原 ID，并在自己的 CAS 事务保存完整信封后才能发送。unknown 或 accepted 的原 stop 信封沿原身份对账；明确 stale_revision 拒绝且重新观察后才可构造新信封。保存失败零发送，停止未结算继续持有义务及额度。原本没有的自动 submit/reply/resume 或跨构建恢复不由本次提取引入。

在线观察、启动前停止准备及 application_verify_stop 的两个内容消费方均接入中性事实。结果成功与资源结算分别由原公开事实核对，纯绑定成功不能替代生成内容、Roadmap 分析或停止证明。

## 7. 通过条件与依赖

| 阶段 | 必要前置 | 完成判断 |
| --- | --- | --- |
| S1 | Runner R1 固定真实 EXE 的公开无服务契约验收 | Web 正式配置页走新接口，原 Orca 协议/凭据代码删除；本机与远端各有目录证据 |
| S2 | S1 和 Runner R2 | Web 原 model/list 删除；取消/清理及真实无模型查询通过 |
| S3 | 当前已公开策略 slots，无需等待 R1/R2 | 硬编码删除，固定目录/commit 预检与冻结字节有回归 |
| S4 | 当前 generate/1 公开事实 | Roadmap 不再转换为测试生成记录，共同核验规则单处维护，两个正常入口、重启与停止异常有回归 |

验收行为见[专属矩阵](../testing/acceptance/runner-boundary-consolidation.md)。四阶段均使用原业务/公开接口验证。项目拆库、通用工作流引擎、app 整体拆包和安装切换另按实际需要规划。
