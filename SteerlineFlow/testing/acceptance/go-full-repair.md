# Go Full 失败续修验收

日期：2026-11-02。用户要求修复全部 Full 失败、不放宽断言、独立审核后更新 PR #423。范围见 [Spec 续修](../../spec/test-content-alignment.spec.md#续修go-full-全部失败2026-11-02) 与 [Plan](../../plans/test-content-alignment.plan.md#pr-423-续修安排)。原始失败及隔离基线证据仍在[初轮验收](test-content-alignment.md)，不覆盖。

基线为 `cfbb33f24e1bef3bbd97c5fd5379b6d232b81fdc`，工作树干净。

| 项目 | 通过要求 | 状态 |
| --- | --- | --- |
| GFR-A1 | 原架构矩阵、扫描范围及所有边界断言不变，消除不合法生产依赖 | 定向通过，独立审核通过 |
| GFR-A2 | 原配置保存/取消/并发/校验断言通过，公开 schema 不放宽 | 整包通过，独立审核通过 |
| GFR-A3 | 原快照并发、ACL/排斥/链接断言及同句柄确定性回归通过 | Windows 压力及 Unix 新回归各10轮通过，独立审核通过 |
| GFR-A4 | 最终 L1/L2、受影响 owner/L3、Go Full 实际完整通过，0测试skip | 全部通过，L2/L3 清理完整 |
| GFR-A5 | 提示词输出/黄金样本正文差异与字节核对、独立审核及原 PR 更新 | 审核通过，同分支提交并更新 PR #423 |

每次实际运行使用全新临时输出目录，保存源码、命令、原始报告/JSONL及清理事实；失败与修正后结果分开。最终结果、根因、正文差异、审核结论追加于此，不将早期未验写成通过。

## 根因与修复

1. **架构依赖**：测试入口模型错误放在项目维护 owner，项目维护直接依赖 prompts，runnerstate 依赖环境业务模型；环境 model 的外部测试包导回同目录 model，触发现有扫描的自依赖约束。原扫描矩阵没有问题。ProjectEntry 及原解析测试原样移到 testmanagement/model，无旧 alias；App 读取固定协议正文并作为必需事实传给项目维护，缺失/正文不符仍阻断，只容忍 CRLF；runnerstate 自有四字段准入证据，App 完整复制，SQLite 在原事务中逐字段核对。环境 model 三份测试改同包保留全部用例/断言。`internal/testkit/boundary` 整目录 diff 为空。
2. **配置夹具**：原正向 settings 缺现行 schema 必需的 `access_mode`，原 `github_auth` 已不属于公开 run schema。补合法前置，后者改为明确拒绝的负例；字段保留正例改用合法 model/reasoning_effort，原标记断言保留并加完整 settings 字节相等断言。独立审核发现原负例浅复制共用 Profiles，可能由前一个 orca 变异或缺字段提前拒绝；已改从合法基线独立深解码，新 executable 还必须命中 `program_read_only`。公开 schema、生产校验器没有修改。
3. **Windows 快照竞争**：先按名字查权限、再打开文件会跨越不同版本；改为先取得允许共享删除/替换的句柄，再对同一句柄校验普通文件/当前用户 owner/受保护单 ACE 私有 DACL。原 1000次发布/8读者测试随后仍复现 `CreateFile` 不存在，定位记录排除了后续读取/校验重开路径。增加 SID 级私有命名内核 mutex，仅协调打开与原子发布两个短步骤，跨进程/路径别名统一；读句柄返回后即释放锁，读取不阻塞后续发布。现有对象 ACL 也严格校验；不吞错、不新增文件系统重试，不改原 RenameInfoEx/既有排斥重试，也无删除/复制回退或磁盘旁置文件。
4. **跨平台同句柄边界**：独立审核发现 Unix FIFO 在打开时阻塞及已解绑 inode 的 Nlink=0 被误拒绝。保留 O_NOFOLLOW 并加 O_NONBLOCK，之后同句柄 Stat 仍拒绝非普通文件；已打开快照只拒绝 Nlink>1，路径级 VerifyPrivateFile 仍严格 Nlink=1。新增确定性回归覆盖 FIFO 无 writer、多硬链接拒绝、路径替换/权限变更/删除后原句柄内容不变。

