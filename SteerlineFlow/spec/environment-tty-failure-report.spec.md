# Spec：TTY前置失败的项目报告收尾

用户授权修改并实测；沿用[发布验收规格](environment-publication-scope.spec.md)和[worker生命周期](../../docs/product-design/worker-lifecycle.md)，不改Runner、生产状态机或安装环境。[Plan](../plans/environment-tty-failure-report.plan.md)。

TTY是被测环境能力。原exec_command TTY入口明确未启动，或纯TTY最小探针已结束且明确失败时，terminal.tty=failed。Agent不得换封装重跑检测；若已有工具仍可用，只能调用同一版本化项目脚本的tty-failure入口记录原错误。该入口不执行任何环境探测、测试、认证或PR操作。超时、启动受理未知、prepare已开始且可能仍有生产者/测试进程时不得使用此入口，保留未知并走原停止；全部工具不可用时保持原报告未完成，不手写兜底。

入口只接受有界非空原错误，固定写terminal.tty=failed，不能由调用方指定通过或未知为已结算；保留原report2关联字段、previous和同轮已有检查/L2事实，缺失的项目检查及L2补为incomplete。项目正常检查与失败记录使用同一声明ID集合。completed=true只表示报告完成。输入须为原始空报告，或同轮已完成且脚本自有清理已确认的报告；部分完成/未知生产者拒绝。拒绝已有PR URL/head_revision、outcome_unknown或PR进度为true的输入，避免覆盖发布事实；不得新增或提升passed、PR或ready。先完整校验result/progress的身份、ID、重复项、状态及布尔进度再原子写，不新增跨文件事务、格式、字段、权威或Runner接口。

正常TTY/prepare/publish仍必须由真实TTY执行。普通工具仅可调用失败记录入口，不可补跑检测；publish前TTY启动失败也可用此入口，保留同轮已验证事实，但不得覆盖已发生/未知的PR动作。文件采集、原Run停止/封存、三轮预算、派发门禁及未知零重发沿既有流程。

验收：TFR-A01 项目脚本实际子进程验证负报告、身份/previous/同轮passed保留、原子写及零探测/PR；空或超长错误/发布事实/部分完成/非法ID状态进度拒绝且两文件不改。TFR-A02 caller三轮接收completed负报告后正常failed/not_ready/settled，不停在unverified、不授予候选资格。TFR-A03 固定execution/generation、项目镜像及文档同步，实际正文/字节经独立审核后更新摘要。TFR-A04 原远端Orca同一正式入口实测，保留原工具错误、脚本负报告、原Run停止封存与清理；环境未ready不算正向L4通过。
