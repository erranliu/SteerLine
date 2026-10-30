# Spec：应用级共享 SteerRunner 服务

2026-10-29：已同步[旧 Check 退役](legacy-environment-check-retirement.spec.md)后的当前资源模型；项目环境准备使用普通 Run。原共享服务验收记录保持其版本范围。


日期：2026-10-19。状态：已按用户授权实施，自动验证完成，人工体验待验。配套 [Plan](../plans/shared-runner-service.plan.md) 与[专属验收](../testing/acceptance/shared-runner-service.md)。普通开发，不占 Task ID、不修改 roadmap。本次实现范围在 SteerLine，不要求修改 Runner 引擎或公开协议。

## 1. 目标与范围

将默认的“每项目一个 Runner 常驻服务”改为“每个 SteerLine home 一个 Runner 常驻服务”。同 home 的多个项目通过原公开 CLI 连接同一实例，各 Run 继续拥有自己的仓库、工作区、材料、执行参数、原生资源和发布目标。

正常加载项目时，产品常驻拓扑为一个 SteerLine 后端、一个 Web、一个 Runner；CLI 短命令、浏览器与执行工作进程另计。不同 home 各有独立 Runner，不做整机全局服务。

包含应用/项目资源所有权、请求身份、全部 Runner 消费者、停服/换代/离线取证、调试生成与重置、测试夹具和文档。保留 Task/DAG、项目 SQLite/actor/锁、候选参与与 TaskDelivery 的原职责。

不增加 Runner 项目管理 API、多租户权限系统、服务池、跨 home 共享、并发调度器、项目热重启或旧状态迁移。闪窗治理后置，不将服务数量减少表述为闪窗已解决；不扩大原生恢复、沙箱或 Orca 专项。

## 2. 已有能力与需要替换的行为

调查基线：SteerLine `abd001f1fc40fb57abec49f4173874c0c942f2f1`；本地 Runner 源码 `C:/Project/SteerRunner` 的 `59c7597ac8056fc5c29ad622034e10e74e4395de`。源码能力不代替交付 EXE 的身份或实跑证据。

| 位置 | 调查基线行为与本次处理 |
| --- | --- |
| Runner `contracts/run.go`、`application/engine.go`、`freeze.go` | 请求按 Run 接收仓库/材料/workflow/execution/publication；服务维护多个 Run，缓存键含仓库身份，私有仓库键含 Run ID。直接复用，不引入项目服务能力 |
| `app/project_runner.go`、`project_runtime.go` | 项目创建、绑定并最终停止 Runner。服务生命周期上移到应用宿主；项目只借用固定客户端 |
| `runnerstatepath`、`runner_build_binding.go`、`state_preflight.go` | 按项目 run-dir 派生短目录、固定构建/候选绑定。改为按 home 派生服务目录，保留项目数据库和原事实校验 |
| `taskrunner`、`runnerbridge`、`environmentadaptation` | 项目局部请求 ID 与 Runner 原请求需分别保存。外部操作改用已持久化的独立随机 ID，产品请求仍按原项目边界去重 |
| `application_verify_stop.go`、debug reset/cleanup、controlled fixture | 逐项目停服/取证/删除目录，部分夹具按 state-dir 选择单一 provider。统一服务所有者与包级清理，夹具补齐共享服务的多仓库路由 |

Runner 的 `commands.request_id` 是服务内全局主键。共享路径本身不能解决请求冲突，也不能作为项目访问授权。

## 3. 服务与目录所有权

应用宿主持有一个最小 Runner owner，保存本次客户端、固定构建身份、服务实例、启动事实及原停止请求；复用现有 `projectRunner` 的身份和结算逻辑后替换项目级 owner，不并存两套服务管理器。

在现有应用启动顺序中完成项目身份 census/冲突检查后，若还有可启动项目，由应用直接初始化一次 Runner，再把同一固定绑定交给项目运行时。不由首个项目触发、不增加借用等待队列或引用计数。零项目、census 后没有可启动项目、Close-before-Start 和只读 catalog 均零启动；项目后续本地 prepare 全部失败时，允许已启动的空闲 Runner 保持到应用整体关闭。

共同初始化使用应用的有界启动 context。通过构建与服务身份核验后立即固定原停止义务，再查询目录/能力；后续查询失败也由应用结算该原实例。失败保留为本次启动结果，应用 API/Web 继续提供目录和诊断，不由各项目重试或创建替代服务。初始化期间收到整体停止，等待原尝试的有界结果并由同一 owner 收尾；已创建但结果未知时保留原事实，不假报未启动。

