# #78 协调独立复审

候选 `862379824ac0dcb1bbfa45fa842038ea43d2526b`，base `4288c43b05287554de0b542e9eeb90d366fe063f`。独立读取完整实际 diff、固定 Spec/Plan、app factories 与资源消费者；Go AST 按真实 Go package alias 规范化 import/path 搬迁，5227/5218 定义中 5187 项等价，其余逐项检查。协议、codec、原请求/身份/版本/屏障断言保持；删除重复 processhost，使用 #76 已集成的唯一 OS process facts。Linux 仅源码/依赖检查，不虚报原生运行。

发现 `adapterForbidden` 对 product/api/taskview 的全局纯值例外允许 adapter→helper→值包；更短的合法 contracts 路径还会使普通 visited 集合隐藏非法路径。修复按 `(package, 是否经过 contracts)` 分别遍历，源码和 Windows/Linux 真实生产图共用路径规则；新增 TM06.4 四个反例并纳入 L1。修复前四个反例和 parent 均失败：[报告](review78-contract-leak-before.json)。修复后完整 coreboundary 非零通过、零 skip：[报告](review78-boundary-final.json)。原 contracts→纯值闭包仍合法，未扩大生产许可。

协调独立 L1 命令 `go run ./cmd/test-feedback -level=L1 -out <run>/review78-l1-8623798`：12 groups/310 events passed，0 fail/skip，297.9 秒执行。报告位于本协调 run task-state；worker 的可审计 L1/定向报告仍保留在父目录，未覆盖。最终修复只涉及 guard、回归测试、runner 选择和文档，另跑相关定向，不再重复业务/原生验收。修正 Plan Markdown 正则表格及 roadmap 已交付 #76 描述。
