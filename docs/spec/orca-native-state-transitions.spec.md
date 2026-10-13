# Spec：Orca 单终端原生状态切换

后续普通终端补查询及无需观察 working 的受理规则以 [Orca 完成补查询 Spec](orca-completion-query.spec.md) 为准；本文保留原实现记录。

日期：2026-10-12。状态：原生配对接入已实现；本机 `worktree.ps.mainAgent` 正常结束补查询验证通过，状态门槛及阶段接线已实施，最小验收通过。普通开发，不占 Task ID，不修改安装环境。

[Plan](orca-native-state-transitions.plan.md) · [专属验收](../acceptance/orca-native-state-transitions.md)

## 问题与范围

SteerlineTest 101 的 publish 仍在执行时，adapter 将上一轮 implement.report 的回复认作发布完成，提前发送 publish.report，干扰了实际发布。

本次只修正 Orca 单终端的执行状态判定：只接受原主 agent 明确的状态切换；长时间缺少信号时补一次只读查询。屏幕继续用于采集普通回复，不再作为结束信号。

保留原终端、原输入和共用阶段流程，不修改 Orca，不使用受监督结算、worker_done、模型结束标记、结构化对话或 agent 私有日志。本机与 environment 复用同一判定；不新增业务阶段、配置项或独立状态框架。

## 状态来源与接受规则

