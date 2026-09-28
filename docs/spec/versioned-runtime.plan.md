# Plan：固定版本与双入口

任务 ID：30；任务名称：固定版本与双入口。唯一执行依据：[Spec](versioned-runtime.spec.md)。无未完成硬前置，一个实现 PR 交付。

1. 审计 InstallSelf、InstallLaunchers、项目选择与 re-exec 的路径解析，确定 manifest 和唯一 channel 选择记录。
2. 实现不可变版本发布、摘要校验、首次初始化与受保护的选择提交；交付 dev select 的离线锁/外部结算/同格式检查及请求幂等，stable 切换留给 #31。
3. 接通 stable/dev 的三个入口与子进程身份传递，并在现有客户端/服务握手中核验 build/home/channel。
4. 用两份含 dirty dev 的 Windows 构建执行 VR1–VR5，验证 dev 连续切换并真实启动，以及安装重入、选择写入中断、误连和路径场景，回归已有服务窗口生命周期。
5. 向 #31 提供可调用的版本校验和选择接口及故障样例；保存 docs/validation/versioned-runtime/ 的命令与真实进程证据，按共同约束验证。
