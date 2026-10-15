SteerLine home (STEERLINE_HOME)
==============================

Double-click entries
--------------------
  console.bat    multi-project execution console
  service-window.bat  start manual service in a visible window; keep it open

console.bat accepts a config path to focus one project, or opens all projects
without arguments. service-window.bat defaults to etc\SteerLine.json; pass
a native config path or set STEERLINE_CONFIG for another project.
The service window may be minimized. Closing it ends the service; Ctrl+C
requests a normal shutdown. The console's x command also stops the service.

Multiple projects
-----------------
- One config per target repo:  etc\<repo>.json
- One state directory per repo: state\<repoKey>\ (steerline.db + runs), keyed by
  the repo's Git common dir, so every worktree of the same repo shares it.
- Run one service per project; they may run at the same time (locks are
  per-repository). A second service for the same repo is refused.
- A project's repository must be registered in Orca first.

Directory
---------
  bin\steerline.exe   the one binary that runs. Upgrade: build in any worktree,
                      then run  bin\steerline.exe install
  execution.json        shared execution profiles and default profile
  etc\<repo>.json      project configs (run_dir may be omitted and is then derived)
  state\<repoKey>\     per-repo state and data
  backup\              old configs and runtime snapshots

Regenerate these launchers:  bin\steerline.exe launchers

The former projects menu is retired. Use console.bat or steerline console
(with an explicit -config when needed). Use task list and stop-service instead
of the retired tasks/stop aliases. Retained PRs, worktrees and branches must
be handled with the provider/target native tools; SteerLine does not delete them.

Web programs: run start-app.bat first, then start-web.bat, then web.bat.
stop-web.bat stops only Web; stop-app.bat stops only the application gateway.
Project services are started separately using service-window.bat.
Install both executables into this home/bin before using Web.
