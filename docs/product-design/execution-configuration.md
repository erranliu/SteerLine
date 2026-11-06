# 执行配置

本文维护执行预设、条件保存、输入冻结和 home 级开发开关。SteerLine 保存业务配置，Runner 解释原生参数及工作流；当前组合和必要能力见[Runner 接入](../integration/steerrunner.md)。开发开关不改变任务、候选或原生事实，也不授予新的业务权限。

## 配置与选择

所有项目共享 home/execution.json，严格版本为 3；至少一个预设，default_execution_profile 必须指向已配置项。runner 固定 EXE 绝对路径、版本与 SHA-256，execution_profiles 保存唯一 id、名称及 execution/workflow。组合与默认值按所选 Runner 的公开自包含 schema 校验，不加载文件/网络额外 schema，不补原生默认值冒充生效值。

项目仓库、发布目标、任务契约和认证正文不随全局预设复制。参数摘要仅覆盖 execution + workflow，名称与部署选择分别维护。示例见[execution.json](../../examples/execution.json)；旧开发格式、implementation_configs 和内部 codec 不兼容、不迁移或回退。

当前支持本机 Codex、Windows/amd64 本机 Orca/Codex、Pi、DSH 及远端 Orca/Codex，资格取公开目录与产品校验；发现其他 Agent 不自动开放执行。provider 凭据、OAuth、base URL 和自定义模型由原 Agent 环境管理。

## Web 配置管理与生效

/settings/execution-configs 在零项目时也可用。Runner 的无服务 native-catalog 负责原生目录、连接和元数据进程，execution-config 负责原生字段、用途转换及来源 identity；Web 只维护逻辑草稿、在途查询、产品支持判断和条件保存，不自建 Orca named pipe 或 Codex model/list 协议。

新增/改变 execution/workflow 必须恰有一条 profile_edit；selection/unset 与保存前 base_profile_id 从同一可信保存版本重建并比较。名称、默认项及删除不伪造 edit；缺能力或非法原文只读显示，不清洗旧字段。

保存按配置摘要条件写入并原子替换，冲突保留输入，失败不覆盖其他页面配置。页面区分草稿、已保存待重启和生效未知；保存后须正式重启，不热加载或自动停服。保存至重启之间按原变更保护暂停新执行受理，已运行执行继续对账；冻结调试包只读。

目录查询使用 home 上下文，不继承项目 cwd/相对 PATH，只合并同上下文在途请求，失败标过期。目录、默认账号和工作区可见性不证明登录有效或项目就绪，环境检测另行办理。接线见[配置边界 Spec](../../SteerlineFlow/spec/native-execution-configuration-boundary.spec.md)及[验收](../../SteerlineFlow/testing/acceptance/native-execution-configuration-boundary.md)，操作归[Web README](../../web/README.md)。

## 两层默认链路

| 层次 | 用途与归属 |
| --- | --- |
| SteerLine | 用户未显式选择时，由 default_execution_profile 选取已有预设 |
| Runner | 工作流 parse 等内部 LLM 操作使用自身公开契约定义的默认入口、覆盖和优先级 |

两层初始默认均为本地 Codex，各自服务不同范围，不共享配置或会话。显式选择不被覆盖，缺预设不自动补齐；候选选远端 Orca 不推定内部 parse 也用 Orca。默认链路不改变冻结运行，不作为失败后隐式改投；职责设计也不证明每项内部能力均已接入。

## 冻结与工作流

候选预留时固定预设 ID/名称/摘要、任务材料、base 与完整提交；Runner 受理时固定规范化 execution/workflow、材料及模板。每轮在结果与授权确定后保存实际输入、模型和来源，不预填未来轮次或原生身份。配置刷新、恢复不重渲染原请求。

普通/审计流程的 ID/version、支持组合、审计模型和允许覆盖步骤取公开目录。Runner 管步骤、裁决、修复与补报预算；SteerLine 不从终端文字推进阶段，不因新增预设取得另一套发布资格。契约见[Runner 协作](../../SteerlineFlow/spec/runner-collaboration.spec.md)，实例、停止和换构建排空见[生命周期](worker-lifecycle.md)。

## 调用方设置与环境边界

