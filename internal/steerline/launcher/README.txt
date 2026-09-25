SteerLine home (STEERLINE_HOME)
==============================

Double-click entries
--------------------
  projects.bat   open the console or choose a project action
  console.bat    multi-project execution console
  viewer.bat     compatibility shortcut to the current console
  run.bat        run one project's scheduler in this window (Ctrl+C to stop)
  service-window.bat  start manual service in a visible window; keep it open
  stop.bat       ask one project's service to stop
  home.bat       print the resolved paths

console.bat and viewer.bat accept a config path, e.g. console.bat etc\other-repo.json
Use service-window.bat for the console upgrade's manual service mode.
The service window may be minimized. Closing it ends the service; Ctrl+C
requests a normal shutdown. The original run.bat retains automatic dispatch.

Multiple projects
-----------------
- One config per target repo:  etc\<repo>.json
- One state directory per repo: state\<repoKey>\ (runtime.json + runs), keyed by
  the repo's Git common dir, so every worktree of the same repo shares it.
- Run one service per project; they may run at the same time (locks are
  per-repository). A second service for the same repo is refused.
- A project's repository must be registered in Orca first.

Directory
---------
  bin\steerline.exe   the one binary that runs. Upgrade: build in any worktree,
                      then run  bin\steerline.exe install
  etc\<repo>.json      configs (run_dir may be omitted and is then derived)
  state\<repoKey>\     per-repo state and data
  backup\              old configs and runtime snapshots

Regenerate these launchers:  bin\steerline.exe launchers
