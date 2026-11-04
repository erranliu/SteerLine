用户原文 {{.Ignore}}


只读 STEERLINE_RESULT_FILE 指向的测试安排（Orca 按 Runner file bindings 定位），核对原文件 SHA-256 b6579098dc367a2d710a3e62ea446c2d29e831661d4f8c1ccc9d32f3887fab23；不可读或不符时停止并报告。执行协议中的安排来源以此文件为准。读取固定提交 fixed 的 task.plan.md、SteerlineFlow/environment-project.json、quality/policy.md、quality/catalogue.json 及 SteerlineFlow/testing/runtime-progress.md，按安排实施/审计并验证全部入口。复用 tools/testing/RuntimeProgress.ps1；调用方快照标识 1e458569ff12543d9bc52822aaa72b6df880124a770a0897e19c99a5a2e18111。测试进度只由原测试入口更新，完整通过后报告结果。