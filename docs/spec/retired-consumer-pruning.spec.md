# Spec：清理退役日志解析与发布元数据

依据：用户要求执行精简建议 1、2 并提新 PR。基线 main 9413865（#202/#203/#204 已合入；测试起点 e449a09 至此只有候选文档变更）。

1. 删除没有生产消费者的 Pi/旧 exec stdout normalizer、格式探测、raw fallback 和活动 seq 解释。通用 ReadJournalBatch 的文件身份、UTF-8/CRLF、物理 offset、轮转/截断、记录与扫描预算，以及当前会话采集保留。中心 activity.jsonl 镜像保留，本次不做建议 3。
2. 删除 PublicationSettings.Create、空 CreateDescription、克隆/校验/展示消费者和退役发布责任常量。worker-only 校验、原 provider/Git/remote/auth 引用冻结及查询/关闭/真实唯一交付保留。
3. 固定执行快照升级 v6，拒绝 v5，不迁移、回填或回退。删除只读 cursor 的退役活动字段；表、Checkpoint 和原生权限契约结构不变，SQLite schema 12、runtime 17、PolicyResources v3 保留，资源仍绑定完整当前 profile 摘要。
4. 旧测试夹具采用 worker 当前责任；退役功能的专属测试/复制 stream fixture 删除。quick owner 选择转为当前会话映射/原生 history/cursor/恢复断言；现行 raw 采集和大小/游标边界不降低。

最小验收：退役符号无源码消费者；通用日志读取与会话入库/正文/游标回归通过；元数据不再含 create，旧快照/字段拒绝，冻结目标与 worker-only 保持；已确认失败的配置/CLI 夹具恢复覆盖；quick 与定向测试实际命中，构建/边界通过。只在受控闭环核对新快照的 claim、交付、重启；不重跑无关真机模型流程。