Runner 部署选择来自 home 的固定 `execution.json`。EXE/version/SHA/resource digest、state-dir 和本次 instance_id 对所有借用者一致；项目单独验证自己的 publication/配置，失败不停止已经由应用持有的 Runner，也不拖垮有效兄弟项目。目录只读和业务查询不得隐式启动服务。

构建核验必须先于 `service start`：将现有构建 pin 的服务准入职责上移，在 home 私有 `application-api/runner-build.json` 保存一份不可变 BuildPin（原 EXE/version/SHA/resource digest/state-dir）。应用租约下先离线验证所选 EXE 并比对该 pin，匹配后才能启动/接纳服务。首次只在 pin 和派生 Runner 目录均不存在的新环境写 pin，再准备 Runner 目录；已有目录但无 pin 时拒绝，不能推测为空或借项目库补认。失败/崩溃不删除 pin，reset 也保留它。项目候选/checkpoint 的 pin 继续作为原操作绑定快照，与该共同准入 pin 校验一致；不再等项目初始化之后才拦截换构建。这份文件只承载原有构建绑定，不记录 Run、服务阶段或重试意图。

短路径继续使用当前本地卷根的 `slr/<26位base32摘要>`，Windows 摘要输入为带用途标记的规范绝对 home，大小写规整；非 Windows 沿用系统临时目录根。保持 128-bit key 与现有 socket 预算、私有目录和重定向拒绝。用途标记区分新应用布局与旧项目布局，不增加用户可选路径模式。

所有启动、业务、环境活动、查询、原生查看、最近消息、恢复、停服、离线取证及调试清理共用同一派生函数。项目 Task 库仍留在各自 run-dir；候选保存精确 state-dir/build pin，环境活动也持久化自己的服务目录绑定。路径不等于执行 cwd，工作区由 Runner 按 Run 管理；不改变进程全局 cwd/环境来切换项目。

旧项目目录、旧候选/活动或旧 checkpoint 与新绑定不符时明确拒绝，不扫描、迁移、改绑或删除旧目录。实施和验收采用新环境；旧环境的原资源由原构建与原入口结算，不因本次文档自动操作。

## 4. 请求、回执和项目隔离

保留用户/产品请求 ID 与项目内原有幂等规则。Runner request_id 使用首次创建外部操作意图时生成的独立 128-bit 随机 ID，复用现有随机 ID 做法，不编码 home/project/动作、不添加 scope 哈希或代次。项目归属来自原项目库、目标绑定和 correlation，不能把 ID 格式当作授权。

submit 由候选原产品请求找到已存信封；环境活动由原操作的产品 RequestID 找到已存信封；reply/resume 在现有 Action 中补存产品请求 ID/原请求摘要，在原候选串行门槛内先按该 ID 核对不可变产品请求，再决定是否生成新 Runner 信封。找到旧动作时直接复用原信封，不能按最新 Run revision 重造正文后误报冲突。调查基线的 `QueueControl` 仅按 Runner ID 去重，直接随机替换会使“动作已保存、外层控制回执未保存”后的重试多建操作。现有 Run stop 已生成并保存独立随机 ID，保持其逻辑；服务停止由应用 owner 固定原实例与原请求。

完整 Runner 信封在调用前持久化，存储更新不能改写已存 Action 的产品 ID/摘要、命令和 Runner 信封，也不能改绑活动的 state-dir；复用 candidate/outbox/原活动，不增加映射表或恢复服务。原产品 ID 与保存的 Runner ID/正文/目标/command_id 关联；重复原请求读取原动作，换正文/动作仍拒绝。查询、重启和未知对账不重新生成 Runner ID，也不能用产品 ID 直接查 Runner。

同 home 不同项目使用相同 task/candidate/request ID 时得到独立 Runner 请求；同项目不同请求保持独立。已调用但回执未知只读取原 Runner ID，重启不补 submit/reply/resume；确认未派发的 pending 意图按原换代规则撤销。ID 调整不授予重发权限。

所有公开结果沿项目库内原绑定消费，核对原 Run ID、input_digest、command/请求身份及可用 correlation；提交增加明确的 SteerLine 项目标识。项目路由不得接受浏览器提供的任意 Runner Run ID 去代理查询或控制。两个项目共享 state-dir 后，不能再仅凭目录相同认定结果属于该项目。环境准备等非候选活动沿原普通 Run/request/input 绑定核对，已完成和未知活动同样受此规则保护。

