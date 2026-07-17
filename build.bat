@echo off
setlocal
cd /d "%~dp0"

set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

:: 解析 rsrc 路径（优先已安装的 %GOBIN%/%GOPATH%\bin\rsrc.exe）
where rsrc >nul 2>nul && set RSRCCMD=rsrc
if not defined RSRCCMD (
    if exist "%~dp0tools\rsrc.exe" (set RSRCCMD="%~dp0tools\rsrc.exe") else (
        echo [setup] 安装 rsrc ...
        go install github.com/akavel/rsrc@latest
        for /f "tokens=*" %%i in ('go env GOPATH') do set GOBIN=%%i\bin
        set RSRCCMD="%GOBIN%\rsrc.exe"
    )
)

echo [1/3] 生成清单资源 rsrc.syso ...
%RSRCCMD% -manifest rsrc/app.manifest -o rsrc.syso
if errorlevel 1 goto :err

echo [2/3] 构建单 exe ...
go build -ldflags="-s -w -H windowsgui" -o smonitor.exe
if errorlevel 1 goto :err

echo [3/3] 完成。产物: smonitor.exe
for %%A in (smonitor.exe) do echo   大小: %%~zA 字节
goto :eof

:err
echo 构建失败。
exit /b 1
