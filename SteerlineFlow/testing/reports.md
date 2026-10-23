# 测试报告与登记

固定输出根、每级文件布局、临时 scratch 和仓库外并行目录见[流程路径清单](../paths.md)。本文件只维护报告内容与登记规则，具体运行的绝对路径和源码版本记录在专属验收中。

分级入口保留原 `summary.json`、JSONL 和退出码，并追加 `managed-<id>.json`。L1 对应一个整体 ID，E2E 对应实际所选场景，L4 Go 对应 `go.full`；定向子集和 build-only 不输出完整登记项通过。E2E 证据位于 `scenarios/<id>/scenario.json`，含实际断言、资源与清理事实。运行结果写任务专属 [验收记录](acceptance/README.md)，清单不维护实时结果。

## 格式

执行前冻结随机报告 ID、`started_at/definition/selection/definitions`、实际 `command/repository/branch` 及可选 `rerun_of`；执行后补 `finished_at/sequence/before_commit/after_commit/before_clean/after_clean/results/evidence`。repository 是 Git common dir。结果项为 `test_id/status/message/log`，status 为 passed/failed/incomplete，log 指向同目录原 summary。

`evidence` 记录同目录 `summary.json` 的原字节数和 SHA256。定义摘要对规范化单项 JSON 取 SHA256：固定按 `id,name,level,environment,command,check` 排序，非空 `note` 最后；UTF-8、无 BOM/缩进/末尾换行，使用 Go `json.Marshal(Definition)` 字符串转义（含 HTML 转义）。不散列原清单排版。

缺失、跳过、零匹配、超时、选择不完整或清理未核实不计通过；失败与补测分别保存，不拼接子集为整体通过。L2 `full-L2` 仅表示当前受控冒烟，历史 gate-only/full-L2 按其冻结定义解释。

## 登记与复验

```powershell
go run ./cmd/test-feedback -level=L1 -record-project <配置ID> -out <新的绝对目录>
go run ./cmd/test-feedback -level=L4 -suite=testing -env=controlled -scenario=testing.records -record-project <配置ID> -rerun-of 'testing.records=<原报告ID>' -out <新目录>
steerline tests register -project <配置ID或项目ID> -report-dir <报告绝对目录>
```

运行后自动登记失败不改变原测试退出码；离线保留原产物，用原报告补登。独立 register 有拒绝/未知项时非零退出。`registration.json` 追加原报告、请求号、登记/拒绝/未知状态及诊断；未知沿原请求对账，不重跑测试。

报告至多 128 KiB，summary 至多 512 KiB，整份 JSON/base64 命令至多 1 MiB；按项目与报告 ID 原子幂等登记。只读取目录内普通文件，越界链接或内容/摘要冲突拒绝。缺失/过大 summary 可保留结果并标证据不可用，不计复验通过；可按原摘要补齐。

复验仅认显式链最新、可比且完整覆盖原失败范围的通过报告，原失败保留。源码脏/未知时显著显示“源码未固定”，不能当固定 SHA 证明；候选当前通过要求原 Run 上干净同 commit。详情 API 将 producer sequence 作为字符串输出，原报告字节不变。

## 自动安排与候选证据

系统读取固定版本的 Plan、`SteerlineFlow/testing.md` 与 `SteerlineFlow/testing/test-list.json`，自动生成测试 ID 安排并存项目库。无需自动测试时 Plan 必须明确原因。无效输出或缺失定义阻断新候选；已有候选按原快照执行。原生成查询/停止使用正式 CLI，未知只核对原请求。

候选消费冻结提示中的安排和真实 worktree 文档；测试程序实际执行并输出报告，不能用模型 final 自述代替。运行内使用项目忽略的 `.steerrunner/artifacts/<唯一执行ID>/` 新目录，最后原子发布 managed 报告；默认系统临时目录不会自动成为 Run 产物。

Runner R2 公开收集/读取与 Git 观察按原候选/Run 核验字节长度和 SHA256，保存不可变副本。collect 原请求先持久化，未知只查询；过期候选不发起新 collect，原证据只作历史，当前性未知不显示当前通过。页面 `/projects/:id/tests` 只读；独立脚本历史不代替候选证明。协议细节见 [测试管理 Spec](../spec/test-management.spec.md)。
