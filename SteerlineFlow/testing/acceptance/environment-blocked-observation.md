# 环境检测阻断可观察性修复

日期：2026-11-09。受测基线为 `69ae53d93a76580ef68491f9f577a9327a8a47b9` 加本次工作区改动；报告保留实际 dirty 状态，不作为该基线提交本身的通过记录。本次未部署，未停止或重启安装现场。

## 问题与修改范围

检测 `4f18a1a9e2cc4a9faa9b06da56ea4283` 的原 Runner Run 在查询远端 Orca 仓库目录时超时，公开事实为 `blocked / operation_rejected`，尚无 verified final。活动仍未结算，原环境页面将其显示为检测中，顶层诊断为空。

- 扩展现有领域只读方法：活动诊断为空时读取当前轮、原 Run 诊断，保留提交/观察/结算原因及 ready 后的正常解释。
- 共用 `Record.Blocked()` 判定，环境就绪投影立即显示阻断；详情、历史增加只读 `blocked`，详情补齐已有历史中的 `timed_out` 语义。
- 活动中心通过原存储读取边界取得阻断事实，显示检测阻断，继续属于进行中，同时标记当前活动需要关注；不写完成时间。
- 指定检测 ID 每 3 秒刷新，页面隐藏时暂停轮询；当前卡片保留停止入口和禁止重复启动的约束。

[执行服务](../../../internal/environmentadaptation/service.go)无正文改动；无 Runner 修改，无数据库或持久化状态新增，无提示词/接线/资产改动。未新增测试级别或场景 ID。

## 回归断言

原 Run 准备失败且无 final 时，重复观察与读取仍能显示原因；状态继续为 running、未结算、未达到总超时，不生成停止请求、不采集最终结果。已结算、已停止、已取消及总超时优先保留各自状态；普通执行和等待输入不误标阻断。提交、观察与轮次诊断优先级及 ready 后不显示旧失败均有覆盖。

App 公开详情/历史、SQLite 实际活动读取与筛选、前端卡片/历史/指定 ID 轮询均补充回归；显式停止按钮仍发送原活动 ID/revision，未产生自动命令。

## 验证与原始证据

全部报告位于仓库外独占根 `C:\SteerlineDebug\wt-3e54-env-observation\`，各目录的 `summary.json` 为入口。

| 检查 | 结果 | 报告相对位置 |
| --- | --- | --- |
| L1 固定集合 | 通过 | `test-results/L1/final-1/` |
| L2 受控交付冒烟 | 通过 | `test-results/L2/final-1/` |
| L4 web.unit | 353/353 通过，无跳过 | `test-results/L4/web.unit-final-1/` |
| L4 go.environment | 通过，含全部 31 个 App 根测试 | `test-results/L4/go.environment-final-1/` |
| L4 go.storage | 通过 | `test-results/L4/go.storage-final-1/` |
| L4 go.http | 通过 | `test-results/L4/go.http-final-1/` |
| 环境/model、App 和活动 SQL 定向诊断 | 通过 | `diagnostics/go-initial/` |
| 产品查询包全量诊断 | 通过 | `diagnostics/query-final-1/` |

固定集合分别执行 `go run ./cmd/test-feedback -level=L1 -out <本次目录>`、`go run ./cmd/test-feedback -level=L2 -out <本次目录>`。L4 使用同一入口，`web.unit` 参数为 `-level=L4 -suite web -scenario web.unit`；三个 Go 专项参数分别为 `-level=L4 -suite go -scenario go.environment`、`-level=L4 -suite go -scenario go.storage`、`-level=L4 -suite go -scenario go.http`，各自指定表中独立输出目录。定向诊断使用 `-- ./internal/environmentadaptation/... ./internal/product/query ./internal/storage/sqlite ./internal/app -run 'Test(ProjectEnvironment|AsyncActivitySQLDetection)' -count=1`，查询包另以 `-- ./internal/product/query -count=1` 全量执行。

前端环境页定向 26 项及 `npm --prefix web run build` 通过。首次沙箱内 `npm ci` 异常退出（Exit handler never called），随后正常网络下按原锁文件安装成功；依赖和锁文件未修改。`python scripts/check_doc_links.py`（712 文件、4404 本地导航链接、0 错误）、测试清单/执行器一致性检查及 `git diff --check` 通过。
