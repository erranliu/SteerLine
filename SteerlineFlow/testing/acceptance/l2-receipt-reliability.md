# L2 回执失败与生成请求未知：定位及修复验收

2026-10-31，普通开发，无 Task ID。用户要求按此前方案修复，优先定位 Windows 回执替换失败，不增加重试或超时，修完运行固定 L1/L2。[原失败及界面验收](runtime-environment-ui.md)。本轮保留此前所有工作区改动，不部署安装现场，不修改提示词、执行材料或 CI。

## 1. 已确认的事实与未确认范围

原始失败分别位于证据根 `C:\SteerlineDebug\wt-5e65-runtime-env-ui` 下：

- `test-results/L2/20261031-01`：自动材料变成 unknown，原请求 inspect 返回 request_not_found，最终 GET maintenance 超时。原临时现场已由当时正常清理删除，只有最后观察，不能追溯首次 submit 的真实错误。
- `test-results/L2/20261031-02`：start_task 的 HTTP 记录由临时文件替换为原 request ID 的 JSON 时，返回 Windows Access is denied。该错误可能发生于正常或拒绝回执保存，不能直接判断任务是否执行。原现场同样已清理，未保存占用者或系统调用阶段。
- 干净 cb0c888e 主干 L2 通过；本轮只增加 rename 阶段诊断、不改变替换行为的 L2 也通过。因而没有证明 Windows 原失败是稳定的业务回归，亦不能证明它已被修复。

Windows 现有实现已经使用共享删除快照、FileRenameInfoEx 和 40 次/10ms 的有界替换重试。本轮完全保留次数、间隔、ACL 与原子替换机制，不加入删除目标、直接覆盖、放宽权限或重发业务的路径。

## 2. 定位实验

1. `TestPrivateSnapshotConcurrentPublication`：8 个合法快照读者与 1000 次发布并发，直接调用单次 rename 原语，不经过重试。通过，未证明合法快照读者会锁死发布。
2. `TestPrivateSnapshotTargetExclusionIdentifiesPublishFailure`：明确用不共享删除的句柄持有目标，单次 rename 在发布目标 FileRenameInfoEx 阶段失败（本机为 sharing violation）；旧目标和源保留。释放句柄后，用同一个源文件单次发布成功，私有 ACL 保持。
3. 上述实验确认句柄排斥的机制和诊断阶段，但错误码与旧现场 Access denied 不完全相同，不能认定它就是旧现场的占用者或根因。原 Windows Access denied 仍未定因；若再发生，新错误将区分打开源、检查源、发布目标，并保留原 Win32 错误链。

先验证替换路径再实施下面的独立正确性修复；不以成功的 L2 或原请求恢复取代对 Windows 根因的证明。

## 3. 实际修复

| 位置 | 改动和边界 |
| --- | --- |
| httpfs Windows rename | 只增加三处系统调用阶段上下文，保留 errors.Is/LinkError 和原替换实现；未增加任何重试 |
| 受控 Runner 状态加载 | 仅文件不存在才视为新状态；目录/读取错误、坏 JSON 明确失败，不再忽略错误后写回空状态。原 lock 和写入流程不变 |
| L2 MainControl / 原 tmControl | 提取既有提交错误判别，所有调用只 SubmitControl 一次；仅 request_receipt_persist_failed 进入原 request ID 的只读 WaitControl，保留原提交错误。明确拒绝或未知仍按原回执处理，不改期望状态。原 6 分钟与 tmControl 30 秒期限均保持 |
| 测试安排生成 | 在原 Generation JSON 中保存一次 SubmissionDiagnostic，原 Diagnostic 仍表示当前观察；SQLite CAS 禁止改写已保存的首次提交错误，无新增表/迁移/旧版分支。材料查询在原 Run 尚未核实时并列原提交与当前观察，原 Run 核实后恢复当前诊断，不隐藏身份冲突等新错误、不重发未知生成 |

新增字段只是原活动的错误事实，不参与资格、生命周期或恢复授权。未知提交仍只观察原请求，配置/环境 ready 和三轮验收规则均未修改。

## 4. 验收安排及结果

基线源码：`cb0c888e7ed6ccea3f9d5c9e2e6427cc3884d0a6` 上本聊天未提交工作树，包含前一轮界面实现。实际输入及构建身份见各 summary/frozen-binaries；不能导出为干净 SHA 的测试通过历史。输出均在上述独立证据根，每次使用新目录。

