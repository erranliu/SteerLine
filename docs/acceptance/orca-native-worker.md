# 本机 Orca 原生通用 worker 验收

只归属任务 #86。依据 [Spec](../spec/orca-native-worker.spec.md) / [Plan](../spec/orca-native-worker.plan.md)，由 [execution-map](../roadmap/execution-map.md) 汇总执行与验收；原始日志放独立临时证据目录。

| ID | 方法 / 步骤 | 通过条件 | 时点与阻塞范围 | 状态 / 证据 |
| --- | --- | --- | --- | --- |
| ONW-A1 | 自动：生产依赖边界、通用配置两 agent、旧配置拒绝、本地策略定向 | Orca 路径无特定 agent 实现；配置值决定原生选择；旧格式在副作用前拒绝，本地 Codex 保持 | 实施后；阻塞本任务交付 | 待验收；新增入口按 Plan 落实 |
| ONW-A2 | 自动：受控两轮原生 worker-start、输入/反馈/输出/结算、身份变化/未知回执/恢复/资源 | 同一原 worker，明确终态；未知不重发，旧结果不串线；无专属探针/relay | 实施后；阻塞本任务交付 | 待验收 |
| ONW-A3 | 自动：隔离 debug runtime、真实 Orca原生 launcher、工作区读写操作、反馈与原生完成、输出、精确清理 | 权限由 Orca 原生设置持有；模型真实执行；各身份与结果可核对；资源停止清理完成 | 驱动/场景落实后；能力或环境缺失为真实阻塞 | 待验收；`orca.native-worker` 入口待新增，当前未运行 |
| ONW-M1 | 人工：新安装构建启动，确认权限来源提示、待处理反馈和 Orca 原生界面 | 可辨识权限由 Orca 管理，无 Codex专属权限配置误导 | 交付后集中；用户参与，窗口待安排；不阻塞无关研发 | 待验收 |

2026-10-09 规划：用户确认先用宽松权限跑通，agent只为配置字段。已读取当前 Orca 原生启动代码，确认其统一 Agent Permissions / YOLO及默认 launcher 参数映射存在；这是能力定位，尚非本任务产品自动通过证据。C:/SteerlineRuntime 的历史102只作设计输入，不修改其任务、配置或冻结执行。
