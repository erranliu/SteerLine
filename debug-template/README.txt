SteerLine independent debug environment

start.bat                  default live: real Codex and SteerlineTest GitHub PR
console.bat                execution console
web.bat                    browser UI (generate with -WebAddress; start the package first)
start.bat --simulate       AI-only simulated worker/PR, real Git and SQLite
console.bat --simulate     select the same simulation scheme
reset-SteerlineTest.bat    reset task state, no backup, preserves Git/PR facts
reset-SteerlineTest.bat --check --no-pause

First run freezes live/simulate for this package. Generate a new package to switch.
Ctrl+C in start.bat requests service shutdown and stops owned helper processes.
Close the service, workers and consoles before reset. No process is forcibly stopped by reset.
Generation is local; first live start copies authentication and pushes the unique fixture base.
All runtime data belongs to this directory; never use C:\SteerlineRuntime for debug.

start-app.bat / stop-app.bat    independent application gateway
start-web.bat / stop-web.bat    independent Web, preserving project services
Web restart: stop-web.bat, then start-web.bat, then web.bat to authorize again.