| ID / 方式 | 命令或方法、通过条件 | 结果与原始证据 |
| --- | --- | --- |
| LRR-A01，Windows 定向 | 单次并发发布和明确目标排斥两类实验；合法读者不阻塞，目标排斥保留旧完整文件，释放后同源发布；原 Win32 错误链仍可匹配 | 通过；`diagnostics/receipt-probe-20261031-01` 及 `diagnostics/receipt-fix-owner-20261031-03` |
| LRR-A02，driver 定向 | 回执文件保存失败只查原 ID，Submit 次数=1；rejected/indeterminate 原样返回，查询超时保留失败；已知 stale_revision 不进入未知恢复 | 通过；同一 owner-03 的 summary/Go JSONL；原 tmControl 提交错误证据文件保持 |
| LRR-A03，controlled 定向 | 不存在、不可读目录、坏 JSON 三例，只有不存在可初始化；错误不删除原文件 | 通过；同一 owner-03 |
| LRR-A04，generation/SQLite/app 定向 | 首次提交未知后反复 inspect 缺失：原错误保留、当前错误可见、提交次数=1；原 Run 核实后不显示过期错误；关闭重开数据库保留两份诊断、禁止改写原提交错误；原 app unknown 不自动重试 | 通过；同一 owner-03 |
| LRR-A05，固定 L1 | `go run ./cmd/test-feedback -level=L1 -out <本次L1目录>`；固定集合非零命中，完整通过 | 通过；`test-results/L1/receipt-fix-20261031-01/summary.json` |
| LRR-A06，固定 L2 | `go run ./cmd/test-feedback -level=L2 -out <本次L2目录>`；完整 smoke.delivery、原请求唯一性、PR/交付和资源清理核实 | 通过；`test-results/L2/receipt-fix-20261031-01/summary.json`；TestDeliverySmoke 102.89 秒，cleanup_complete=true |

定向实际命令：

```powershell
go run ./cmd/test-feedback -out <owner目录> -- ./internal/product/httpfs ./internal/product/httpapi ./internal/testmanagement ./internal/storage/sqlite ./internal/app ./tests/e2e/internal/controlled ./tests/e2e/internal/driver -run 'Test(PrivateSnapshot|RequestRecordReaderAllowsOutcomeReplacement|TMGenerationUnknownPreservesSubmissionCause|GQDurableBatchAndSubmissionBoundary|PMSGenerationUnknownDoesNotOfferAutomaticRetry|TMControlReceiptFailure|RunnerFixtureReadFailure)' -count=1
```

保留中间失败：owner-02 的新用例发现原 state 可能仍为 unknown 但 Run 已核实；已将诊断展示条件收紧为“原 Run 尚未核实”，owner-03 通过。owner-01 是更早的局部实现结果，不替代最终 owner-03。

诊断专用 L2：`test-results/L2/receipt-diagnosis-20261031-01`，替换行为未改，仅新增阶段上下文，TestDeliverySmoke 105.03 秒通过、cleanup_complete=true。它不能作为随后修复的 LRR-A06 通过证据。

## 5. 收口边界

原 Windows Access denied 占用者/权限条件仍未定位，不宣称已根治。已修复的是可确认的夹具读错处理、首次提交诊断丢失和 L2 原请求对账缺口；永久替换失败仍上报，原请求无法查实时仍失败，不以重试或放宽验收掩盖问题。固定 L1/L2 已在修复后串行通过；不将通过结果当成旧 Windows 失败根因已消除的证明。


2026-10-31 收口：LRR-A01–A06 按上述范围通过。定向新增用例 03 通过，L1 preparation=0.4s/execution=10.7s，L2 原任务交付及清理通过。没有修改原 40 次/10ms 替换重试、全流程超时、模型提示词或安装现场；旧两次失败与本轮诊断/修复结果分别保留。Windows 原始 Access denied 根因项仍未闭合，必须有新现场句柄/权限证据才能宣称根治，不能从本次 L2 passed 倒推原因。

## 6. 用户要求的有界复现（2026-10-31）

用户另行要求尝试复现；本轮只运行诊断实验，不再修改生产或已入库测试代码。预先限定旧二进制场景最多三轮，出现非预期失败即停止，不以重跑覆盖失败。所有输出位于同一证据根的 `diagnostics/`，不作为新的正式 L2 登记通过历史。

### 6.1 复用原失败的冻结二进制

