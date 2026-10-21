# 可靠性交付摘要实施计划

范围：当前 Run/原命令的只读摘要、当前操作说明收敛、已有异步对账复核。依据 Runner `docs/reliable-delivery.md` 第 6–8 节。基线 SteerLine `36d4f596`，Runner `8261310`。不修改调用方调度、原命令投递、数据库权威、权限、默认 workflow 或安装环境。

1. 固定两仓基线与原能力，确认 Bridge.Step/deliver 已存在，parser 已默认关闭。
2. 从同一公开 Run 快照生成有界观察值，命令回执独立列出；经原 Project → application → query DTO → 候选详情展示，不读取 Runner 私有库、不新增请求。
3. 保留原 session/turn final 关联、原 revision、controls 和缺口；身份不符只显示未核实；候选停止参与后不显示可新发输入。仅原始 PR/停止事实展示，不推导总完成状态。
4. 更新当前文档，不改写历史验收。对账无具体重复则不新增辅助层；验证长期 pending/unknown 的单次 Step 返回后仍可处理 stop。
5. PR 摘要沿用原 `checked_at` 显示缓存观察时间，缺失不补造；查询失败保留原时间及错误，刷新页面不新增 provider 查询。

## 测试计划

结果单独登记在[本任务验收](../acceptance/reliable-delivery-simplification.md)。

| 验收 | 级别与命令 | 环境与时点 | 通过条件与证据 |
| --- | --- | --- | --- |
| RD-D1 文档 | 差异、链接和当前换代语义检查 | 两仓工作树，文档修改后 | 当前操作指引无强制全量 resume/先全停；历史事实保留；验收文档登记 |
| RD-D2 摘要 | `go test ./internal/app ./internal/taskrunner -run 'TestRunnerDelivery' -count=1 -timeout=2m`；web `npm test -- src/features/runs/delivery-summary.test.tsx src/features/runs/runs.test.tsx`；`npm run build` | L1，隔离夹具，代码完成后 | accepted、等待、unknown、pr_ready、内容成功/结算分离、外来身份/陈旧 final 及消费者不反向修改；原 PR 时间/缺失/失败沿用缓存，时间副本不反向修改；组件无 I/O |
| RD-D3 对账 | `go test ./internal/runnerbridge -count=1 -timeout=2m` | L1，受控 CLI/store，核查后 | 丢响应按原 ID；pending/unknown 不引入阻塞等待，stop 后续可处理；不重复输入 |
| RD-BOUNDARY 边界 | `go test ./internal/app -run 'TestCM71ProductDependencyBoundary|TestHTTPConsumerBoundary' -count=1 -timeout=2m` | L1，源码闭包，映射后 | API/HTTP 不取得原执行 owner；既有失败须按原 HEAD 复现并单独登记，不放宽守卫 |

不选择全量、L2–L4 或原生开发交付：本仓变化只有展示、纯投影和文档，无新原生副作用。Runner 的审计提示词 V1 另做隔离能力试验；未完成正式 Run/接管资格前不改变默认行为。
