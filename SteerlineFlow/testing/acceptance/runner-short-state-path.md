# Runner 短状态路径验收

归属：[实现 Plan](../../plans/runner-short-state-path.plan.md)。本任务只调整 SteerLine 的 Runner 目录，不修改 Runner 的路径限制。

## 自动验收

| ID | 要求 | 验证入口 | 状态 / 证据 |
| --- | --- | --- | --- |
| RSP-A01 | 安装/深调试 home 不再增长 Runner socket 路径；真实 OS socket 可创建 | TestRunnerSocketWorksForDeepApplicationHome | 通过：home/run-dir 长 245、socket 98 字节，真实 Windows AF_UNIX bind 成功；最终 `.evidence/runner-short-path/paths/go-test.jsonl` |
| RSP-A02 | 不同 home/项目独立；查询无创建；目录别名拒绝；删除不影响兄弟目录 | runnerstatepath 定向 | 通过：`.evidence/runner-short-path/paths/summary.json`，3/3、0 skip，包含无提权 junction 拒绝、私有 ACL 及兄弟 sentinel 保持 |
| RSP-A03 | 启动、候选绑定、检查、查询、停止证明及测试清理共用短目录 | app 定向、provider 路由、L2 controlled | 通过：首轮定向（9 pass、1 skip，跳过项已由 paths 复验）；`.evidence/runner-short-path/gate-r2/summary.json` passed/cleanup_complete=true，smoke.delivery 105.03 秒；新命名空间已清理 |
| RSP-A04 | 深目录调试包双项目及 reset 仍隔离，活动原资源阻止重置 | 新包 TestDebugReleaseWebProjects | 通过：`.evidence/runner-short-path/debug-r3-host.log`，91.27 秒；深 home 两项目独立启动、活动时 reset 拒绝、停止后两项目 reset 通过，清理后无本包进程或短目录残留 |

## 现场与边界

原安装现场 `101/C1`：Run `e9f565d31ef034f70e8652da2c78d286`，旧 state-dir `C:\SteerlineInstall\state\SteerlineTest-b55e8cc42ae8\runner`；17:00:31（北京时间）prepare_session 被拒绝。只读取证，未修改、移动、重置或重发。

旧布局不兼容、不自动迁移；新代码不会把原 Run 冒充短目录中的新 Run。源码测试通过不代表安装环境已经更新或 C1 已恢复。无人工验收项，不运行真实业务模型或创建业务测试 PR。

## 保留的失败记录

- `.evidence/runner-short-path/gate`：首次 smoke 在异步 stop 回执前断言两个请求而失败，停止义务已保存、原 Run fixture 随后记录 stopped，但整批失败且 cleanup_complete=false，原目录保留。已改为等待公开 stopped 后检查最终请求数，不删减断言；新隔离 gate-r2 通过。
- `.evidence/runner-short-path/debug-two-projects.log`：直接在工具进程启动宿主，被 kill-on-close Job 保护拒绝，未创建业务候选；改用已有独立进程入口。
- `.evidence/runner-short-path/debug-r2-host.log`：测试调度过早，生成器尚未完成，缺配置文件即退出；未进入包运行。等待同一个尚未运行的新包 manifest ready 后，以独立进程执行，结果另记 debug-r3-host.log。

## 最终验证与构建

- 新包：`C:/SteerlineDebug/short-runner-path-r2/dev-20261016-173030-34703795-dirty`；原有默认 live 生成后，测试显式冻结 simulate。生成器从本次独立构建 EXE 固定 Runner，没有使用安装版 EXE、数据库、配置或认证。
- 该 Runner 来源 `072688f145c8917bceb08223aa019b9560d366f6-dirty`，SHA256 `08d57c2ab1f26f532b6b81ca17b23e6367c4b4fb5b30174d20be6e803b4f08b8`，见 `.evidence/runner-short-path/tools/steerrunner.exe.build.json`。未修改 Runner 源码或放宽限制；模拟测试不声称验证其业务执行。
- `entry/summary.json`：固定 L1 选择器定向通过，新三个路径用例已登记 `quick-owner-runner-state-path`。
- `boundary/summary.json`：`TestTM08AppHostBoundary`、`TestCM71ProductDependencyBoundary` 通过，调用方依赖边界不变。
- `git diff --check` 通过。未运行 Full、完整真实 L2 或真实 PR；以上通过仅覆盖本次路径布局、接线和资源定位。

## 提交前主干同步复验

合入 `origin/main` 的 `6f275525`（含环境检查契约修复及旧执行域清理），仅 `SteerlineFlow/testing.md` 同位置新增段落冲突，保留双方内容。代码合并提交 `6ac67290` 上复验：

- `.evidence/runner-short-path/pr-targeted/summary.json`：7 包、74 项通过、0 失败，78.514 秒；`TestEnvironmentCheckPublishedContract` 因未提供显式发布 Runner 配置而按设计 skip，不计实际 EXE 契约通过。包含短目录的三个用例、caller/CLI、环境检查和 reset。
- `.evidence/runner-short-path/gate-pr/summary.json`：`smoke.delivery` 86.04 秒，passed、cleanup_complete=true、gate-only；使用独立进程及新夹具，未覆盖前一轮证据。
- 主干同步后的代码未新增安装环境操作。最终文档更新不改变上述受测代码。
