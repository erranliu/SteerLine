# 2026-10-20 自动补验交付证据

本目录复制本轮实际输出，不重新生成历史成功记录。完整成功与失败日志仍在[验收总记录](../../merged-pr-regression-20261020.md)指定的本机证据目录。

- [pre-pr-directed.json](pre-pr-directed.json) / [原始 go test 输出](pre-pr-directed.jsonl)：提交前六包定向 54 项通过，零失败/skip、complete=true。命令、源码基线和工作树状态记录在原输出中。
- [generation-takeover.json](generation-takeover.json)：同一生成 Run 在 A/B 间的原始公开快照，包含 SHA、服务实例、input/final/content_result。
- [proof.json](proof.json)：该 live 包程序/报告/API/浏览器与正式结算完成记录。
- [owned-refs-cleanup.json](owned-refs-cleanup.json)：同一包 base/head 原 commit/Run 绑定与清理后验。
- [qualification.json](qualification.json)：同包只读资格复核，原 Run 全部结算、两分支不存在且 PR 数为零。
- [final-summary.json](final-summary.json)：此前 83 项 Runner、20 项调用方定向、三个受控场景及 i/j 遗留收尾汇总。

历史 A/B 的 source_dirty=true 及构建身份保持原样；本目录不证明最终提交的干净构建已经重新执行真机全流程。Runner 主干 #37 的按能力校验实现被完整保留，本次新增 queued/result、codec、材料及实际 admission 回归在接上主干后另行复验。人工主观体验与 Orca 外部接口前置仍为待验。
