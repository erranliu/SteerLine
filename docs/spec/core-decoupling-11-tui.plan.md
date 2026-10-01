# Plan TUI与中心实现解耦

任务 ID：72；任务名称：TUI与中心实现解耦；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-11-tui.spec.md)及[共同约束](core-maintenance-constraints.md)。硬前置：#71 整体交付；一个实现 PR。

## 实施顺序

1. 接收 #71 的产品契约/adapter/暂存桥清单，核对 console 所有 model/source/control/profile/view 和测试消费者；标明纯 UI 与默认 I/O 归属。
2. 外层应用注入同一个客户端实现，ProjectRef/Snapshot 和接口参数切换公共 DTO；把配置/开库/探测/控制剩余消费者归 adapter，删除薄桥与中心类型别名。
3. 模型只管理 UI 状态，方案/资格读取服务事实，提交使用精确身份/版本和已保存请求 ID。中性 fake 接通 UI 单元，实际应用构造接线在外层集成保留。
4. 增补或扩展既有依赖守卫覆盖 console 全部 OS 文件及测试，更新模块说明、测试映射和 quick 必要入口；不放宽主包最终隔离。

## 验证与交付

CM72-1 对应全源码依赖审查、禁用 import 的守卫负向 fixture 及实际注入构造；CM72-2 从现有任务进入/启动、会话锚点/正文、方案选择/决策、请求身份/回执测试各选必要代表，不重复 UI 全矩阵；CM72-3 对应一组外层 CLI/TUI 构造与既有 quick/守卫。

各测试迁包保留原断言及实际命中记录，TUI fake 不替代中心资格或协议认证。稳定后将定向选择和 quick 合并一次执行，运行 git diff --check，不跑 full、不新增完整视觉验收。

PR 列 console 禁用依赖检查、所有默认 I/O 和薄桥的删除清单、客户端/视图所有者及实际命中。与存储线仅协调组装/守卫段，领域/写端口不修改。规划阶段不运行代码测试或中心任务。
