# SteerLine Windows UI

This is the WinUI 3 client foundation for the 2.1 task view. It connects only to an already running local SteerLine service through `steerline-client/v1`; the window never starts or stops the service. The initial submit button records a local intent and explicitly reports that no business command was sent.

## Fixed build target

- .NET SDK `8.0.416`, pinned by the repository `global.json`.
- Windows App SDK `1.8.260921001` (1.8.12), pinned in the app project.
- Windows target `net8.0-windows10.0.19041.0`; minimum Windows platform build `19041`.
- `win-x64`, unpackaged and self-contained. Build output carries the .NET and Windows App SDK runtime files, so it does not install or replace a machine-wide Windows App SDK runtime.
- Build and UI verification in this task used Windows 11 x64, version `10.0.26200` (25H2). The unpackaged app also requires the machine's Visual C++ runtime.

## Build and test

Run from the repository root:

```powershell
dotnet restore ui/SteerLine.Windows/SteerLine.Windows.csproj
dotnet build ui/SteerLine.Windows/SteerLine.Windows.csproj -c Debug
dotnet test ui/SteerLine.Windows.Tests/SteerLine.Windows.Tests.csproj -c Debug
```

The executable is written to:

```text
ui/SteerLine.Windows/bin/Debug/net8.0-windows10.0.19041.0/win-x64/SteerLine.Windows.exe
```

Launch the executable, select a project configuration under the chosen SteerLine home and the exact repository directory named by that configuration, then choose **连接现有服务**. The client verifies the discovered protocol, project, instance, process, credential and current-user ACL before refreshing tasks. It never falls back to cached tasks or starts a replacement service. Select a task explicitly before editing; the composer and local-intent button stay disabled until a target is selected.

For task-specific controlled service testing, first create an isolated package with `scripts/new-debug-release.ps1`, then start its generated `start.bat --simulate` entry. See [the independent debug environment guide](../../docs/runtime-install-debug.md). Do not point tests at `C:\SteerlineRuntime`.

The edit box is the native WinUI multiline `TextBox`; Enter inserts a line break. The separate intent button does not call a business-command route. Drafts are stored under the selected home at `ui/drafts.json`, keyed by the authenticated project ID and exact task ID.