使用 Orca 已有的 `session.tabs.subscribe` 接收普通 terminal 的状态更新。结束补查询读取 `worktree.ps` 中原 pane 的 `mainAgent`；`session.tabs.list` 只用于终端身份/元数据核对，其界面 done 不作结束证据。[订阅接口源码](https://github.com/stablyai/orca/blob/main/src/main/runtime/rpc/methods/session-tabs.ts)

本机实际探针：CLI 原生认证管道可查询，但订阅需要流式配对连接。用户提供的 runtime 配对已验证可用，现有 WebSocket 加密认证、`session.tabs.subscribe` 和 `session.tabs.list` 均已接通。接入从进程环境或原生 environment 存储读取凭据，不写入业务配置、封套或证据。用户另行要求保存的配对链接放在仓库外个人私有目录，验证时由调用方注入进程环境。

此前两轮 `session.tabs` 样本的 working 带 hook 来源和顺序，done 则丢失 observation/mainAgent；同接口补查也无效。新隔离包改查 `worktree.ps` 后，两轮均取得原主代理 working→新的 done，旧轮 done 被拒绝。该查询的主代理记录有独立状态时刻，并保留原证据 updatedAt；不因查询快照缺少事件 revision 而拒绝它。不能以整体工作区状态代替主代理状态。

中断样本和本机安装包的纯规则核验表明：明确 Interrupt 与推断中断都可写出 `mainAgent.done/outcome=cancellation`，查询不能区分两者，不能按普通完成接受。普通 Stop 的原生 outcome 缺失，保留未知。详细字段、通过范围与限制见专属验收。

发送前取得原终端状态基线，沿用现有 runtime/terminal/incarnation 与 Command/原请求绑定。只读取原主 agent 状态；工作区总体状态和子 agent 状态不能代替它。接口或字段不能证明来源、身份和新旧顺序时，明确保持未核实，不退回屏幕猜测。

| 信号 | 行为 |
| --- | --- |
| input_accepted | 只记录输入已接受，不判为执行开始 |
| 原请求的明确 turn_started 与本轮新的主 agent working | 记录本轮执行开始 |
| waiting / permission / blocked | 记录等待原因，不判为轮次结束 |
| 本轮开始之后、同一原实例的新主 agent done / ended | 记录轮次结束，读取本轮普通回复 |
| 初始基线、旧记录、重复快照、推断/合成 done、其他目标或子 agent 的结束 | 不触发本轮结束 |
| 订阅不可用、身份变化或记录归属不明 | 保留最后可信事实及具体原因，不投递下一阶段 |

结束记录必须来自明确的原生主 agent 信号，且可核实它晚于本轮开始、属于当前唯一输入。仅有 done 或较新的 updatedAt/stateStartedAt 不够；快照发布版本也不等于原生状态切换。重复信号不重复结算，迟到信号无法归属时不接受。waiting 后重新 working 仍属同一轮。

同一原终端一次只投递一轮。人工或其他输入使本轮归属不明时，不自动接受结果。订阅事件校验原生来源和事件顺序；查询快照核验固定 runtime、原 terminal/incarnation/pane、本轮明确开始及主代理状态时刻，拒绝旧记录和未核实的中断。prompt 是最多 200 字符的预览，不作为唯一轮次身份或完整输入证明；生产绑定沿用原请求回执。事实通过现有 lifecycle/observer 交给 Service 单写者接受，不增加第二写者。

订阅固定 runtime/handle/pane；每次读取状态前后通过既有 `terminal.show` 核验原 PTY incarnation。`session.tabs` 的 incarnation 元数据不保证存在，明确返回不一致时拒绝，缺省时也不能省略 CLI 身份核验。本机 1.4.224 与远程 1.4.223 已核对该字段差异。

## 缺少信号时补查询

连续 30 秒没有新的可信状态信号时，对原终端补一次有界只读查询，每个执行最多一个查询在途。同一无信号窗口不重复发查询；新的可信信号或新轮次开始后重新计时。原 pane 的新 done 通知另触发一次主代理确认查询；该通知只作查询提示，不能直接结束本轮。重复同一通知不重复查询。补查仍 working 后，后续新 done 通知仍可再次核实；不建立周期轮询回退。期限沿用现有 CommandTimeout，不增加退避策略或配置。

查询到可核实的本轮新结束记录，可以补认结束；仍是旧记录、状态缺失、查询失败或超时，就保留等待/未核实及原因。查询不补造 working，不要求模型补报，也不重发输入或补 Enter。订阅和查询同时返回同一记录时只接受一次。

## 轮次结束与结果处理

主 agent done 表示本轮停下来了，不表示业务任务、工具或 PR 成功；原生 outcome 缺失时保留未知；成功采集回复与原生轮次 outcome 分别保存。[主 agent 状态定义](https://github.com/stablyai/orca/blob/main/src/shared/main-agent-status.ts)

结束信号到达后，核验原生 Codex Stop 的普通 lastAssistantMessage 与完整可见回复一致，再保存该本轮普通原文。原生字段上限为 8000 个 UTF-16 单元；可能截断的边界或缺正文保持未核实，不把前缀当完整结果。屏幕折行/JSON规范化只用于核对，保存原生普通原文以保持中文、空格与 URI。屏幕未刷新或完整回复缺失时继续只读采集，不冻结上一轮正文，不投递下一阶段。屏幕稳定、回复摘要变化或旧 Worked for 行均不能提前结束本轮。

普通回复中的 inline Markdown 链接、代码和粗体按 Codex 实际渲染核对；链接显示为名称或“名称 (目标)”时只接受由本轮原文生成的显示形式。保存的原生正文不可被后续不同原文覆盖，即使忽略空白后的显示摘要相同。

只有本轮明确结束、完整普通结果已保存，才沿共用流程解析业务 outcome 和推进阶段。publish.report 只处理真正结束的 publish 的业务结果，不用于修补状态或传输证据。正文可与上一轮相同，归属取决于本轮证据而非文本差异；已有中文、折行和 JSON 处理保持。

在现有资源封套中保存必要基线和最后接受的状态证据，沿用现有幂等提交；不保存完整平台状态历史。实际字段变更需要时更新 codec，不新增旧记录兼容。共享流程不解析 Orca 原生字段；PR 真值核验保持现状。

## 最小验证

验证原生接口/状态样本、状态接受与补查询的定向测试、受控 publish 阶段回归，并在独立 live 包中进行本机和远程首轮真实 PR 验收。用户已取消第二轮并要求整理提交；已验、未完成和取消范围统一登记专属验收。

真实 PR 仅针对 erranliu/SteerlineTest 本次专属 base/head；记录原终端、输入/结果和合并交付事实。仍不跑完整测试集或单独人工验收。实际接口或信号不可核实时记录未通过，不用其他执行路径代替。
