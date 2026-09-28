# Spec：固定版本与双入口

任务 ID：30；任务名称：固定版本与双入口；目标：`PHASE1-RUNTIME`。配套 [Plan](versioned-runtime.plan.md)。状态：待实现与验收。一个独立实现 PR 交付。

## 范围与前置

无未完成硬前置。复用现有安装/启动器、[服务生命周期](pr-03-service-lifecycle.spec.md)与[共同约束](phase1-delivery-constraints.md)。交付不可变版本目录和 stable/dev 各自的服务、控制台、任务 CLI 固定入口；#31 负责实际环境隔离和受控提升。首阶段产品平台是 Windows；Linux Go 构建保持通过，不把构建结果当作 Linux 安装验收。

## 构建与选择

版本目录由 build_id 唯一定位，保存固定二进制与 manifest。manifest 至少包含 build_id、源码 commit、构建时 dirty 标志、目标 OS/arch、二进制 SHA-256、运行/命令/数据库格式版本。干净构建才能成为 stable 候选；dev 允许脏源码构建但必须给出独立 build_id 和准确摘要。发布目录完成摘要校验后才可被选择；同 ID 不同内容拒绝覆盖，同内容重复发布幂等。

只设置 stable/dev 两个 channel，不做可任意扩展的环境框架。每个 channel 的唯一选择记录固定 build_id、manifest 摘要及规范 home；入口从该记录解析精确二进制，不从 PATH 或“最新文件”猜版本。安装、build 和 launcher 重新生成不移动 stable 的选择。首次初始化只在 channel 尚无有效选择时允许；初始化已有生产 home 时须停服并核验原进程/状态，沿用原 home 中的业务数据，不移动或清空数据库。已有 stable 选择的切换必须进入 #31 的提升流程，在该能力交付前拒绝切换。dev 的日常构建切换由本任务交付。

提供 `dev select`，输入 request_id、expected selection version、build_id 与摘要。操作者先停 dev 服务；命令取得 dev 渠道锁及注册项目仓库/状态锁，核验没有活动或未知外部执行、未结算回答/派发，确认目标 manifest 完整且与 dev 当前数据库格式一致，再原子切换 dev 选择并记录原请求回执。允许 dirty 构建。未满足停服/结算/同格式条件时拒绝，不能静默停 worker、迁移库或清库；stable 选择/home 不参与变更。命令不自动重启，操作者从固定 dev 入口启动选定构建。重复请求返回原结果，选择后丢回执不二次切换；开发者可反复构建→发布不可变版本→停止并结算 dev→select→启动调试。

首次初始化已有 home 时，活动/未知外部执行、未结算启动或无法核验的状态必须阻止选择提交，不能用“首次初始化”绕过后续提升门槛。服务窗口、控制台与任务命令在同一次调用中只解析一次选择，传给子进程精确 exe/home/channel/摘要。运行中的服务始终保留启动时 build_id，即使之后选择文件变化，也不热换版本或另起同项目服务。启动器路径带空格、用户自定义安装根、从无关 cwd 调用都须正确。明确渠道的任务命令不能被外部 config/home 参数静默重定向。

## 连接与误用保护

服务/控制台/CLI 显示 build_id、channel、规范 home 和项目；客户端连接在读写命令提交前核验服务实例及同一构建/home/channel。错误入口拒绝写操作并给出实际身份，不能只靠窗口标题识别。复用现有发现和命令服务携带这些字段，不新增业务服务端。直接调用二进制仍受同样身份核验，不能绕过启动器保护。

构建身份、channel/home、服务实例和项目/配置身份分别核验；相同 config_digest 不代表同一构建或渠道。入口解析出的身份固定用于本次调用及子进程，后续发现、握手和服务端命令校验消费该身份，不再重新读取“当前选择”来改写正在运行的实例身份。

任何缺失 manifest、摘要不符、格式不支持或选择文件不完整都在副作用前失败。版本选择以原子替换保存，选择写入中断只暴露完整旧值或新值。通过内部接口提供 ValidateRelease、ReadSelection、InitializeSelection 和选择提交原语；stable 只允许 #31 的提升资格使用，dev 只允许上述离线切换资格使用，禁止公开裸写入口绕过核验。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| VR1 | 两个版本同时存在，构建/安装 dev 不改 stable 文件或选择；同 ID 不同摘要、缺文件、损坏 manifest 均拒绝 |
| VR2 | Windows stable/dev 的服务、控制台和任务 CLI 各自启动同一固定 exe/home；中文/空格路径及无关 cwd 正常 |
| VR3 | 客户端连接错误 channel/home/build 或直接 exe 绕过时，写操作在服务端拒绝；界面准确显示身份 |
| VR4 | 安装器/启动器重复执行幂等；stable 切换在 #31 前拒绝；dev 可连续切换两个含 dirty 的同格式构建并真实启动，重复请求/丢回执只切一次；活动/unknown/错 schema 的 dev 切换拒绝，stable 不受影响 |
| VR5 | stable 在 dev 连续构建/重生成入口期间保持原进程和构建摘要；缺失/不支持版本在副作用前报错 |

真实 Windows 启动与连接证明保存于 docs/validation/versioned-runtime/。本任务不交付公共安装包、自动更新服务、旧版迁移或失败自动回滚。
