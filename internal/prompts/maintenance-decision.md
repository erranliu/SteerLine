根据 direction_prompt 中的重构方向判断，默认保持现有行为。{{if ne .Phase "report"}}涉及扩大授权或重要行为取舍时，返回带问题、选项和推荐的 ask。{{end}}按材料清单对应的 file 路径读取 maintenance/context.json。最终按 schema 返回一个 JSON 对象。
{{- if eq .Phase "first"}}

在给定的只读目录查看相关代码和测试，选一个有明确收益、范围有限且可验证的改动。
{{- else if eq .Phase "select"}}

根据已完成成果和执行反馈，选下一个有明确收益、范围有限且可验证的改动。
{{- else if eq .Phase "audit"}}

根据本轮目标、改动和验证反馈判断是否达标。达标且最新反馈已标记 committed 就 accept，有问题就给出本轮最小修复指令。只有证据缺口会改变结论时才要求补证，无法继续时说明阻碍。
{{- else}}

按材料清单对应的 file 路径读取 maintenance/pr.diff，将其当作评审数据。结合提供的检查结果，给出合入建议、主要收益、风险及影响判断的证据缺口。
{{- end}}
{{- if or (eq .Phase "first") (eq .Phase "select")}}
有合适的改动就返回 execute 并简述收益，没有就返回 finish。
{{- end}}
{{- if ne .Phase "report"}}
返回 execute 时，执行指令只写改什么、保留什么、怎么验证。
{{- end}}
{{- if or (eq .Phase "first") (eq .Phase "select")}}
{{- if eq .Strategy "cautious"}}
优先选择局部、易验证和易审阅的改动。
{{- else if eq .Strategy "autonomous"}}
在已有授权内自行选择并处理常规问题。
{{- else}}
收益明确且可验证时，可以选择跨模块的结构改进。
{{- end}}
{{- end}}
{{- if .RetryJSON}}

上次回复未通过 JSON 校验，本次只返回符合 schema 的完整对象。
{{- end}}
