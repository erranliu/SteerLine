# L3 完全访问验证

2026-10-02：delivery/recovery 两组共六个场景均取得完整 pass 和 cleanup_complete=true。四个真实场景分别用单场景 ID 独立运行，模型 `gpt-6-luna` / `medium`，无需人工 sandbox 初始化。

| 场景 | 环境 | 测试耗时 | CLI 报告累计 tokens | 结果 |
|---|---|---:|---:|---|
| delivery.basic | controlled | 65.97 s | 无模型请求 | pass |
| recovery.waiting-merge-restart | controlled | 84.74 s | 无模型请求 | pass |
| delivery.local-codex | live | 约 102 s | 94,386 | pass |
| delivery.orca-codex | live | 203.69 s | 225,912 | pass |
| recovery.local-codex | live | 113.85 s | 62,972 | pass |
| recovery.orca-codex | live | 234.67 s | 325,426 | pass |

有效成功场景累计 **708,696 tokens**，包括 cached input；before/after-restart/final/cleanup 是同一累计值，不重复相加，不换算账单。各子目录保存独立 summary、scenario、实际冻结 policy 和筛选原生 usage；recovery 另存 identity.json 及重启前后 usage，不保存认证、私有 SQLite 或完整原生指令正文。

## 命令及证据来源

```powershell
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex'
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.orca-codex'
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.local-codex'
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-codex'
```

每次使用新的 `-out` 目录。有效证据来源位于 Windows 临时目录：

- controlled：`sl-l3-full-access-controlled-20261002-094021`。
- delivery.local-codex：`sl-l3-full-access-delivery-local-20261002-094246`。
- delivery.orca-codex：`sl-l3-full-access-delivery-orca-stable-20261002-104913`。
- recovery.local-codex：`sl-l3-full-access-recovery-local-20261002-094923`。
- recovery.orca-codex：`sl-l3-full-access-recovery-orca-push-20261002-100623`。

全部配置 host 读/写、network allow、escalation deny，冻结为 danger-full-access / approval_policy=never，资源契约为 `codex-policy-v1:0.159.2:host-write:network-allow:never`，无附加 write roots 或 Windows sandbox 设置。controlled 使用受控 app-server，不制造真实 usage。L3 使用真实 Git 传输与 loopback PR provider，不创建 GitHub PR；L2 的真实 GitHub PR 验收记录单独保留。

两条 recovery 的项目、数据库、执行、工作流、session、turn、工作区和 attempt 数在重启前后保持一致，usage 不变；Orca readiness/business 两个回合均 succeeded。合并原交付后后继任务就绪但未启动，清理本次服务、终端、worktree、仓库注册及 scratch。结束后未发现测试/helper 进程。

## 发现与修复

1. 全部 L3 改为显式完全访问。runner 向环境传入 Level，未来新增组也沿用，不用组名猜权限；共享断言核对正式冻结策略与资源，recovery 重启后再次核对。
2. 原 L3 Orca 脚本使用 loopback URL、dispatch 使用正式 remote，原生模型据此停止发布。脚本改用 origin，仅单次 push 通过 `-c url.insteadOf=...` 路由到 loopback；仓库级重写会破坏正式 remote 校验，因此不用仓库级配置。明确 PR head/base 含义并禁止向 main 推送。最终脚本的 recovery、delivery 均完成真实原生发布。
3. readiness 只投影 stdout 和手写 exit 文本不足以证明原生结果。fixture 明确保留完整工具 JSON；核验器仍要求原生命令、exit_code、nonce/hash 和绑定身份，不放宽证据。
4. 原输入目标短暂未就绪时，通过已有公开 recheck_environment 一次重查，保持原 Task/terminal/Dispatch；不重发旧业务输入。
5. Orca 长提示早期出现 bracketed-paste 预览，立即 Enter 可能先于后续字节。适配器等待原终端 draft 稳定至少一秒，完成一次 Enter 并记录提交后状态；不把 accepted/bytesWritten 当作已开始的证据。

原生失败未被跳过：一次契约冲突、一次无模型的仓库校验失败、一次准备证据超时、一次业务输入未核验、一次未启动的准备超时，均清理通过。旧 delivery 成功样本因发布脚本变化被重新验证替代。以上额外运行已取得的 usage 合计 547,340；加有效场景，本轮所有**已取得**报告合计 1,256,036。未取得 usage 的失败不当作零计费，也不推断账单。

最后一轮 delivery 的成功证据独立保存；未将失败运行的准备或局部成功拼成通过。准备指令/有界重查/提交稳定性修复只补证据协议和时序，未修改恢复身份/usage 断言或产品权限映射。

定向检查通过：

```text
go test ./cmd/test-feedback ./tests/e2e/internal/... -count=1
go test -tags=e2e ./tests/e2e/live ./tests/e2e/suites/... -run '^$' -count=1
git diff --check
```

未重复 L2 真实模型或 full Go 回归：L2 已通过的完全访问路径和产品映射保持原行为，保存于相邻 live-l2-full-access 目录。sandbox 初始化工具不是 L2/L3 前置条件，本轮未触发授权。
