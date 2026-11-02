# Roadmap 材料自动准备 Spec

用户于 2026-11-02 授权：静态规则初始化准备，动态材料由 SteerLine 自动生成并固定，通过 Runner 正式文件渠道提供，跑通草稿转正式 DAG。普通开发，承接已有 LF/CRLF 修复，不创建开发 Task，不改 execution-map。

## 成果与边界

项目只需维护 Markdown 草稿和项目规范，不手写任务图、来源映射或确认规划 JSON。新增窄命令 `steerline project prepare-roadmap -repository <绝对目录>`，准备 parse-rules.md、document-rules.md 和缺失的 SpecGuide.md；`-check` 只核对。此命令不纳管、建库、创建任务或执行模型，不扩建完整项目初始化。既有自定义 SpecGuide 保留；规则版本不符明确拒绝，不静默覆盖。静态文件通过正常 Git 发布后供固定源码快照读取。

解析从正式 Task owner 和来源映射获取数据，与实际草稿原文一次冻结；文档生成从已确认记录冻结完整规划。动态材料不写业务 Git、工作树或共享文件，不要求用户准备 JSON。源码 commit/tree 与动态输入的原始正文、路径和摘要各自可追溯。

## Runner 正式文件供给

扩展 generate/1 的 `materials` 支持 `kind=file`，复用原材料持久化、请求幂等、私有文件和生命周期；conversation 保持原 inline 契约。file 正文仅用于传输/持久化和私有文件，不展开进模型提示词。提示只提供逻辑路径、真实只读位置和摘要，模型实际读取原文件。固定源码仍用 generate.repository，文件材料的独立 manifest 不混入 Git manifest，不覆盖同名源码。

Runner 公布文件能力和 schema；不具备该能力时在提交前明确阻断。接受后、派发/恢复及最终采用均核验原私有普通文件、内容和摘要；缺失、重定向、篡改、重复/大小写或前缀路径冲突拒绝，不自动重建。Run 与 content_result 的文件 manifest 都须与 SteerLine 原请求完整材料一致。原请求先持久化再提交；未知只查询原号，恢复不读取当前工作树/图重构原正文。

生成文档的 investigation 由模型只返回 source=repository 或 input_file、path、reason。平台用原 CodeCommit 读取引用的 Git 对象并核验 manifest 字节摘要/大小，或采用已验证的原请求输入文件，填写保存的 SHA-256；不要求模型计算或抄写摘要。必须保留固定仓库调查引用，不能用只有输入文件的引用代替代码调查；未知来源、错误类型、路径和来源摘要均拒绝。两类引用即使逻辑路径相同也不混淆。结果只有在 Runner completed 且 stopped 后可进入确认。

动态逻辑路径统一在 `roadmap-inputs/` 下，静态规则保留项目路径。原始正文和摘要不归一化，LF/CRLF 容差仅用于既定的内容一致性判断。JSON 完整生成并固定，不截断、不把空图/空映射当成缺材料兜底。明确无映射时由权威返回的真实空映射可正常供给。

## 通过条件

- RM-A01：新目录准备静态材料、重复调用幂等、只读检查、既有指南保留及链接/规则冲突拒绝，有实际文件证据。
- RM-A02：没有任何手工动态 JSON 的项目可解析、确认和生成文档；请求只含 file 材料，不把长正文放进提示词/schema；固定源码和动态 manifest 分别核验。
- RM-A03：动态内容冻结后，外部原文/图变化不改原请求；缺能力、错误文件、manifest 缺失/篡改和原请求冲突均阻断，未产生重复模型/正式任务。
- RM-A04：隔离真实 Git/SQLite、公开 provider 及真实 Runner/Codex 完成草稿→解析→确认→Spec/Plan/验收→文档发布/合并→正式 DAG；自动派发关闭，记录原请求、模型读取、完整文档、映射及资源停止/清理。真实外部 PR/安装部署另记实际范围。

固定短提示/材料/接线按 [提示词治理](../../docs/prompt-governance.md)列实际正文差异、字节变化并独立审核，不用黄金样本更新替代审核。实施与验证见 [Plan](../plans/roadmap-material-preparation.plan.md) 和 [专属验收](../testing/acceptance/roadmap-material-preparation.md)。
