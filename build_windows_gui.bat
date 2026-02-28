@echo off
setlocal
cd /d "%~dp0"

echo Building namebench Windows GUI executable...
go mod tidy
if errorlevel 1 (
  echo Build failed during dependency resolution.
  pause
  exit /b 1
)

go build -ldflags="-H=windowsgui" -o namebench.exe .
if errorlevel 1 (
  echo Build failed.
  pause
  exit /b 1
)

echo Build complete: %CD%\namebench.exe
echo Launching namebench...
start "" "%CD%\namebench.exe"
exit /b 0
