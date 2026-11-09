# 主 Agent 提示词建议模块：generate 接入 Plan

状态：用户已授权实施并更新现有 PR #478，未授权合并或部署。目标见[Spec](../spec/main-agent-advisory.spec.md)，实际结果仅登记[验收](../testing/acceptance/main-agent-advisory.md)。实现基线为 448ddda0，保留纯 contract/core 和最小 Web/活动结构。

## 1. 复用与取舍

直接使用现有 Runner generate/1，不改 Runner、不创建新 workflow 或原生链路。只读工具与隐式上下文按 generate 原契约，不再要求严格零工具 capability。来源改为持久开发候选的明确待答问题，使用原 Attempt/Question 身份；用户已明确接受普通待答。删除不存在的两类原因码依赖。

## 2. 实施顺序及所有者

1. 更新 Spec、Plan、设计与提示词治理，明确硬边界（模块不执行业务）和行为要求（只使用提供材料）。固定提示词删除“没有工具”的不实能力声明；记录实际正文 diff、字节/hash与黄金样本，经独立审核。
2. app 读取已持久候选的真实待答投影，冻结原题、版本及限定材料；无 question 不推断。由后台登记，不从 Web 发起。准备失败保存已取得材料，不让有效问题因 goal 过长而静默消失。
3. bridge 接入现有 runnerclient/runnerbinding。准备后冻结唯一 generate 请求、Sol/high 本地只读配置和完整文本；加入共享 ContentQueue，保存 started 后首次提交。所有网络调用位于核心外；查询层保持纯读。
4. bridge 以原请求身份异步观察，使用 Content binding 验证原请求/Run/correlation/digest/revision。经核实完整文本交核心 Accept，等待资源结算后终态。未知不重投；取消和源解除持久化原停止信封，迟到结果不能覆写取消。关闭等待原生资源结算，不能提前释放配额。
5. 把 advisory 加入 app 的构建 namespace 检查、启动前停止准备和离线停止核验，恢复原记录而非重建输入。存储 pending 范围覆盖所有未完成/未结算工作，活动删去内部执行字段。无旧数据兼容或迁移。
6. Web 更新 generate 阶段和文案，继续只读/复制/取消；无新功能面。用真实持久待答→后台→generate→结果→活动的受控集成证明生产接线，并通过一次真实 Runner/本地 Codex 验证及停止证据。

## 3. 验证安排

正式测试选择见同名 [.tests.json](main-agent-advisory.tests.json)，定义/环境/通用断言沿[测试指南](../testing.md)及正式清单，不复制定义。

| 验收 | 测试安排与原因 |
| --- | --- |
| A01/A02/A03/A09/A10 | 核心/contract/prompts/依赖守卫定向及 l1.minimum；确认纯核心、字节限制、实际完整正文和唯一输出 |
| A04/A05/A07 | bridge 定向：唯一提交、未知只查、重启、关联伪造、结果/结算分开、取消/源解除/停止回执、共享配额；原 Runner generate 公开客户端验证 |
| A06/A08 | go.http、web.unit、生产构建；旧动作拒绝、仅展示/复制/取消、模型设置保持本地 |
| A02/A11/A12/A13 | app 真实 SQLite 待答持久化集成、go.storage、query/Web 活动定向；不只手工传 Source fixture；同一问题恢复时只恢复来源/提醒，不替换快照或重开生成 |
| A07 关闭/恢复 | go.deployment，覆盖启动前 pending stop、namespace 与离线结算；smoke.delivery 为固定 L2 必跑 |
| A04/A05 真实链 | runneracceptance 标签的 bridge 定向测试：显式固定 Runner/Codex EXE、真实 Sol/high、完整 request/result/source/settlement，不创建开发任务或真实 PR |

每次独立 out，定向不冒充全集；失败和复跑分别保留。HTTP 压力与其他压力/E2E 串行；真实测试只运行明确用例，不能 skip 冒认通过。源码完成后固定提交跑 L1/L2，导出受测 SHA 的 Git 历史。人工浏览器可读性/导航/复制未观察就列未验，不阻断已证实自动项。Runner 源码无修改，因此无需 Runner worktree；若后续确需修改，必须另建 worktree。

## 4. 正式 L4 接入

新增现有runtime suite内的 runtime.main-agent-advisory（L4/controlled），映射A07/A08/A11–A14。沿 Scenario/Project/公开CLI peer及Playwright，来源从脚本化公开原Question进入生产观察路径；不构造Source、不改SQLite，不增加测试框架。夹具只补限定建议请求、回执丢失/查询不可用、结果与停止结算脚本及请求计数。Web实际验证活动精确导航、上下文、复制和无执行入口。同步正式JSON、L4说明、manifest、执行器Web构建/串行/必要证据和清理校验。每次新out，完整场景先定向跑通，再在干净提交运行L1/L2及本L4，登记实际证据；对应修改的controller/fixture/driver定向回归，已有活动场景复核防公共夹具回归。提示词资产无变化，提交前独立审核实际请求断言及测试接线。
