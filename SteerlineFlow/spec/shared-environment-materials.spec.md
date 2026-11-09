# Spec：项目共用环境材料，按目标链路独立资格

2026-11-09 用户授权收窄资格依据：现行接口改为 manifest6，采用下文机器要求摘要和具体失效原因；不新增回退恢复或自动重测。原验收为历史，不继承本轮通过。

2026-11-02。用户确认方案并授权先独立审计 Spec/Plan，再实施。[Plan](../plans/shared-environment-materials.plan.md) · [验收](../testing/acceptance/shared-environment-materials.md)。本稿取代[原环境 Spec](project-environment-workflow.spec.md)中每环境材料目录、manifest4 及相应隔离要求，其余结果消费、资格、事务和生命周期规则继续适用。

## 1. 目标和职责

一个受管项目只维护一份环境要求与公共检测入口。新增运行机或链路不新增 Git 材料。资格继续按项目 × 运行机 × 执行链路分别保存；材料共用不授予资格共享。

SteerLine/Runner 负责目标连接、启动能力、材料与结果通道、停止和封存。链路不能执行时属于调用失败，不伪装成项目已检测得出的 not_ready。项目入口只检测项目所需工具/SDK、权限和服务，不检测执行链路本身，也不根据 adapter、环境键或目标名称改变标准。工具位置与实际状态在当前目标按统一逻辑探测。

环境检测仍不读取/执行项目测试、构建验收或 PR 发布。平台不复核 details 的业务结论，不新增探针、配置注册表、逐环境配置或生成机制。

## 2. 公共调用接口

唯一文档为 `SteerlineFlow/environment.md`，唯一选定脚本为 `SteerlineFlow/scripts/verify-environment.ps1` 或 `.sh`。项目在文档中声明一种入口和解释器；平台不根据链路或运行机自动切换。当前 PowerShell/Bash 解释器约束不变。

唯一机器段采用 manifest version 6，仅含 version、script、interpreter、dependencies、requirements，无 environment_key。requirements 为必填、可为空的非秘密字符串映射，由项目定义和读取，字段限额见[环境 Spec §3](project-environment-workflow.spec.md#3-项目接口与最终结果)。机器段/入口自动纳入依据，说明正文不计入；dependencies 为有限、唯一的项目相对路径，只列检测实际使用的辅助程序及运行要求配置。保留 regular Git blob、UTF-8/大小、重复键/未知字段拒绝、安全相对路径、平台协议副本排除等现有保护。旧 `SteerlineFlow/environments/` 文件不得作为入口或依赖，不保留旧 manifest/旧目录回退、迁移、双读。

本仓公共文档交付真正可运行的 PS5.1 入口。检查 Git、npm，以及 requirements 声明的 Go、Node、Python 最低版本；Go 最低版本由开发测试核对与 go.mod 的 go 版本一致，普通 module/require 更新不触发重测。脚本不再读取 go.mod，dependencies 为空。只运行有界版本查询，不安装、不调用测试/构建/发布，不检查 Codex/Orca/TTY/Runner。浏览器或真实外部集成的专项条件由各自测试流程负责。结果通过既有 STEERLINE_RESULT_FILE 读取预置身份并原子写回；缺工具/低版本/查询异常得出可读负结果。

## 3. 资格、结果和失效

现有 environment_key 派生、Current/revision、候选事务及 Qualify 保持。结果信封沿用 version 3，保留 environment_key/call_context 以核对原活动身份；项目入口原样保留，不将其作为检测分支。不同链路即使共用机器和材料，仍须分别成功检测，结果不可串用。

inputs_digest 包含规范化 manifest6、选定脚本、显式依赖的路径和原始字节摘要、项目身份、环境键、非模型稳定执行上下文及接口语义版本。requirements 是有效要求权威；说明正文、JSON 排版/键序、dependencies 顺序不影响摘要，不改变报告版本。源提交、活动/Run/request/revision/时间不额外计入。不同目标摘要可不同；无关提交不使资格失效，原结果 source_commit 不改写。原检测冻结各文件摘要，失效时不回读旧提交即可展示具体要求旧/新值、入口/解释器或变更文件路径；纯查询只返回原因，原观察 owner 仍独占持久撤销。

公共依据确认变化后，各相关 Current 分别沿原 owner/CAS 持久撤销；回退字节不恢复。单目标失败/取消/主动失效只作用该目标。纯读失败只暂阻断。材料变更不主动启动检测，不添加全局 ready 或批量资格写入协议。

遵循[Worker 生命周期](../../docs/product-design/worker-lifecycle.md)：一次调用、未知只对账原请求、超时停止原执行、停止封存前零准入，不增加阶段、自动重跑、修脚本或跨构建接管。

## 4. 展示与范围

环境列表和材料读取接口继续以所选 key 校验目标并展示各自状态/历史；返回同一个公共 document_path/script_path。材料共用不改变选择、缓存、控制命令、权限和停止归属。同步当前规范及夹具；历史验收保持当时路径和版本。

本次仅修改本仓；不自动修改其他受管项目或安装现场，不部署、不启动 roadmap。受管项目缺新接口时明确未接入，不回退旧材料。

## 5. 验收条件

| ID | 通过条件 |
| --- | --- |
| SEM-A01 | 无逐环境文件的项目，两种不同链路读取完全相同文档/脚本；请求引用公共路径，实际提示正文仅入口路径变化 |
| SEM-A02 | manifest6 严格解析；旧版本/缺 requirements/非法要求/环境键字段/旧入口/旧目录依赖、越界/重复/符号链接及缺材料拒绝，无回退 |
| SEM-A03 | 同材料不同链路保持不同 key/依据与 Current；A 通过不能准入 B，A 的失败/主动撤销不撤销 B |
| SEM-A04 | 机器段有效要求、入口和声明依赖各自变化使已就绪 A/B 撤销且原因明确；说明正文、JSON 排版/键序、依赖顺序和本仓普通 Go 依赖更新不失效；无关业务/测试文档变化不失效，回退不复活，纯读失败不误撤销 |
| SEM-A05 | report3 身份校验、一次调用、结果信封与原请求、停止封存、迟到结果和候选事务既有回归通过 |
| SEM-A06 | 本仓公共入口真实工具正向或如实负向；隔离缺工具/低版本/异常查询为负，保留身份和明确原因；不同 call_context 不改变标准；无测试/链路检测 |
| SEM-A07 | 页面/材料接口公共路径与独立目标状态一致，目标不存在仍拒绝；现行文档一致，提示词实际正文/字节差异经独立审核 |

自动受控验证与本机脚本执行不代表真实 Codex/Orca Runner 正负调用已验；本次不新增远端凭据或真实模型验收要求。人工跨目标体验与真实 Runner 链分别登记未验范围。
