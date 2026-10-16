# Orca 提示词最小探针（2026-10-10）

用户要求：先验证调用 Orca 执行一段提示词能否稳定完成；Orca 路径不承载特定 agent 的参数、按键、文本识别或日志知识。普通直接调试，不进入 roadmap。

源码已 rebase 到 `42f36428`。独立现场由 `scripts/new-debug-release.ps1` 创建：`C:/SteerlineDebug/dev-20261010-114743-42f36428/`，AI 调试配置显式选择 simulate。下面原生探针只在该包独立仓库和自己创建的临时 Orca 终端运行，没有启动安装版业务任务、复制安装版数据/配置/认证或修改 PR。

## 最小入口与通过条件

[基础提示词探针](https://github.com/erranliu/SteerLine/blob/20f347cdec99dc0f0701b41311112f7e330389b6/scripts/probe-orca-terminal-prompt.py)仅调用 Orca `terminal show/wait/send`。接收器是已经在本包工作区就绪的原终端，agent 标识由 Orca 管理；探针不选择模型、不添加 provider 设置、不读 provider 日志或解析 TUI 文本。

一次请求计算一个随机验证任务，并只在本包 evidence 写出带唯一 nonce 和计算结果的 JSON。通过必须同时满足：原 runtime/terminal/incarnation 不变、输入被接受、Orca 原生 `turn_started` 事实属于原请求、计算结果精确相等、相同 request ID 重放返回 `replayed=true` 且仍指向原请求。接受、TUI 空闲、输出有文本或等待时间都不能单独证明执行成功；未知状态不补 Enter、不重发原文。

```powershell
python scripts/probe-orca-terminal-prompt.py --root <本次生成包> --orca <本次确定的CLI> --terminal <本包已就绪接收器> --lengths 500,7500 --repeat 3 --budget 90
```

脚本不负责初始化代理的登录、升级弹窗或权限配置；这些是独立前置。原生默认冷启动曾报告 `agent_readiness / agent-update-prompt`，该尝试失败并释放，不能计为启动稳定性通过。测试接收器的一次性启动配置属于独立夹具前置，没有写进探针或产品适配器，也不作为输入问题的解决方案。当前结果只证明本次已就绪接收器的最小输入通道，不泛化为所有 agent、冷启动、远端或完整交付。

## 结果

Orca CLI/runtime 1.4.223。本机 runtime `e11d4024-70e8-46c6-b86a-e17338831ba9`。

| ID | 方法与范围 | 结果 / 证据 |
| --- | --- | --- |
| MPI-A1 | `terminal.send --text ... --enter --wait-submit 10 --retry-request <UUID>`；短提示和约 7500 字符提示各重复 3 次，两批连续执行 | 已验收，本次 12/12：482 字符 6 次、7477 字符 6 次；全部 `input_accepted + turn_started`，nonce/算术结果一致；7.09–14.22s。首次独立现场脚本在 `evidence/orca-transport-f8f12ea1/`；加强 runtime/原请求/incarnation 核验后的仓库入口复验在 `evidence/orca-transport-2436f661/`。各目录 `result.json`、send/replay 回执及实际结果文件保留 |
| MPI-A2 | 同一原生请求重放，比较 request/incarnation 和原生 replay 事实 | 已验收，上述每次完成后用相同 UUID、相同提示和目标重放；12 次均返回原请求和 `mutation.replayed=true`。没有因静默而生成新投递或额外 Enter |
| MPI-A3 | [worker-start 对照探针](https://github.com/erranliu/SteerLine/blob/20f347cdec99dc0f0701b41311112f7e330389b6/scripts/probe-orca-prompt.py)：原生 Task/Dispatch、nonce 结果、结算、重放及连续轮次 | 验收失败。`evidence/orca-prompt-c6113b8f/` 中两轮短提示完成，第三轮原生返回 `agent_unconfigured`；增加原生 `tui-idle` 和原身份核验后，`evidence/orca-prompt-312ba029/` 首轮完成，第二轮在 90s 内无唯一正确结果，`turnStart=unsupported`。短任务 Spec 为 491 字符，Orca 自身还会添加完整原生 preamble；不能冒充基础 send 通道通过 |
| MPI-A4 | 原始现场症状的独立复现 | 已验收，`input-start-reuse.json` 中原生返回 `outcome_unknown / turn_start_unobserved`，`input-draft-before.json` 保留原生公开 draft 字段。仅该早期诊断实验补过一次 Enter，之后原 Dispatch 完成；此人工性质的实验不计自动稳定性通过，也不进入后续探针逻辑 |
| MPI-A5 | 脚本语法及本地导航检查 | 已验收，`python -m py_compile scripts/probe-orca-prompt.py scripts/probe-orca-terminal-prompt.py`；文档链接和 diff 检查通过 |

两条入口的提示词、完成回执形式和原生附加 preamble 有差异。本次可以缩小问题范围，但不能仅凭 A/B 结果断言某个内部函数丢了 Enter。现有 `worker-start` 的 accepted/ready 也不能代替对应的 turn-start 或真实原结果证明。

## 当前收敛与清理

按照用户“先做最小探针”的调整，先前通用回执/恢复实现草稿已保存到 Git stash `orca-input-contract-wip-before-minimal-probe`；当前工作区只保留探针与验收记录，未发布新产品修复或 PR。后续改动以 MPI-A1/A2 的稳定投递事实为基准，先复核 Orca `worker-start` 与基础投递的差异，不加入 agent 专用判断。

失败的原终端属于探针自己创建的外部接收器，Orca `worker-stop` 明确返回 `stop_unknown / external terminal`，release 为 retained；没有把它记为已停止。随后对本次确切 Dispatch 显式 abandon，确认 fencing，再关闭自己创建的接收器，关闭回执 `ptyKilled=true` 保留。其他已结算 probe worker 按原生 release 处理。原先独立大样本和配置实验的临时终端已关闭。此次剩余基础接收器和 coordinator 在验证结束后关闭，清理结果保留在该包 evidence；保留独立仓库和所有证据用于复查。

## 1.4.224 原生启动复验

独立包：`C:/SteerlineDebug/dev-20261010-130631-42f36428-dirty/`；simulate 夹具，未使用安装版业务现场。CLI 与运行时均为 1.4.224，runtime `fb7953d3-29a6-4f81-b6b9-fa257bb98d04`。

探针增加互斥 `--agent <opaque ID>` / `--terminal <handle>` 两种入口。前者完全由原生 worker-start 创建代理，不再依赖手工 terminal.create 的识别状态；不传模型、effort 或代理设置。每个成功样本仍须证明原 Dispatch 的 nonce/算术结果、原终端 incarnation、结算及相同请求重放。失败启动先验证原请求重放，再仅按回执明确提供的 worker-release 清理。执行中的预算用尽只保存 worker-show/read，不补 Enter、不重复创建任务、不自动取消。新样本保存脚本快照、SHA256、CLI 版本和每次原生回执。

```powershell
python scripts/probe-orca-prompt.py --root C:\SteerlineDebug\dev-20261010-130631-42f36428-dirty --orca C:\Users\lmas1\AppData\Local\Programs\orca\resources\bin\orca.exe --coordinator term_959c5285-384f-4f63-ae6a-70086d7e4fd9 --agent codex --lengths 500,7500 --repeat 3 --budget 120
```

agent ID 仅作为调用参数，探针没有对应代理的配置、按键或文本分支。

| 验收项 | 本轮最小验证 / 证据 | 结果 |
| --- | --- | --- |
| MPI-A3（冷启动） | 原生新建 agent：491 字符任务；`evidence/orca-prompt-eb36983e/` | 阻塞。ctx_ce4b45fbe75d 在 agent_readiness 返回 agent-update-prompt；没有任务投递/执行证据。release 确认 closed_agent_terminal |
| MPI-A3（冷启动长任务） | 7500 字符任务；`evidence/orca-prompt-acd16154/` | 同样阻塞，ctx_4f69dbe6ee37 尚未进入提示词提交阶段；不能归因于提示长度，也不能报告原来的 Enter 问题已解决 |
| MPI-A2（失败请求幂等） | 第二轮相同 UUID/目标/spec 重放 | 通过，返回原 ctx_4f69dbe6ee37，replayed=true；清理回执 closed_agent_terminal，无替代 Dispatch |
| MPI-A1 / MPI-A3（已就绪输入） | 在独立调试项目的已就绪 agent 上分别运行基础 send 与 worker-start 短/长提示各三轮 | 本轮待验。启动弹窗属于独立前置，未修改代理设置绕过，也未冒充投递测试通过 |
| MPI-A5 | 两脚本 py_compile、git diff --check、原生失败/重放/释放路径实测 | 通过 |

结论范围：本机当前默认启动环境下，升级后 worker-start 冷启动仍不能执行任务；本轮尚无法比较已就绪终端上的提示词提交可靠性。失败终端均由 native release 清理，本次 coordinator 已关闭，关闭回执及两 Run 的 inventory 保留在包 evidence。后续复验需在同一独立项目新建 coordinator，命令中的旧 coordinator handle 仅用于追溯历史执行。

## 1.4.224 处理启动弹窗后的输入复验

用户随后明确授权处理弹窗并继续探针。仍在上述独立包运行，CLI/runtime 未变化。仅在测试环境通过原生终端读取确认升级菜单，先按菜单提示 Esc 跳过；后续另一个临时终端选择菜单的“Skip until next version”，以允许默认原生 agent 创建路径继续运行。没有安装/升级代理、直接编辑配置或加入代理专用参数。这是夹具准备操作，未写进探针或产品 Orca 适配逻辑；菜单跳过会记住当前可升级版本，应与投递实现变更区分。弹窗前后读取和选择回执保留在包 evidence 的 startup-popup-* / popup-menu-*。

| 验收项 / 范围 | 本轮结果 | 原始证据目录或文件 |
| --- | --- | --- |
| MPI-A3：已就绪终端连续 worker-start 短任务 | 前两轮 491 字符 Spec 正确计算并结算，分别 57.82s、22.95s；相同请求重放通过。第三轮在前置 show/wait 仍识别且空闲时返回 agent_unconfigured；无 Task/Dispatch ID。连续稳定性仍失败 | evidence/orca-prompt-ffa5ccec/result.json 和第三轮 start 回执 |
| MPI-A3：同一终端独立长任务尝试 | 7500 字符 Spec 在原生 preflight 返回 agent_unconfigured。request-show 为 absent，task-list 无任务；没有盲目重放或把 absent 当作无副作用证明 | evidence/orca-prompt-d62cb8e4/，包含 request-show/task-list/show/read 诊断 |
| MPI-A3 / MPI-A4：默认 worker-start --agent 原生创建 worker | 7500 字符 Spec，Orca 加入 preamble 后终端 draft 为 12288 字符。ctx_e799ece7e4b4 返回 outcome_unknown / turn_start_unobserved，turnStart=unobserved，prompt.stages 只有 input_accepted，原 request/incarnation 对应；120s 内没有唯一正确 worker_done。复现提示词留在输入框，没有补 Enter | evidence/orca-prompt-a0f8903e/；fresh-long-draft.json、fresh-long-final-draft.json、fresh-long-before-stop.json |
| MPI-A1 / MPI-A2：基础 send 短/长各三轮 | 本轮 6/6，通过原请求 input_accepted + turn_started、nonce/计算输出及幂等重放。实际提示长度 488 和 7483；8.83–15.70s | evidence/orca-transport-b9bcc2ef/ |
| MPI-A1：基础 send 接近 native 完整投递长度的对照 | 12294 字符输入首次即未确认启动，10s 原生提交观察结束只有 input_accepted；之后公开读取 draft 仍为 12295 字符，且无本轮结果文件。本批在第一轮停止，不能计为三轮失败或120s预算失败。证实基础通道也不能泛化为所有长度稳定 | evidence/orca-transport-ea97e1c7/；transport-12k-draft.json、transport-12k-binding.json |
| MPI-A5：探针加强与检查 | 增加 preflight 错误 code/message、无 Dispatch 时原生诊断、原请求/进程回执核验、基础 send 失败时 show/read、执行脚本快照。脚本 hash 固定读取快照，避免运行期间源码编辑导致证据 hash 漂移。py_compile 和 diff --check 通过；本轮保存执行版本，新增诊断不冒充已经跑过的完整稳定门槛 | 两脚本与各样本快照；a0f8903e/result.json 的 hash_source 说明现场 hash 纠正依据 |

**本轮结论：1.4.224 仍可复现输入后未提交，不能认为 worker-start 已稳定。** 原生新建 worker 的复现排除了仅由手工终端 agent 识别造成的解释；基础 send 在接近完整投递长度时也卡在 draft，不能仅依据较短提示词通过就采用它作为完整解决方案。长度相关性是本轮对照发现，不能据此断言内部根因或确定阈值。默认代理/模型及夹具准备与此前 1.4.223 样本并非严格一致，本轮不做版本变化的因果归因。

探针不解析代理 TUI 文本决定操作，不读代理日志，不按 agent 类型分支，也不对静默补 Enter。公开 draft 仅作为现场诊断证据；通过仍须绑定原请求与结果。产品修复草稿继续留在 stash，未发布未经验证的替代投递方案。

清理：在预算结束后，对原生创建且仍持有未提交 draft 的确切测试 Dispatch 显式 worker-stop；回执 stopped / closed_agent_terminal / ptyKilled=true。随后按 inventory 的明确 nextAction 执行 worker-release，回执 released / processAction=none；没有将取消后的结算记为计算成功。其余 caller 创建的测试终端及新 coordinator 均关闭，关闭回执保存。最终本项目 terminal-list 为空，本 Run 的 reclaimable 列表为空。保留独立包和所有证据，未改安装版业务任务。

## 1.4.224 原生新建 worker 短提示词三轮对照

用户要求进一步单独验证短提示词。相同 CLI/runtime、独立目录、默认 agent 启动条件；每轮 `worker-start --agent` 新建终端，不复用先前手工启动的接收器，不更换模型或加入额外代理参数。入口仍是 probe-orca-prompt.py，参数 `--lengths 500 --repeat 3 --budget 120`，本轮 coordinator 为 term_0cf01051-cced-4818-9d7c-5ef5f018342f。

MPI-A3 本轮短输入范围 **3/3 通过**：实际任务 Spec 均为491字符，Orca另追加原生 preamble；三轮均 ready / turnStart=observed，input_accepted + turn_started，原请求/进程绑定正确、nonce/计算结果正确、原 Task/Dispatch 完成、同请求 replay_verified=true，无额外 Enter。耗时分别51.01s、69.16s、66.43s，包含启动、执行、结算及释放。本轮精确 Dispatch 为 ctx_bbaaea7e1602、ctx_34ddbf4c5ce3、ctx_f0e3e479b7dc。证据：`evidence/orca-prompt-cd0b5003/`，result.json、脚本快照、逐次 start/settled/replay/release/inventory 回执。

三轮 agent 均由原生 release 关闭，coordinator随后关闭；reclaimable.json及final-terminals.json记录清理。未修改探针或产品代码。

与上一轮相同原生创建路径的7500字符任务未提交相比，短提示词通过支持“完整投递长度可能影响可靠性”的判断；样本有限、未控制所有时序变化，尚不能证明长度是唯一根因或指定安全阈值。短启动指令引用工作区内任务文件、让worker读取完整任务内容，可以作为下一步代理无关的验证方向，本轮没有实现或宣称该方案稳定。

## 2000 字符步长扫描与反例

随后用户要求逐档扫描，每档三次。worker-start 扫描现场 `evidence/length-sweep-af02ef89/`：精确500字符 Spec 三次仅1/3，另外两次的原生回执均 outcome_unknown / turnStart=unobserved，120s内无本次正确 worker_done，公开draft均5288字符。原 Dispatch 分别ctx_ff6329ed7a9d、ctx_64ae2f8c4689，观察后显式stop/release。它推翻了“短任务就稳定”的推广；前一批3/3保留为真实有限样本。2500档只完成首次（启动失败并已释放），driver对失败资源状态的检查阻止继续；本扫描未完成全部档位，不能冒充长度边界。用户随后转向基础输入扫描，旧coordinator已关闭；全部逐次回执及汇总保留。

基础输入扫描现场 `evidence/base-length-sweep-b0a4628d/`：保持1.4.224与原runtime；500开始，步长2000，每档三次，顺序执行，每次独立创建同配置的已就绪caller终端。探针精确填充指定长度，submit观察30s、结果预算90s；输入一次，不补Enter。每次通过须含原request/incarnation的turn_started、唯一nonce计算结果和原请求幂等重放。命令/driver、每次脚本快照、raw send/replay、诊断和close回执均保留。出现首个非3/3档后完成该档三次并结束，未继续向上宣称阈值。

| 实际输入字符数 | 通过次数 | 诊断 |
| --- | --- | --- |
| 500 | 3/3 | turn_started、结果与重放核验通过，无draft |
| 2500 | 3/3 | 同上 |
| 4500 | 3/3 | 同上 |
| 6500 | 0/3 | 提交观察结束只有input_accepted；每次公开draft为6501字符，本次结果文件在检查点不存在。此结果是30s提交门槛失败，不是90s计算超时 |

MPI-A1/A2本轮前三档范围已验；6500档未通过。每次caller终端已关闭，final-terminals.json记录为空。py_compile验证精确填充和新增submit-wait参数；没有加入代理专用配置、按键分支或日志读取。

本轮最后全过档4500、首个全失败档6500是有限样本的观测范围，不能承诺小于4500永不失败，亦不能建立4500–6500的确定阈值：历史7483字符曾6/6通过，500字符worker-start也出现过1/3。更合理的后续是验证短启动指令＋工作区任务文件，并保留原请求投递和真实启动的核验；不把改成unsupervised或基础send本身当作修复完成。

## 最新主干同步

用户要求后 rebase 至 origin/main `4da3dcb6`，恢复本页、验收索引及两个探针；验收索引冲突保留主干新增入口和本页入口。主干已有 worker-prompt-convergence 的三短模板及事实引用，并保持 Orca 原生 worker-start；后续方案以此实现为基线，不重复新建短模板流程。上述真实探针来自42f36428生成的独立包，不改记为最新主干的产品端到端验收。本次仅进行脚本py_compile、冲突及diff检查，未在旧调试包运行新产品代码。
