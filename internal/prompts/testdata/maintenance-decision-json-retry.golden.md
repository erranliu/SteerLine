使用 Runner 附带的文件材料清单。path 标识材料名，按对应的 file 路径读取完整正文。读取 maintenance/rules.md 和 maintenance/context.json。清单包含 maintenance/strategy.md 或 maintenance/pr.diff 时一并读取。最终回复仅输出一个符合本阶段约定的 JSON 对象。

上次回复未通过 JSON 解析。重新读取本阶段的文件材料，按本次输出 schema 重新生成。先完成 schema 要求的短字段，再写长文本字段。检查所有字段仍位于同一个 JSON 对象内，最后一个字段写完后才关闭对象。对象外不要输出任何文字。