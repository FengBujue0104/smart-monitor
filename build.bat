@echo off
setlocal EnableExtensions
cd /d "%~dp0"

set "GOOS=windows"
set "GOARCH=amd64"
set "CGO_ENABLED=0"

set "RSRCCMD="
where rsrc >nul 2>nul
if not errorlevel 1 set "RSRCCMD=rsrc"

if defined RSRCCMD goto :rsrc_ready

for /f "delims=" %%i in ('go env GOPATH') do set "GOPATHDIR=%%i"
if exist "%GOPATHDIR%\bin\rsrc.exe" set "RSRCCMD=%GOPATHDIR%\bin\rsrc.exe"
if defined RSRCCMD goto :rsrc_ready

if exist "%~dp0tools\rsrc.exe" set "RSRCCMD=%~dp0tools\rsrc.exe"
if defined RSRCCMD goto :rsrc_ready

echo [setup] Installing rsrc...
go install github.com/akavel/rsrc@latest
if errorlevel 1 goto :err
if not exist "%GOPATHDIR%\bin\rsrc.exe" goto :err
set "RSRCCMD=%GOPATHDIR%\bin\rsrc.exe"

:rsrc_ready
echo [1/3] Generating manifest resource...
"%RSRCCMD%" -manifest rsrc/app.manifest -o rsrc.syso
if errorlevel 1 goto :err

echo [2/3] Building smonitor.exe...
go build -ldflags="-s -w -H windowsgui" -o smonitor.exe
if errorlevel 1 goto :err

for %%A in (smonitor.exe) do echo [3/3] Done: %%~zA bytes
exit /b 0

:err
echo Build failed.
exit /b 1
