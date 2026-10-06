# 产品设计目录与内容校准

日期：2026-10-07（Asia/Shanghai）。源码基线：9b648c69。当前工作树同时保留已授权的单安装版 roadmap 修订，本记录只描述本次文档分类与内容校准，不宣告任何实现任务交付或修改中心 Task。

## 调整范围

- docs 根目录保留 README、整体 architecture 和 testing。具体配置、数据、反馈、工作区、会话、活动及拟议规划/UI 设计进入 product-design/。
- hosting 的有效权威、职责和生命周期原则吸收到整体架构；删除重复/过期文件。state-model 与 data-contract 合并为产品数据模型，去除“答案已应用证明”和旧远端 claim 说明。
- 二期设计按功能命名，原型进入 product-design/prototypes/planning-ui/；推进边界进入 roadmap/phase2-delivery.md，删除空 phase-2 目录。原型 HTML 字节保持，仅在其说明中标明旧演示范围。
- 统一持久格式源码定位，校准旧 schema、activity.jsonl 镜像、自定义 run_dir 和已实现本机全访问策略说明。远端配置中独有的旧连接/协议资料归档，不作为当前配置入口。
- 更新当前 README、设计、Spec/Plan、源码注释及历史 Markdown 导航。历史代码块、命令、Canonical 路径、固定 commit/摘要和原始 JSON/日志不改写。

## 最小检查

| 范围 / ID | 环境和命令 | 通过条件 | 证据 |
| --- | --- | --- | --- |
| 文档静态 / doc-links | Windows、Python 3：python scripts/check_doc_links.py | README/docs 本地导航目标存在；代码示例和外部链接不充当本地目标 | 最终结果见 checks.log |
| 文档静态 / task-identities | python scripts/roadmap_task_id.py check | 当前 roadmap/执行图身份、名称、Spec/Plan 一致；#40/#41 不重入 roadmap | checks.log |
| 文档静态 / whitespace | git diff --check | 无新增差异格式错误 | checks.log |
| 文档静态 / format-sources | 比较数据模型统一表与当前 SQLite、checkpoint、Profile、Conversation 和配置源码声明 | 当前定位值与源码一致，其他设计不重复旧 schema | checks.log |
| 资料完整性 / evidence-integrity | 迁移脚本对本次调整前后所有原有非 Markdown 文件计算 SHA-256，按路径映射比较 | 6,802 份原始证据、身份及原型文件字节不变 | 已通过；含 task-ids/83.json 和原型 HTML，非历史 HEAD 回填 |

不选择 L1/L2/L3、Full 或模型/原生实验：本次只调整文档、导航、一个源码注释和文档检查工具，没有改变产品实现、协议或 E2E 场景。现有未闭合原生验收限制保持；静态检查不补齐那些缺口。

导航检查验证文件/目录目标，不声明历史章节锚点、外部网页或原实验可重放性已通过。历史验证报告只修复导航，原始事实和工具版本保持。当前中心契约仍读取其固定 commit 的路径与字节。
