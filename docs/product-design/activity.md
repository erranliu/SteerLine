# Activity 契约：数据库事件与服务控制台

中心活动只随业务状态写入 SQLite events。控制台通过 RecentActivity 读取所选候选当前 Attempt 的最近五条流程/状态变化、五条最近原始变化及十条其他活动；重合事件去重，共用原 1 MiB 预算，可读变化优先。右下默认只投影最多五条流程/状态消息，技术变化及其他活动摘要放入 d 详情；不提供工具正文展开或历史翻页。服务窗口输出、规范活动采集和诊断读取仍保留。产品 Activity 查询绑定明确的项目/Run/Execution/Attempt，以数据库 EventID 排序和推进游标；没有中心 activity.jsonl 镜像、文件 seq 恢复、格式探测或 raw fallback。

## 活动记录

活动 payload 使用 ActivityEvent v1：UTC ts、source、run_id/task_id、item_id、kind/status、title/detail、elapsed_ms、meta 和原始引用。元数据带 execution_id/attempt_id。身份、唯一外部事件键和 payload 与业务状态在同一事务提交；文件写入不参与活动采集或恢复。

数据库 EventID 是查询排序与分页依据。payload 的 seq 不再从文件生成；产品客户端用 EventID 填充展示 seq。相同来源和 item_id 的规范活动保留原去重规则，不通过重读文件去重。

完整消息正文和 raw 证据归 ConversationEvent 的正文/修订引用，活动摘要不代替会话正文。不同 Run/Attempt 和晚到原 Attempt 的事实保持原归属，不能以 task_id 改选另一个运行。

worker_state 的 from/to 业务字段为 workflow_stage/workflow_status，原领域记录仍用对象内 StepID/Status；缺少新目标字段的事件登记采集诊断，不读取旧字段补画。候选快照和历史 WorkflowSummary 同名，finish_task_status 已删除。详见[状态命名 Spec](../spec/state-naming.spec.md)。

## 原始取证与采集

原始 events.jsonl、实现的 history/source/mapper 和会话文件采集保留。通用 JSONL framing 只消费完整记录，保留 UTF-8/CRLF、物理 offset、文件截断/替换、扫描/记录预算与超长行后的继续采集；不解释活动 seq。数据库或采集错误明确返回，产品不从旧镜像补画。

旧 activity.jsonl 文件不再读取、写入、修复、迁移或自动重建。诊断通过现有规范活动/会话查询和原生日志进行；删除旧镜像不影响恢复。

## 服务端控制台

`steerline run -console pretty|summary|json|off` 保持。TTY 下默认 pretty；summary 输出生命周期，json 输出活动 JSONL，off 静默。json 控制台 seq 只表示当前服务输出流的顺序，不作为数据库身份或恢复游标。

破坏性 payload 变更才提升 ActivityVersion。活动版本由其 payload 契约决定，不与数据库/服务 checkpoint 版本混为一谈；持久格式统一见[数据模型](data-model.md#当前格式来源)。不通过旧镜像兼容读取或补画事件。

## 三窗运行与反馈

右上显示候选/冻结方案/原始状态/阻塞原因/PR；右下优先显示当前问题或答案预览与回执，再显示当前 workflow_stage/workflow_status 与变化，一条消息仅表示流程或状态，未变化的流程只作为状态消息上下文。execution_status/resource_status、turn ID、输入版本和恢复字段不作为默认消息。RecentActivity 在同一读取事务中核验项目/Workflow/Execution/Attempt；按预算拒绝补画过大/缺失 payload。UI 有界保留原始变化字段和诊断供详情读取，不保存工具全文，候选或 Attempt 变化清空样本并拒绝旧响应。具体规则及验收见[可读性收敛](../spec/console-flow-state-readability.spec.md)。

右下按a激活既有Unicode编辑器，Tab循环列表/选项/输入，Enter换行，F2单次提交。长背景用F8只读查看，完整最终回复用f；普通回答不切整页详情。
