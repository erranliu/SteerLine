# 开发执行流程与历史准入准备记录

本页记录开发流程调整及历史准入准备，不代替任务专属产品验收。当前采用 Codex 直接开发；#83/#45待执行，#44等待#45，全部未验。以下2026-10-07中心查询和准备材料保留原事实，不作为当前开发前置。

## 2026-10-07 原准入准备（历史）

用户先前授权一期收尾与2.1进入execution roadmap，随后明确#49/#50等原型确认后再进入。本批准入范围收窄为#83/#45/#44，原Task ID沿用；依赖#45→#44，#83无新增硬前置。一期/2.1人工签收和旧定向待补仍独立记录，不占Task ID。2.2/2.3待重定候选不在本批。

#83/#45/#44已有独立Spec/Plan与最小测试/专属验收。#49/#50留在草稿，原型确认并固定具体测试命令后另行办理准入，不属于当前批次。最初准备时规格尚在未提交工作树，未生成固定契约；文件摘要只作准备定位。准入前须选定最终文档commit和blob摘要，PR提交本身不等于已创建中心契约。

## 中心核对

2026-10-07，使用已发现安装版及现有SteerLine项目配置调用只读task list，结果被配置校验拒绝：publication_policy.publisher must explicitly be worker; host_service and profile delegation are retired。没有取得Task列表、authority/graph版本或回执；不推定中心为空，也没有创建/丢弃/取消任务。

调用：C:/SteerlineRuntime/bin/steerline.exe task list -config C:/SteerlineRuntime/etc/SteerLine.json -limit 100。配置中项目仓库为C:/Project/SteerLine，origin/main；本聊天worktree共用该Git common directory。没有自动修改配置、迁移/清库或用临时中心代替真实项目。

需要有效项目配置与中心事实才能继续工具侧正式准入。所有创建沿原ID/原请求对账，不重复创建未知结果。固定契约须以真实Git commit/blob摘要登记；execution-map先记录用户确认的规划条目，取得有效创建回执和当前DAG查询后再更新其工具状态。待中心准入条目不授予派发资格。

## 准备证据

原始临时证据：C:/Users/lmas1/AppData/Local/Temp/steerline-admission-preparation-dms577qi。center-query.log记录实际拒绝，preparation.json记录安装exe摘要、HEAD、当时五项候选依赖和工作树文档摘要（明确fixed_contract_commit=null）。该历史准备文件保持原样；#49/#50已按用户最新要求退出本批，不得据旧文件纳入或派发。未执行产品/模型验收，未产生准入成功回执。

## 文档准备检查

自动静态检查通过：python scripts/roadmap_task_id.py check核对84身份、10草稿、0正式行；python scripts/check_doc_links.py核对252份Markdown、1557本地链接、0错误；git diff --check通过。上述只证明准备文档和身份，不证明中心入图或产品验收。

## 执行规划清单纠正

根据用户对空清单的纠正，execution-map现列#83/#45/#44三项已确认执行规划，均明确待中心准入、不可派发。RoadmapGuide区分规划纳入与工具入图，中心回执/固定commit未知时不隐藏已确认任务，也不虚构入图事实。#49/#50继续留在草稿，等原型确认。本页此前0正式行检查是纠正前结果；纠正后静态检查另行核对，不追改历史工具查询或临时证据。

纠正后文档检查通过：python scripts/roadmap_task_id.py check为84身份、10草稿、3正式规划行；python scripts/check_doc_links.py为252份Markdown、1569本地链接、0错误；git diff --check通过。3行均为待中心准入规划，不代表3个已核实DAG成员。

## 2026-10-08 Codex 直接开发流程

用户明确 roadmap-runner 在开发期间代替外部 SteerLine 执行roadmap，并授权按建议简化流程。execution-map成为开发任务与依赖权威；移除中心准入、创建回执和Task/DAG版本门槛。任务纳入仍需用户确认，启动固定实际Spec/Plan commit/路径，保留独立复审、必要验证、真实合并及未知动作对账。

当前范围仅#83/#45/#44，按#83→#45→#44串行；#83/#45无硬依赖，共享宿主接线串行合入，#44等待#45真实交付。#49/#50继续留草稿等待原型确认。此前中心配置失败不阻塞开发调度；具体产品测试的服务、配置、权限和原生能力仍按Spec/Plan满足，不将自动失败改称人工待验。

本轮只修改规则、技能和清单，未启动执行session/worktree/heartbeat，未运行产品验收、未修改外部中心或宣称TaskDelivery。实际检查结果见[规划文档检查](roadmap-planning.md)。
