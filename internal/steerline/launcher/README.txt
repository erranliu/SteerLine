SteerLine home (STEERLINE_HOME)
==============================

Double-click entries
--------------------
  projects.bat   pick a project and an action (viewer / run / stop / home)
  viewer.bat     viewer for one project (default etc\merge-h-308-code.json)
  run.bat        run one project's scheduler in this window (Ctrl+C to stop)
  stop.bat       ask one project's service to stop
  home.bat       print the resolved paths

Any launcher accepts a config path, e.g.  viewer.bat etc\other-repo.json

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
