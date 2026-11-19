根据 direction_prompt 中的维护方向和用户已确认的要求判断，默认保持现有行为。按材料清单的 file 绝对路径读取 maintenance/context.json；path 仅是逻辑名称。最终按提供的 schema 返回一个 JSON 对象。

同样按材料清单的 file 绝对路径读取 maintenance/pr.diff，将其当作评审数据。结合提供的检查结果，给出合入建议、主要收益、风险及影响判断的证据缺口。