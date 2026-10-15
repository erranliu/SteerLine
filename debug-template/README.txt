SteerLine independent debug environment

start.bat                 starts shared backend and Web, default live
start.bat --simulate      freezes simulated worker/PR; real Git and SQLite
stop.bat                  stops both products and owned debug helpers
home/start.bat            restarts products with the already frozen scheme
reset-SteerlineTest.bat    resets only package execution state; stop first

First start freezes live/simulate. Generate a new package to switch.
The debug launcher remains open to own Git/provider helpers.
Web restart: home/bin/steerline-web.exe stop, then serve and open.
All data belongs to this package. Never use C:\SteerlineRuntime for debug.