复用 `test-results/L2/20261031-02/build/` 中的主程序、受控 helper 和原场景测试程序；执行前逐个核对原 frozen-binaries 的 SHA256，一致后才启动。主程序 `1fc8ad56ad3c697e91744f67dc5dd6aeac885a03cc76801d62d4ea4f3c6e405d`、helper `4848a3264c841f90244b5ea740cb1826e8e490c3799a9e058aedff3da2a195c0`、场景程序 `a858efa7983443a4ef7b3a2fe861d802da600fd49bec4443cbd32e60cc9ab226`。因此本轮场景没有使用随后新增的回执恢复逻辑。

每轮独立 Output/Scratch/home/仓库，沿原受控 `TestDeliverySmoke` 和原 6 分钟测试期限；命令为原 `scenario-1.test.exe -test.v -test.count=1 -test.parallel=1 -test.timeout=6m -test.run=^TestDeliverySmoke$`，输入信封分别保存为 run-input.json。第一轮不并发施压；后两轮与下面的有限文件读写实验重叠。没有人为持有禁止删除的句柄，也没有增加重试或期限。

| 轮次 | 退出码 / 完整场景 / 清理 | 墙钟时间 | 原始证据 |
| --- | --- | --- | --- |
| 旧二进制 1 | 0 / passed=true / cleanup_complete=true | 112.99 秒 | `diagnostics/receipt-repro-20261031-01/frozen-01/scenario.log` 及 `scenarios/smoke.delivery/scenario.json` |
| 旧二进制 2 | 0 / passed=true / cleanup_complete=true | 118.11 秒 | 同根 `frozen-02/` |
| 旧二进制 3 | 0 / passed=true / cleanup_complete=true | 121.54 秒 | 同根 `frozen-03/` |

汇总 `diagnostics/receipt-repro-20261031-01/results.json`；复现驱动 `diagnostics/receipt-reproduce.py`。驱动准备了只在观察到 rename 失败时采集目标属性、ACL 和 Restart Manager 占用进程的只读监测；三轮没有触发该错误，因而没有捕获到占用者，不能凭空推定是杀毒软件或某个进程。场景清理后仅移除了三个已核实为空的 scratch 根，证据全部保留。

### 6.2 原回执 owner 与单次原语的并发压力

临时 `TestReceiptReproductionConcurrentLifecycle` 通过仓库外 Go overlay 加入原 httpapi 测试包：每轮 1000 个不同请求经过真实 owner.Register，再更新原回执四次，8 个合法快照读者同时读取，检查请求身份及最终文件数。三轮全部通过，分别 58.83 / 55.60 / 65.42 秒；合计 3000 次注册、12000 次回执替换，无向上抛出的替换失败。owner 路径保持原已有替换重试，未增加次数。

原 `TestPrivateSnapshotConcurrentPublication` 另执行三轮：合计 3000 次直接单次 rename，不经过替换重试，每轮 8 个合法快照读者；三轮全部通过，分别 1.27 / 1.37 / 1.90 秒。此项未出现任何需要靠重试消除的失败。

实际命令：

```powershell
go run ./cmd/test-feedback -out <本次pressure目录> -- -overlay=<diagnostics/receipt-reproduction-overlay.json> -p=1 ./internal/product/httpapi ./internal/product/httpfs -run 'Test(ReceiptReproductionConcurrentLifecycle|PrivateSnapshotConcurrentPublication)$' -count=3
```

报告 `diagnostics/receipt-repro-pressure-20261031-01/summary.json` 及 Go JSONL/stderr，工具墙钟 191.603 秒、退出码 0。临时用例为 `diagnostics/receipt_lifecycle_reproduction_windows_test.go`，overlay 为 `diagnostics/receipt-reproduction-overlay.json`；没有加入仓库或改变原测试集。驱动/临时用例/overlay SHA256 分别为 `052ef9648235530704ffe67ec4451e7fb9a03fdcf9407759d8503383c7975dfc`、`c942fbf7b196fbe9724c5dc13a591dfc18bf5e73de34d1c8df9f8d4b59376077`、`588672c4eafedd7bae169995c17f249204ff1c792b2d4cce0dc54e46df7cfe62`。

结论：本轮没有复现原 Windows Access denied，也没有自然出现另一种回执替换错误。有限轮次、有限负载不能证明问题已消失；原现场的确切持有者/权限条件仍未知，不提升为“已根治”或“永远不可复现”。本轮不再无界重复完整场景。
