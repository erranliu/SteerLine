# Gitea 草稿发布与人工核验验收

2026-11-15。SteerLine 基线 `68829335`，Runner 基线 `cf3da565`，使用各自带本次改动的工作树。联验使用官方 Gitea 1.27.1、测试专用本机实例和 `e2e/verification` 仓库。产品行为见[人工核验设计](../../../docs/product-design/manual-verification.md#plan-与草稿-pr)。

## 发布正文

Runner 的 `internal/execution/workflow/publication-draft-gitea.md` 新增以下正文，首尾各有一个 LF。UTF-8 从 0 增至 331 字节，SHA-256 为 `87dd3bb2d2a95c6f388a48ccded72ae4af533d491b99fd2800f615d215db08da`。

> 首次发布前按固定仓库、目标分支和当前候选分支查询开放 PR。已有原 PR 时继续使用原 PR；创建响应未知时先按同一目标查询，避免重复创建。首次创建 Gitea PR 时，将发布目标的 draft_title_prefix 放在提交给平台的标题开头，使 PR 从创建时就是草稿。

Gitea 1.27.1 通过实例配置的 WIP 标题前缀创建草稿。新增固定正文说明如何在创建时应用冻结前缀，以及何时沿原目标查找 PR。实际目标在原 JSON 后新增 `"draft":true,"draft_title_prefix":"Review:"`，Runner 保留原有发布阶段。独立审核使用 `http://127.0.0.1:3000`、`e2e/verification`、`origin`、`main` 和 `Review:` 核对实际输入。Gitea 普通发布 131 字节，新增草稿目标及正文 506 字节。GitHub 固定正文为原 141 字节，代表性普通发布 136 字节、草稿发布 290 字节，前后完全一致。SteerLine 固定提示词正文无差异。独立审核确认新增槽位仅含草稿意图和标题前缀，并提出源分支删除后的 PR 引用问题，已在两端按原 PR 编号解析 `head.label`。

## 自动验证

- SteerLine 固定 L1、L2 通过。报告位于 `C:/SteerlineDebug/wt-4bd2-gitea-manual/l1-f6db4ea4cda24f0d85fd32c6ae449d9a/summary.json` 和 `C:/SteerlineDebug/wt-4bd2-gitea-manual/l2-ab8bd217d6b946d49bafbfd6cf9cd87a/summary.json`。改动后的 Gitea 查询和测试注册另作定向验证。
- Runner 的 `contracts`、`resources`、`execution/publication` 和 `execution/workflow` 包通过。定向验证首次草稿要求显式草稿状态及匹配的远端 Head，拒绝普通、未知、已关闭或不同 Head 的 PR，并保留原草稿通过后的 Ready 资格。较早对完整 `internal/application` 包的直接调用到达 Go 默认 10 分钟超时。

复跑命令见[正式 L4 登记](../l4.md)。联验使用两份独立 EXE 和一次性本机 Gitea 凭证。

## 真实 Gitea 联验

`manual-verification.gitea-local-codex` 通过，报告位于 `C:/SteerlineDebug/wt-4bd2-gitea-manual/e2e-37eea05d559940d6bd521fa060571b3e/summary.json`。Runner 构建为 `0.2.0-dev.4-gitea-draft`，SHA-256 为 `41db7c72b07c2426a91b727e98447dc198966967e8eff1abf0073d8b5c1997fe`。

真实 Codex 使用固定发布正文和自定义 `Review:` 前缀创建 Gitea PR #1。Runner 与平台同时确认初始草稿状态，待办出现后 Edge 从页面启动 `3efe2fed7871131bc7674e66da34035fc7b72e94`，预览返回该提交的内容并保存失败结论。推送 `650fd3b4c45b382c42304cc4f9abbcecfc95e1be` 后，旧提交写入被拒绝，新提交从独立 worktree 启动并保存通过结论。待办和 PR 随后关闭，源分支删除，记录继续绑定原 PR 并归档。两个预览均确认停止，两个 worktree 均已移除。通过正式任务废弃接口结算此未合并夹具的原 Run，应用正常停机，报告记录 `cleanup_complete=true`。

此验收覆盖本机 Codex 与隔离 Gitea，使用开发构建跳过环境就绪门禁。308 安装现场和项目仓库未参与写入，正式安装待部署。
