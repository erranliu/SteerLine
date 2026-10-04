# Plan：候选观察界面

任务 ID：38；任务名称：候选观察界面。唯一执行依据：[Spec](candidate-console.spec.md)。硬前置：#37，一个独立实现 PR 交付。

1. 接 #37 的候选集合、配置/耗时/结果定位，复用原Task主行、会话与待决策组件，不新增实验权威或自动比较。
2. 接逐个选方案新增、精确候选反馈与不可撤销放弃；展示开始时5个检查、退出实验和真实停止状态，合并后不提供放弃。
3. 用五行列表与少量交错消息验证身份/阅读位置，用原请求与真实应用接线验证开始/反馈/放弃。不穷举五候选排列或配置组合。
4. 完成真实Windows输入/导航代表验收，保留历史与资源定位；向 #39交接真实操作入口与证据。

## 测试计划

规划新增或迁入顶层TestCC1–TestCC4，先复用现有console中性产品fixture及外层接线；测试名称和当前路径在实际实现时登记，零匹配不算通过。解耦改变路径时更新本Plan到唯一实际包。

| 级别/组或场景ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向Go：CC1、CC2、CC4 | Windows；五候选产品fixture，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-console/view -- ./internal/steerline/console -run '^TestCC[124]$' -count=1` | 展示/选择/放弃接通后 | 配置、身份、正文、退出实验/停止事实和历史正确；各用例非零命中、无fail/skip | view/summary与原始Go JSON |
| 定向Go：CC3及CC4真实接线 | 临时服务/SQLite和可控后端 | `go run ./cmd/test-feedback -out docs/validation/candidate-console/wiring -- ./internal/steerlineintegration -run '^TestCandidateConsoleWiring$' -count=1` | 默认客户端接通后 | 精确原请求/版本，5个串行检查，合并后拒绝放弃；UI不自行操纵进程或业务状态 | wiring/请求、回执、快照与测试JSON |
| 手工原生UI：CC5 | Windows真实终端，隔离配置与可控worker | `go run ./cmd/steerline console -config <隔离fixture配置绝对路径>`；实施记录实际命令 | Go代表用例通过后 | 中文组词/备注、候选选择/放弃、窄窗口/滚动和退出语义通过 | windows/操作记录、构建/配置身份及必要截图 |
| L1固定最小入口 | 隔离STEERLINE_HOME，无原生开关 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-console/l1` | 自查完成后一次 | 原UI/客户端/守卫代表实际命中且通过 | l1/summary与原始日志 |

fixture与真实接线仅补缺口，不增加全面视觉矩阵。输出目录使用本次新的空目录。Full/L2/L3不默认选择；后端实际实验/放弃证据由 #39负责，CC5不冒充真实模型验收。
