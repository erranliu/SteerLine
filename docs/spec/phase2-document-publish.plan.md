# Plan：正式文档发布与任务创建

任务 ID：19；任务名称：正式文档发布与任务创建；目标：`PHASE2`。唯一执行依据：[Spec](phase2-document-publish.spec.md)。一个独立实现 PR 交付。

硬前置：#17。全部按原任务交付核验，不能按接口里程碑提前执行。

## 实施顺序

1. 以 #17 的 proposal/guard/文档快照建立 Publication DTO 与人工 publish/reconcile 命令；先固定阶段和恢复日志。
2. 实现隔离 index、确定 Git commit、CAS 分支、逐文件物化和实际 worktree index 的加锁/原子更新；故障注入验证查询版本及 HEAD/index/文件一致。
3. 实现 documents_only 路径，再接目标分支的规划提交祖先及规格 blob 双重检查、固定 task-batch payload 与原请求创建/回读。
4. 用真实临时 Git 仓库验证人工集成、文件冲突、目标漂移及引用内容门槛；不自动操作用户主线或真实任务。
5. 完成 DP1–DP9；覆盖连续发布/同步、同内容 squash 拒绝、needs_review 修订、取消与创建意图竞争、两侧部分完成和在线/离线读取。使用 #17 turn owner 夹具；若 #18 已交付，后合入方补跑消费者回归；真实 Codex 交叉验收归 #24，不新增开工前置。

## 验证与交接

按本任务验收编号逐项记录于 docs/validation/phase2-document-publish/；保存数据库/文件/Git/外部身份和前后状态，不以最终文本代替应用事实。先跑领域定向和故障测试，再跑受影响 Go 全量回归、go vet 与 git diff --check，服从所引用共同 Spec 的测试门槛。需要真机的项单列环境和未测项。公共接口变化同时更新消费者 Spec、schema 和契约样例，不通过添加旧协议兼容掩盖不一致。
