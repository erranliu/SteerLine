# Plan 发布与 PR 实现迁移

任务 ID：56；任务名称：发布与 PR 实现迁移；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-03-publication.spec.md)。硬前置：#54 合入，一个实现 PR 交付。

## 迁移步骤

1. 接收 #54 身份与工作区契约，列出 PRClient、publishLocalWorkerRun、marker、PR 关联和 TaskDelivery 的所有消费者，区分外部操作与事务。
2. 创建 publication 包，先移 PR 类型、URL/remote/分支校验和 provider，再拆发布请求、Git 操作与对账。工具配置通过小类型提供，不导入父包 Runner/Config。
3. 中心保留 publication Operation 意图和阶段回执、BindWorkerPR 和交付事务；readiness 的完成/PR 线索不受理成业务发布，也不在发布层清除 blocker。保持 intent-before-action；未知发布只读对账 index-tree/ancestry/remote HEAD 和唯一 PR，不自动补发布。
4. 迁移 provider 和临时 Git 纯/操作测试，中心保留一条代表性发布与交付接线；删除原外部实现。与 #55 串行集成共享 Git/中心文件。

## 验证

```powershell
go test ./internal/steerline/publication -count=1
go list -deps ./internal/steerline/publication
go test ./internal/steerline -count=1 -run '^(TestLocalWorkerPublishUsesHostGitAndExactPR|TestLocalWorkerPublishMarkerMustBeFinalAndBound|TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged|TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor)$'
```

provider 纯断言迁入新包，旧名字若消失记录映射。EP1 用依赖和消费者检查；EP2 用原发布 fixture 及意图提交失败断言；EP3 用错误 PR/合并门槛和已有未知发布对账 fixture。缺少不重复创建断言时在同一发布 fixture 补一次恢复重放，不重建多套端到端场景。

## 交付

PR 说明外部步骤与中心写入点、原生发布限制、保留的未知请求和认证边界。向 #57 交接固定请求，向 #60 交接支持的发布能力；不实施责任切换。
