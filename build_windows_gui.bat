@echo off
setlocal
cd /d "%~dp0"

set GOEXE=go
if exist ".tools\go\bin\go.exe" set GOEXE=.tools\go\bin\go.exe

if exist "namebench.exe~" del /q "namebench.exe~" >nul 2>nul

echo Building namebench Windows GUI executable...
%GOEXE% mod tidy
if errorlevel 1 (
  echo Build failed during dependency resolution.
  pause
  exit /b 1
)

%GOEXE% build -ldflags="-H=windowsgui" -o namebench.exe .
if errorlevel 1 (
  echo Build failed.
  pause
  exit /b 1
)

if exist "namebench.exe~" (
  del /q "namebench.exe~" >nul 2>nul
  if exist "namebench.exe~" echo Note: stale namebench.exe~ remains because an older process is still holding the previous binary.
)

echo Build complete: %CD%\namebench.exe
echo Launching namebench...
start "" "%CD%\namebench.exe"
exit /b 0
