@echo off
setlocal
cd /d "%~dp0"

set GOEXE=go
if exist ".tools\go\bin\go.exe" set GOEXE=.tools\go\bin\go.exe

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

echo Build complete: %CD%\namebench.exe
echo Launching namebench...
start "" "%CD%\namebench.exe"
exit /b 0