## 5. 项目退出、整体停服与恢复

统一 worker 语义归 [worker-lifecycle](../../docs/product-design/worker-lifecycle.md)，关联 W01/W08–W14/W18、E04/E05/E07、V01/V03/V06–V08。本次只改变调用方服务所有权，不复制节点状态机。

### 5.1 单项目退出

当前产品无单项目 stop-service/热重启入口，本次也不新增。项目初始化失败或运行时退出时，关闭其本地新请求/派发门槛、排空借用调用并关闭自己的 handler/product/store/log/锁，按既有本地失败保锁规则处理；不得调用共享 service stop、杀共享 PID，或因该项目配置不合法替换全局绑定。

这里明确改变原“项目退出即所属 Runner 停服”的隐含关系：单项目退出只结束 caller，本身不撤销已受理 Run 的 Runner workflow 授权。原 Run 可能继续执行，结果不再由该项目实时消费。保留绑定与未知操作，目录诊断明确“项目不可用，已有 Runner 执行可能仍在继续”；本地关闭不证明资源已结算。未完成的 Task 终态停止义务仍持久保留，项目 actor 已关闭时不承诺继续派发/对账；由应用整体停服结算资源，或显式重启后按原义务继续核验。不新增后台项目结算器。人工候选退出仍不产生资源停止授权，不引入逐项目暂停/脱离协议。

### 5.2 整体停止

整体停止先同步关闭全部项目的新业务和 caller 派发门槛，禁止初始化迟到结果启动项目。排空已进入的项目调用和正在进行的应用初始化，汇集已提交事实；调用有界，不能因一个项目失败而跳过兄弟或共享服务收尾。

应用 owner 对原 Runner 实例发出唯一的服务停止意图，复用固定 request_id 的未知对账/有界补查；不得由每个项目分别 stop，也不能因项目库已关而漏停初始化时创建的服务。Runner 按原冻结 stop_execution/continue_execution 策略处理全部 Run。策略由服务真正停止触发；不得将 continue_execution 强制转换为逐 Run stop。

宿主私有停机结果使用新的当前格式：一份应用级 Runner 结果记录是否曾初始化、原 state-dir/build/instance、原 stop request_id、服务退出确认、最终资源结算及诊断；缺失或未知不视为未启动。逐项目只记录原实例、本地关闭、锁释放和本地诊断，删除项目级 `ResourcesSettled`。不把同一全局真假复制到每项目，更不在整体停止后重新修补各项目的资源状态。只有核实从未尝试服务/外部调用时才可记无资源；空候选列表不能推出服务无资源。

Runner 的 `last_shutdown` 仍是服务范围证明，由应用 owner 统一核对原实例/请求/目录/EXE；有界补查可重复读同一原证明，不以“只调用一次”禁止必要对账。共享证明失败时保留全局未核实，不推测哪个项目已安全；原先退出的项目仍参加本地关闭汇总。

正常 stop 成功要求原宿主停机结果合格、共享 Runner 最终结算合格、全部项目本地关闭/锁释放合格、原宿主和 Web 的精确退出及租约核验通过。Runner 继续外部执行、停止未知或证据错配均返回非零。本地关闭失败仍保留锁/宿主；仅外部资源未知不妨碍记录失败停机并关闭已完成的本地资源。start 180 秒、stop 120 秒、verify-stop 60 秒的外层预算保持，调用方超时不生成新停止意图或强制释放未关闭资源。

### 5.3 换代与离线取证

Runner 单独换代后，整个应用的借用者继续绑定原实例，业务与查询经原 `--expected-instance-id` 围栏拒绝替代实例，不局部重绑。显式重启 SteerLine 后才以同构建共同绑定新实例，各项目先只读核对自己的原 Run 或原 submit 请求；不一致项目不可准入，兄弟不被其局部错误停止。旧停止不能影响新实例。换构建只用无旧执行状态的新环境。

