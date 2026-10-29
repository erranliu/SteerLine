# 环境准备阶段

项目文档声明环境要求，用户负责在远端预先准备工具、环境变量和PATH。只按文档执行固定验收脚本prepare阶段，逐项检查所有要求并保留通过、不通过及未验证原因；不要在首项失败后省略其余独立检查。禁止搜索安装路径、安装/修复依赖、修改环境变量/PATH、Git配置或受测标准，也不询问用户路径。只能读取文档指定的变量或命令入口；不满足由脚本写最终失败报告并返回blocked。要求全满足才运行原完整L2。此阶段禁止创建PR或手写验收报告；成功才声明实施及必要验证已完成。
读取 STEERLINE_RESULT_FILE 初始JSON取得并保留本轮 activity_id、round、source_commit、script_digest；若含 previous，先读取其中完整前轮报告及诊断，再按同一固定脚本重新验收，不省略历史失败或清理事实；最终报告字段不变，previous 仅作只读输入元数据。按固定环境文档执行脚本。script_digest 核验固定提交的原始Git blob，工作区CRLF转换不算变更。返回前原子写完最终报告；Runner仅在执行结束且生产者停止后封存一次，不等待回传、不用resume刷新结果。



# 环境发布阶段

仅在prepare完整L2通过后执行同一验收脚本publish阶段，这是唯一PR创建入口。使用固定publication base和实际当前分支head，提交专属验证改动，完成必需检查/审核、真实合并和脚本资源清理，再原子写STEERLINE_RESULT_FILE最终报告。未知先查询原PR，不重建；报告原PR URL。不要删除Runner持有工作区/分支或预设验证base。
读取 STEERLINE_RESULT_FILE 初始JSON取得并保留本轮 activity_id、round、source_commit、script_digest；若含 previous，先读取其中完整前轮报告及诊断，再按同一固定脚本重新验收，不省略历史失败或清理事实；最终报告字段不变，previous 仅作只读输入元数据。按固定环境文档执行脚本。script_digest 核验固定提交的原始Git blob，工作区CRLF转换不算变更。返回前原子写完最终报告；Runner仅在执行结束且生产者停止后封存一次，不等待回传、不用resume刷新结果。


# 最终报告契约

最终报告保持结构：{"version":1,"activity_id":"原值","round":1,"source_commit":"原值","script_digest":"原值","completed":true,"checks":[{"id":"项目声明项","status":"passed|failed|incomplete","message":"期望、实际与原因；不得含凭据"}],"l2":[{"test_id":"正式ID","status":"passed|failed|incomplete","message":"真实摘要"}],"pr":{"url":"","base":"固定base","head":"实际分支","checks_passed":false,"reviews_passed":false,"merged":false,"merge_commit":"","cleanup_completed":false,"outcome_unknown":false},"message":"失败或需人工处理的原因"}。completed仅表示报告结束，非成功。checks必须非空、ID唯一并列出全部环境要求，包括通过项和未通过项。仅checks全通过且完整L2/PR均通过才就绪。必须列出全部L2，不以零匹配/跳过/超时为通过；L2失败时PR保留未执行，不伪造通过。PR写入/合并未知时outcome_unknown=true；已创建资源未完成清理时如实写false。

原脚本可原样保留 previous；调用方须与冻结初始文件逐值核对，拒绝篡改及其他未知字段。previous 不进入最终报告领域记录，也不能替代原轮次报告或放宽通过、停止与清理核验。
