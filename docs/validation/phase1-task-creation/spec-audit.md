# 第一阶段新任务规格审计修订

本记录只说明文档审计整改，不代表实现或平台验收通过。任务身份为 #26–#32，各自拥有独立 Spec/Plan；与已有 #14–#24 的补边只调整中心执行资格，不改其固定契约。

| 审计问题 | 修订归属与验证要求 |
| --- | --- |
| stable 提升预检后仍可能派发新执行 | [#31](https://github.com/erranliu/SteerLine/blob/0decdeb0321845e4e388a0aaa51063d27cc87cf8/docs/spec/runtime-isolation-promotion.spec.md)增加项目级持久维护屏障、事务仲裁、部分取得/重启/离线写者处置及旧源协议离线回退入口；RP7 覆盖竞态 |
| dev 无日常构建切换入口 | [#30](https://github.com/erranliu/SteerLine/blob/0decdeb0321845e4e388a0aaa51063d27cc87cf8/docs/spec/versioned-runtime.spec.md)定义离线 dev select：精确锁和外部结算、同 schema、dirty 构建、请求幂等；VR4 验实际切换启动 |
| multiple Task 整体失去补边和退出能力 | [#26](https://github.com/erranliu/SteerLine/blob/0decdeb0321845e4e388a0aaa51063d27cc87cf8/docs/spec/task-candidates.spec.md)恢复零候选补边/丢弃和未发布候选全部结算后的任务级变更；[#27](https://github.com/erranliu/SteerLine/blob/0decdeb0321845e4e388a0aaa51063d27cc87cf8/docs/spec/candidate-delivery.spec.md)扩展已关闭未合并 PR 的安全重置及历史 PR 继续核实；CM5/CD7 验收 |
| 旧契约摘要保留规则不具体 | #26 固定 schema 5 源格式、legacy-v1/candidates-v2 元数据与算法，禁止试算回退；CM6 验原字节/摘要与恢复身份 |
| 缺同 Task 混合后端真机证明 | [#29](https://github.com/erranliu/SteerLine/blob/0decdeb0321845e4e388a0aaa51063d27cc87cf8/docs/spec/candidate-native-acceptance.spec.md)新增本机/远端混合候选的真实问答、中心重启、PR 及资源结算，CN7 单列 |
| 长期运行资源门槛不可判定 | [#32](https://github.com/erranliu/SteerLine/blob/0decdeb0321845e4e388a0aaa51063d27cc87cf8/docs/spec/phase1-release-acceptance.spec.md)固定 60 秒采样、99% 覆盖率、180 秒最大间断、预热/初末空闲窗口、Private Bytes/句柄/goroutine 增量及待办时限；P1A2 覆盖 |

文档检查覆盖任务身份、每项独立 Spec/Plan、直接依赖、合并后的 DAG 无环、链接及 Spec 不反向依赖 roadmap。实际中心创建与补边回执另存于本目录，不能由本审计记录代替。
