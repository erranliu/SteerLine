# SteerLine 公共执行环境要求

项目按运行机和执行链路维护自己的环境说明与单次检测入口，接口见[环境适配指导](../SteerlineProjectGuideDoc/environment-adaptation.md)。

环境检测只确认所需解释器、工具/SDK、权限及服务是否可用。辅助程序和实际采用的环境要求配置可显式声明为依赖；环境说明和入口自动计入资格依据。

项目测试、文档导航、Spec/Plan、roadmap、构建验收和 PR 发布沿各自流程执行，不参与环境检测或资格失效判断。原 tty/prepare/publish 分阶段脚本及其测试已删除，不保留旧入口或回退路径。