Windows 同步以[官方 mutex 线程所有权约束](https://learn.microsoft.com/en-us/windows/win32/api/synchapi/nf-synchapi-createmutexw)使用 LockOSThread/ReleaseMutex 配对，等待失败和释放失败都返回错误。POSIX rename 的打开句柄语义见[官方说明](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information)。两份只读独立审核分别覆盖全部实现/提示词，以及 Windows/Unix 快照线程、句柄、安全与测试边界；确认问题修正后均无剩余阻塞发现。

## 提示词正文与独立审核

| 范围 | Git 规范化正文 UTF-8 字节前→后 | 实际正文差异 / 原因 |
| --- | --- | --- |
| 固定 Markdown 10 文件 | 7,279→7,279 | 无，资产 diff 为空 |
| 黄金样本 7 文件 | 2,588→2,588 | 无，保留 CRLF 样本逐字节相同；未更新任何黄金样本 |
| prompts.go 组装源码 | 4,606→4,558 | 类型改为正确 owner、删除旧导入；渲染规则/行为正文零差异 |
| prompts_test.go 源码 | 5,696→5,707 | 使用搬迁后的模型构造原输入；原黄金断言不变 |
| ProjectEntry 实现 / 原测试 | 2,574→2,574 / 1,574→1,574 | 原样搬迁，无解析或路径规则差异 |

业务原因是纠正已被原边界测试拒绝的依赖归属；App 仍从固定文件取必需协议，业务代码没有 inline 行为规则或兜底。实际渲染正文以原黄金样本完整相等及 prompts owner/L1 为证；固定正文与黄金样本 diff 均为空。独立审核明确批准上述接线及测试变化，不以更新黄金样本代替批准。

## 实际运行

源码为 `cfbb33f24e1bef3bbd97c5fd5379b6d232b81fdc` 加本次工作树，dirty=true。以下 `test-feedback` 叶目录均位于 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`；原始 JSONL、源码身份、实际选择及报告分别保留。后续审核仅补强 config 负例及 Unix 专属分支，L2 的 Windows 业务实现未变；完整 Full 启动后没有继续修改实现。

| 命令 / 范围 | 实际结果 | 输出叶目录 |
| --- | --- | --- |
| `-- ./internal/testkit/boundary ./internal/environmentadaptation/model ./internal/projectmaintenance/... ./internal/testmanagement/model ./internal/prompts -count=1` | passed，11.671s；原边界矩阵未变 | `20261102T035511.511883600Z` |
| `-- ./internal/storage/sqlite ./web/server/executionconfig ./internal/product/httpfs ./internal/runnerclient ./internal/runnerbridge ./internal/taskrunner -count=1` | passed，53.275s；快照后续仍压力复现并继续修复，此结果不代替最终快照 | `20261102T035604.305131300Z` |
| app 三项：提示词固定输入、材料新鲜度/版本、环境准入/模型分组 | passed，100.783s | `20261102T040118.025458000Z` |
| 原并发发布 + 跨进程普通/extended别名 + 同句柄权限/替换回归，`-count=10` | passed，30终态/0skip，114.107s；每轮原发布和跨进程各1000次、各8读者 | `20261102T041744.146216000Z` |
| 全 executionconfig（独立审核补强负例后）`-count=1` | passed，4.531s | `20261102T042013.114466300Z` |
| `-level=L1` | passed/complete，准备0.340s、执行26.595s | `20261102T041850.410087800Z` |
| 独立审核修正后的最终 `-level=L1` | passed/complete，120终态/0skip，准备0.308s、执行15.472s | `20261102T042944.667246800Z` |
| 独立同用户 `scripts/test-l2.ps1` | passed，准备15.765s、执行117.744s，cleanup_complete=true | `20261102T041856.118650800Z` |
| 正式 `go.http` | passed/complete，3包104终态/0skip，32.327s | `20261102T042200.705469100Z` |
| 正式 `go.storage` | passed/complete，2包82终态/0skip，65.885s | `20261102T042205.869885000Z` |
| L3 `testing.project-environment-readiness,testing.material-generation` | 两项 passed/0skip，cleanup_complete=true；准备12.256s、执行358.325s | `C:/Users/lmas1/AppData/Local/Temp/steerline-go-full-repair-l3-20261102-01/L3/` |
| 最终普通 Go Full `-level=L4 -suite=go` | passed/complete、退出0、managed passed；71包、1431测试终态、0失败/0实际测试skip；准备17.682s、执行1321.463s | `20261102T042055.512033700Z` |

全部 E2E `go test -tags=e2e ./tests/e2e/... -run '^$'` 编译通过，不计场景通过。Linux/amd64 的 httpfs 整包（含新测试）交叉编译通过。现有 WSL2 Linux 6.6.87.2 上实际运行两项 Unix 新回归各10轮，20终态全部 PASS/0skip；使用 Linux 临时目录，无需安装软件或启动 Docker 引擎。二进制与实际输出为 `C:/Users/lmas1/AppData/Local/Temp/steerline-go-full-repair-httpfs-linux-20261102.test`、同目录 `steerline-go-full-repair-httpfs-linux-20261102-02.log`，不泛化为 Linux 全量通过。

### 保留的诊断失败

- `20261102T035348.594319700Z`：首次修复仍遗漏外部测试自包 alias，配置保留正例仍有旧 github_auth。随后纠正归属引用与合法 schema 夹具，原 schema 不改。
- `20261102T035654.585333200Z`：只做同句柄权限验证后原并发10轮失败2轮。`20261102T035926.984558800Z` 的5轮偶然通过不认定修复；`20261102T040103.705709500Z` 的30轮再失败1轮，8读者均在 `open discovery snapshot` 的 CreateFile 返回不存在，随后才增加短 namespace 同步。
- `20261102T040858.748833600Z` 的 httpfs 全包、`20261102T041059.486177600Z` 的三项10轮均通过；独立审核指出文本路径 hash 不能统一别名，改 SID mutex 并新增普通/extended 路径组合后，最终10轮见上表。
- 首次 Linux 直接运行参数被 PowerShell 拆分，Go 二进制明确拒绝 `-test`，未执行测试。保留 `steerline-go-full-repair-httpfs-linux-20261102.log`；完整引用各参数后另存 `-02.log`，不覆盖。

## 最终核对与交付

Full 从 `2026-11-02T04:20:55.8480055Z` 到 `04:43:15.4657501Z`，原 summary 的 passed/complete 均 true，退出0，`managed-go.full.json` 的唯一选择为 go.full、status=passed。无测试文件包15个按 `packages_without_tests` 单列：ProjectEntry 原测试搬到正确 owner 后，原 projectmaintenance/model 变为编译项；用例未删除。Full 覆盖了全部环境/存储/HTTP owner 的完整普通 Go 测试，不用定向通过代替全量。

对照初轮 `20261102T025454.875678400Z`：原13条失败终态逐项均为 pass；将 ProjectEntry 的包名迁移映射后，原1427条终态没有缺失。当前1431条比初轮多出的4条为：初轮 Full 后已加入的 GOFLAGS 拒绝反例、本次固定协议必需回归，以及两项 Windows 快照回归。Unix 两项另在 Linux 实跑，不混入 Windows Full 计数。

独立审核对全部实现、提示词实际正文及测试保护条件给出通过结论；专门快照审核确认路径别名、FIFO 及已解绑 inode 的发现均修正。最终只更新验收/导航及提交元数据，不再修改实现。固定正文与黄金样本零差异，原边界目录仍 diff 为空，原配置 schema 未改；不放宽断言，不忽略读错误。链接和 diff 检查在最终文档更新后复核，同分支提交/推送并更新 [PR #423](https://github.com/erranliu/SteerLine/pull/423)，初轮 Full failed 仍按原运行保留。

本次未执行真实模型、原生 Runner result 专项、远端或安装变更；这些不属于普通 Go Full。没有合并 PR。