`verify-stop` 在原宿主/Web 退出、各项目原配置/checkpoint/锁满足现有前置后，按 home 获取一份公开 `service verify-stopped` 证明并核对全体项目的 Run 与原 input_digest，删除逐项目重复取证/嵌入同一证明的结构。退役后的公开证明只含 Runs，不要求旧 Checks 字段。unknown submit 若尚未通过原请求查证建立资源绑定，现有离线 API 无法恢复该映射，继续阻断；不因列表为空推定未受理，不扩展 Runner 或读其私有库。验证不启动服务、不改旧失败回执/DB/授权，另存新证据。显式重启时，每项目在开放入口前只读核对已存 Run 与未知操作的原外部请求；核验失败只关闭该项目本地资源，兄弟项目和共享实例继续沿原绑定。

## 6. 调试重置、消费者与文档

唯一调试生成器与 `debug-template/` 继续使用。reset 为整包操作：先核验本包宿主/Web、所有成员项目本地资源及共享 Runner/全部 Run 均已停止，再处理各项目任务/执行/UI 状态。任一项未核实则在修改前拒绝；零项目库也要检查共享 Runner，不能提前返回绕过服务检查。

保持无备份、保留任务定义/契约/依赖和 Git/PR 事实。reset 保留应用构建 pin、整个共享 Runner 目录及其历史/认证/仓库材料，删除原逐项目 `RemoveStopped` 路径；不直接读写其私有库、不增加 Runner reset API。项目 UI 和候选重置不等于抹去 Runner 历史。已绑定活动的资源清理采用本次共享停止证明，不能用停服前缓存的 cleanup 状态否决原资源已结算的证明；尚未核实的操作仍阻断重置。重置后明确新建的产品操作生成新的 operation ID，避免重用历史 Runner 回执；正常重启/未知对账则始终沿原信封。多库重置不承诺跨 SQLite 原子提交；中途失败明确记录进度、保留原证据，允许同包再次核验并完成重置，不伪报整体成功。

同步生产、CLI、项目环境准备、原生查看/最近消息、部署停服/verify-stop、debug driver、cleanup、公开 CLI fixture 和 Web helper 的所有目录/所有权假设。共享 fixture 按请求仓库/correlation及已保存 Run 绑定选择 provider，不能再把唯一 state-dir 映射到某一个项目。fixture 只模拟公开契约，不重建 Runner workflow。

实现时更新当前架构、集成运行指南、测试清单及 worker-lifecycle 的当前实现；历史 Spec/验收保留原证据，并以本 Spec 标明后继差异。当前实现与验证结果在专属验收登记，历史基线不作为当前所有权说明。

## 7. 最小验收

| ID | 通过条件 |
| --- | --- |
| SRS-A01 | 应用在现有 census 后初始化一次并把同一绑定交给两项目；零项目/无可启动项目、Close-before-Start 零启动；后续项目 prepare 失败不触发重启/误停，初始化与整体停止竞争可收尾；不同 home 隔离 |
| SRS-A02 | home 短路径/预算/私有性/别名拒绝正确，全部消费者同源；应用构建 pin 在 service start 前核验，缺失/错构建/旧目录零启动，reset 不解除绑定；项目候选/活动保留原绑定且不迁移 |
| SRS-A03 | 同号产品请求跨项目/业务/环境活动隔离；原请求重复复用原随机 Runner ID；动作先保存而外层回执丢失/并发重试不多建 Action；未知/重启仅查原信封，错 Run/input_digest/correlation 拒绝 |
| SRS-A04 | 项目退出不停止共享服务或兄弟，未完成义务保留且不伪称 actor 仍在对账；整体门槛/排空/原停止/最终证明及预算成立，只有一份共享资源结论；旧实例/未知不假报成功，显式应用重启只核对原事实 |
| SRS-A05 | verify-stop 一次共享核验且覆盖全部绑定/未知请求及活动，旧结果不改；整包 reset 全体预检、保留共享目录/历史/认证/Git/PR、重置后新操作不误用历史回执；部署/夹具/cleanup 不再逐项目停同一服务，失败不伪报成功 |
| SRS-A06 | 新 L3 controlled 双项目共享场景中两 Run 并存、同号请求不串、项目事实独立、整体停启零自动新输入、完整清理；既有等待合并重启回归通过，局部故障/写者身份沿 L1 验证 |
| SRS-A07 | 新双项目包使用固定真实 Runner EXE，两个项目绑定同一真实 PID/instance/state-dir，重复启动无额外服务，正常停启/全包 reset 证据完整；该项只证明真实服务拓扑，不冒充真实模型双 Run 执行 |

SRS-M01：自动项完成后，用户可后置体验双项目启动、项目错误诊断和整体停止；仅阻塞日常体验签收，不阻塞无关开发。实时结果均归专属验收。
