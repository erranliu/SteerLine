# #77 协调独立复审与集成

原候选 `28806633d436d9e18e04528c16c1f76b6a7901a1`，原 base `4288c43b`；先集成 #78 已合入 main `fdb9ad75b1c5797503af9ced198732c62c9968dc`。executor 停写后协调唯一修改。全部旧/新 Go AST 对照有 4923/5227 定义在 namespace 搬迁后等价，287 个非等价项逐项检查，另检查新 API DTO、query/app 投影、原候选身份/版本/回执与真实调用路径；未把只读准备模块当成生产证据。

确认并修复：活动身份和事件分两次读取，改为唯一 SQLite 只读事务并在两节之间真实 WAL writer commit 取证；两份活动 renderer 改为唯一纯 UI 视图；迁移后的 query inactive-tag import 与 adapter→新 console 路径补守卫；真实任务查询的 task_not_found 公开错误分类保持；迁移遗漏的 501 候选排序断言恢复到产品 projector，原 renderer 断言随唯一实现迁入。最后修订源码 AST 哈希索引、架构、Plan、共享交接和测试映射。

首轮集成 L1 12组中仅 quick-execution-boundary 失败：TM06 临时模块仍按迁出 console/query roots 建立，真实仓库图与其他组通过。失败报告保留为 review77-l1-integrated.json，未记通过。修正临时模块种子为当前实际目录，最后再跑 review77-l1-final；最终结果另附，不覆盖失败。

已通过定向：final-targeted2 九包/183事件、SQL/app 两包/7事件、真实候选控制台 wiring 两事件/141秒、恢复的候选排序与最终事务两事件，全部0fail/skip。报告保存于本目录，原命令/版本与原始Go JSON位于本 run task-state。最后只新增测试的非零命中断言，额外定向通过；未改变已验业务代码。不启用 Full/L2/L3/live 或远端准入；#39 本机协议/权限/native启动和停机实现证据仍有效。

最终 L1：12/12 组、425/425 events passed，0 fail/skip，420.2 秒执行；[实际 summary](review77-l1-final.json)。运行开始时工作树正在集成，实际 Go 生产内容对应最终 `b974fd66de45f41df05f21ac0e60c840a265492b`；其后只补已定向验证的测试断言与文档，最终提交全部生产内容复核一致。首轮失败没有覆盖。
