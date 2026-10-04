# #74 协调复审

起始候选 `39ef87498bb12a371e7329e32863c4c496e2533c`；已集成 main `766eacc72bfdccef9cfe693b2e4032452da96877` 的 PR213 控制台增量。协调聊天在确认执行聊天停写且工作树干净后，独立检查 Spec/Plan、实际 Go diff、app/CLI/产品调用、领取/规范结果/丢弃的 SQLite 专用提交、原请求回放、任务/图/候选集合 CAS、交付/丢弃竞争和字段/文件交接。

确认并修复：

1. WorkPort 原先返回 task 应用包的 Work，工作与结果接口也定义在那里，妨碍执行线脱离 task 实现。工作纯值现归 task/model.FixedWork；TaskWorkPort/TaskResultPort 归当前执行消费方 command_boundary.go，task 服务由 app 结构匹配注入。执行生产 import 不再依赖 task 实现。无 alias、转发或新写权限。
2. 当前架构索引仍列已删除文件，并把字符串包名计为 import、破坏表格。依据 go/parser 全 OS/tag 生产源码刷新 392 文件/33 包的路径、hash、直接 import；修正旧 task 职责和 schema，当前索引见 [coordinator-source-index.json](coordinator-source-index.json)。
3. 收口活动规划中的 #74 阶段行与“待评审”记录，保留后继必要能力与固定硬前置；同步实际端口/字段/方法位置和补充最小测试计划。#77/#79–#82 的用户暂停继续保持。

独立验证：

- 集成 main 后固定 L1：passed=true（约 220.6 秒），证据在协调 run 的 review74-l1-main-integrated。
- 纯工作值调整后，任务/实际工作端口/结果 CAS/重放与交付丢弃竞争/产品接线：3 包、20 parent/subtest passed，0 fail/skip，review74-boundary-final。
- 消费方端口及所有构造切换后，任务代表、真实工作端口/产品接线、全 OS 源码和 Windows/Linux 依赖守卫：3 包、39 parent/subtest passed，0 fail/skip，review74-port-wiring。
- 最新 main 控制台代表：72 parent/subtest passed，0 fail/skip，review74-console-main。
- 当前 Markdown 相对链接/表格列数、完整 PR git diff --check 通过；go list 确认执行生产包无 internal/task import。

本任务既有 controlled recovery.waiting-merge-restart、schema 14 拒绝和构建证据已读取；协调修复不改事务/恢复语义，未重复 Full/L2/L3/live，也没有验收豁免。原始报告与独立 AST 核对保留仓库外协调 run。最终修复 SHA、复审和合并前 head/base/检查记录由 PR 评论及协调状态关联。
