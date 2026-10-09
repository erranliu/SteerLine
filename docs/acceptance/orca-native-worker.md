# 本机 Orca 原生通用 worker 验收

只归属任务 #86。依据 [Spec](../spec/orca-native-worker.spec.md) / [Plan](../spec/orca-native-worker.plan.md)，由 [execution-map](../roadmap/execution-map.md) 汇总执行与验收；原始日志放独立临时证据目录。

| ID | 方法 / 步骤 | 通过条件 | 时点与阻塞范围 | 状态 / 证据 |
| --- | --- | --- | --- | --- |
| ONW-A1 | 自动：生产依赖边界、通用配置两 agent、旧配置拒绝、本地策略定向 | Orca 路径无特定 agent 实现；配置值决定原生选择；旧格式在副作用前拒绝，本地 Codex 保持 | 实施后；阻塞本任务交付 | 待验收；新增入口按 Plan 落实 |
| ONW-A2 | 自动：受控两轮原生 worker-start、输入/反馈/输出/结算、身份变化/未知回执/恢复/资源；不支持的 structured 类型与部分效果；worker-stop/release 资格及拒绝/未知 | 同一原 worker，明确终态；未知不重发，旧结果不串线；不支持类型阻断后续，首轮输入/效果如实保留；无专属探针/relay，清理有原生证明 | 实施后；阻塞本任务交付 | 待验收 |
| ONW-A3 | 自动：隔离 debug runtime、真实 Orca原生 launcher、工作区读写操作、两轮原会话反馈与原生完成、输出、精确清理；环境先准备 Orca Terminal + YOLO | 权限由 Orca 原生设置持有；模型真实执行；各身份与结果可核对；资源停止清理完成 | 驱动/场景落实后；Terminal模式与权限准备是前置，缺失为真实阻塞，不能用M1后置豁免 | 待验收；`orca.native-worker` 入口待新增，当前未运行 |
| ONW-M1 | 人工：新安装构建启动，确认权限来源提示、待处理反馈和 Orca 原生界面 | 可辨识权限由 Orca 管理，无 Codex专属权限配置误导 | 交付后集中；用户参与，窗口待安排；不阻塞无关研发 | 待验收 |

2026-10-09 规划：用户确认先用宽松权限跑通，agent只为配置字段。已读取当前 Orca 原生启动代码，确认其统一 Agent Permissions / YOLO及默认 launcher 参数映射存在；这是能力定位，尚非本任务产品自动通过证据。C:/SteerlineRuntime 的历史102只作设计输入，不修改其任务、配置或冻结执行。

本轮仅调整规划、未启动实现。独立只读复审 `/root/review_orca_plan` 确认安装版 Orca 的 structured worker 无已核实的同会话续轮入口；已将本轮范围明确为 Terminal模式，并补齐首次参数/单次启动、worker资源清理、测试包命令与清单顺序。此复审不构成 ONW-A1–A3 已验收。

## 本轮文档验证

2026-10-09：安装 Orca CLI 版本 `1.4.209`；其 `out/shared/tui-agent-permissions.js` SHA-256 为 `af431d7d001695dee57537308ded5d0bb3c2630aee2779515450e9d7572411c4`。原生 launcher 权限设置、worker-start 参数及 structured 续轮限制按该安装版本只读定位，不宣称其他版本支持。

独立复审最终通过，无新的实质文档问题；组合启动后的首轮输入/效果按事实保留，不再承诺不支持类型一定零执行。`python scripts/roadmap_task_id.py check` 通过（86身份文件、11草稿任务行、5正式任务行）；`python scripts/check_doc_links.py` 通过（274文件、1764本地链接、零错误）；`git diff --check` 通过。本轮未运行产品自动验收或模型；ONW-A1–A3 与 ONW-M1 均保持待验收。