max_generate_concurrency 是 home 级辅助 generate 准入上限，省略为 5，显式值为正整数；项目已登记生成共享队列，名额覆盖实际执行和待核实请求，开发候选和 Runner 内部辅助操作不计入。保存后重启生效，范围见[生成队列验收](../../SteerlineFlow/testing/acceptance/generate-queue.md)。材料准备仍须按[项目维护](project-maintenance.md)显式登记。

usage_policy 的采集授权与模型权限分开，保存、生效和远端范围归[计费职责](../../SteerlineFlow/spec/billing-ownership.spec.md)。Git、Codex、Orca 与登录属于执行环境；原生沙箱、写入根、helper 和停止证据归 Runner。远端使用执行主机登录，不接受本机 helper 路径/SHA，不套用本机权限结论。不复制项目认证或扩权绕过阻断；项目环境使用普通 Run/封存结果，旧 Check 已退役。

## 开发者配置与环境就绪阻塞

当前普通开发构建提供“配置 → 开发者配置”，所有项目共用 execution.json：

```json
"developer": {
  "skip_environment_readiness": false
}
```

界面“环境未就绪时阻止执行”默认开启，对应 false。关闭后，未检测、失败、撤销或材料失效的**项目环境资格**不再阻止手动启动、新增候选及自动派发。原检测结果不改为 ready，页面显示阻塞已关闭。重新开启并重启后仅新候选恢复检查，原请求重放保留历史事实。

覆盖不绕过项目统一就绪、材料/测试安排、未结算环境占用、执行方案、Runner、任务依赖、版本、模式和名额。最终预留在原事务核验；environment_readiness_skipped 由 writer 的可信策略写入且后续不可变，只是该次事实，不能反向授予免检资格。

定义与默认值集中在[developerconfig](../../internal/developerconfig/config.go)，采用明确布尔字段、默认关闭覆盖；定义包无文件读取、环境探测或可变全局状态。应用[能力映射](../../internal/app/capabilities.go)在加载时将构建/配置转为业务所有者的不可变策略，零值要求检查，按值供服务、writer 和查询使用，生命周期内不热切换。业务不读取原始开关或持有整个开发者配置包，后续开关仍从同一组合入口映射。

[源码守卫](../../internal/testkit/boundary/developer_config.go)把原始配置限制在定义、全局配置类型、应用校验/映射及 Web 编辑器。其余生产 Go 不导入 developerconfig 子树或访问原 Developer/SkipEnvironmentReadiness，经 Config/DTO/辅助包间接读取也拒绝。扫描覆盖全部平台/build tag；测试夹具可构造，注释/字符串不计访问。现有 L1 边界入口及正反例测试保护，不另增测试根或 E2E 场景。

## 发布构建与移除

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\build.ps1 -Web -Release
```

-Release 给 SteerLine 和 Web EXE 添加 release 标签，Web 仍保留 webui，不改变 Runner 构建。直接构建分别用 go build -tags release ./cmd/steerline 和 go build -tags webui,release ./cmd/steerline-web。

发布能力常量为 false：Web 返回 developer_available=false 并隐藏整类入口，配置校验/写入及应用加载拒绝非默认覆盖，组合层始终注入正常门禁。发布前删除覆盖或置 false，环境变量、项目配置、请求或手改文件不能重新启用；业务策略/存储只消费可信策略，不依赖 release 标签或原开发配置。

未来若彻底退役开发覆盖，移除集中定义、面板及组合层映射，业务策略恢复默认即可，无需改变业务模块依赖；历史标记只作记录。本功能不新增表、迁移或旧格式读取路径。

## 验证与历史

开发开关的实际结果见[开发者配置验收](../../SteerlineFlow/testing/acceptance/developer-configuration.md)，不能从源码接线推定发布构建或安装版已验。旧原生探测、接管和历次配置证据见[第四批历史入口](../../SteerlineFlow/archive/design/runner-reliability-history.md#第四批整理来源)，两份旧文档及合并来源见[历史入口](../../SteerlineFlow/archive/design/runner-reliability-history.md#首轮文档合并)。本文维护当前配置规则，后续扩展与退役说明不代表已交付能力。
