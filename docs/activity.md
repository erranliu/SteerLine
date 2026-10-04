# Activity 契约：数据库事件与服务控制台

中心活动只随业务状态写入 SQLite events。产品 Activity 查询绑定明确的项目/Run/Execution/Attempt，以数据库 EventID 排序和推进游标；没有中心 activity.jsonl 镜像、文件 seq 恢复、格式探测或 raw fallback。

## 活动记录

活动 payload 使用 ActivityEvent v1：UTC ts、source、run_id/task_id、item_id、kind/status、title/detail、elapsed_ms、meta 和原始引用。元数据带 execution_id/attempt_id。身份、唯一外部事件键和 payload 与业务状态在同一事务提交；文件写入不参与活动采集或恢复。

数据库 EventID 是查询排序与分页依据。payload 的 seq 不再从文件生成；产品客户端用 EventID 填充展示 seq。相同来源和 item_id 的规范活动保留原去重规则，不通过重读文件去重。

完整消息正文和 raw 证据归 ConversationEvent 的正文/修订引用，活动摘要不代替会话正文。不同 Run/Attempt 和晚到原 Attempt 的事实保持原归属，不能以 task_id 改选另一个运行。

## 原始取证与采集

原始 events.jsonl、实现的 history/source/mapper 和会话文件采集保留。通用 JSONL framing 只消费完整记录，保留 UTF-8/CRLF、物理 offset、文件截断/替换、扫描/记录预算与超长行后的继续采集；不解释活动 seq。数据库或采集错误明确返回，产品不从旧镜像补画。

旧 activity.jsonl 文件不再读取、写入、修复、迁移或自动重建。诊断通过现有规范活动/会话查询和原生日志进行；删除旧镜像不影响恢复。

## 服务端控制台

`steerline run -console pretty|summary|json|off` 保持。TTY 下默认 pretty；summary 输出生命周期，json 输出活动 JSONL，off 静默。json 控制台 seq 只表示当前服务输出流的顺序，不作为数据库身份或恢复游标。

破坏性 payload 变更才提升 ActivityVersion；本次仅移除诊断副本，活动 v1、FixedExecutionProfile v6、SQLite 12/runtime 17/PolicyResources v3 保持。未新增兼容读取或事件重放路径。
