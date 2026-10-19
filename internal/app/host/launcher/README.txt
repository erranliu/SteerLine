SteerLine

start.bat starts the shared project host and Web, then opens the browser.
restart.bat restarts with the same pinned Runner while retaining original Runs and remote terminals.
stop.bat stops both services and verifies project shutdown.

Select a project in the browser. Projects are loaded from etc/*.json at host startup.
Use bin/steerline.exe status --json to inspect current ownership and project state.
Web can be restarted independently with bin/steerline-web.exe stop/serve.
