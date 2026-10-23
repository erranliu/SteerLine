# Git 测试历史文件

每个受测提交一个 `<完整 SHA>.json`，由 `go run ./cmd/test-feedback record -report-dir <本次报告绝对目录>` 合并生成，随 PR 提交。先提交代码再测试，导出后另提交记录。只保存实际执行结果，不手工填写通过。

格式与流程见[报告约定](../reports.md)。SteerLine 只读已核实主分支中的 JSON；本说明不计作历史记录。删除旧 JSON 并合入主分支即可清理页面历史，Git 保留追溯。
