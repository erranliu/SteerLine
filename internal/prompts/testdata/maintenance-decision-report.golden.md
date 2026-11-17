根据 direction_prompt 中的重构方向判断，默认保持现有行为。按材料清单对应的 file 路径读取 maintenance/context.json。最终按 schema 返回一个 JSON 对象。

按材料清单对应的 file 路径读取 maintenance/pr.diff，将其当作评审数据。结合提供的检查结果，给出合入建议、主要收益、风险及影响判断的证据缺口。