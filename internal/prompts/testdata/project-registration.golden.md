请将你当前工作且已完成初始化的 Git 项目接入下面的 SteerLine 安装实例。

安装目录（STEERLINE_HOME）：D:\Tools\SteerLine 安装

先核实服务机器上的 Git 根目录、remote、目标分支和 PR 发布目标，确认初始化材料已进入目标分支。材料缺失或尚未发布时，请说明缺项并请我先完成初始化或发布。

打开该安装实例的 SteerLine Web，从项目切换器或零项目页面进入「添加已初始化项目」。填写项目名称、服务机器上的仓库绝对路径、Git remote、目标分支、PR 提供方、服务地址和仓库，核验目标提交。Gitea 如需凭证，只填写服务机器上的令牌环境变量名。若你无法操作此页面，请把目标提交和可直接填写的参数交给我。

请在核验后交给我确认添加。保存后，使用安装目录下的 `restart.bat`，或为本次命令设置上述 `STEERLINE_HOME` 并调用该目录下的 `bin/steerline restart`（Windows 为 `bin/steerline.exe restart`）。重启后，从项目切换器核对项目是否已加载。