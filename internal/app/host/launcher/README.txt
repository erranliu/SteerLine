SteerLine home (STEERLINE_HOME)
==============================

Web is the interactive frontend. Install both steerline.exe and
steerline-web.exe into this home/bin before using it.

Start each project with service-window.bat (default etc\SteerLine.json;
pass a config path or set STEERLINE_CONFIG for another project).
Then run start-app.bat, start-web.bat and web.bat in that order.
The service window may be minimized; Ctrl+C requests normal shutdown.
Closing the browser does not stop a project service or worker.
stop-web.bat stops only Web; stop-app.bat stops only the application gateway.
Project services stop through their own service window or stop-service CLI.

One etc\<repo>.json per project and one state\<repoKey> per Git common dir.
Web's stable loopback endpoint is saved in web.json.
Execution profiles are saved in execution.json.
Regenerate these launchers with bin\steerline.exe launchers.

Retained PRs, worktrees and branches are managed with provider/native tools.
