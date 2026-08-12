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
echo [1/4] Generating manifest resource...
"%RSRCCMD%" -manifest rsrc/app.manifest -o rsrc.syso
if errorlevel 1 goto :err

echo [2/4] Building smonitor.exe...
go build -ldflags="-s -w -H windowsgui" -o smonitor.exe
if errorlevel 1 goto :err

echo [3/4] Signing...
call :sign
if errorlevel 1 echo   [sign] skipped: no code-signing certificate found.

rem Remove the intermediate resource object. smonitor.exe already embeds the
rem manifest, and a leftover rsrc.syso makes every later `go test .` link the
rem requireAdministrator manifest into the test binary (UAC elevation prompt).
if exist "%~dp0rsrc.syso" del /q "%~dp0rsrc.syso"

for %%A in (smonitor.exe) do echo [4/4] Done: %%~zA bytes
exit /b 0

:err
if exist "%~dp0rsrc.syso" del /q "%~dp0rsrc.syso"
echo Build failed.
exit /b 1

rem ============================================================
rem Code signing (idempotent: silently skipped when no cert).
rem Sources, in priority order:
rem   1) Env var SMONITOR_PFX -> .pfx/.p12 file; optional
rem      SMONITOR_PFX_PASSWORD supplies the password.
rem   2) First Code Signing cert in the store (CurrentUser first).
rem NOTE: keep this file ASCII-only; cmd parses batch files in
rem the ANSI codepage and non-ASCII text corrupts parsing.
rem ============================================================
:sign
set "SIGNTOOL="
rem Preferred exact versions first, then fall back to the newest installed
rem Windows SDK kit, then PATH.
if exist "C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\signtool.exe" set "SIGNTOOL=C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\signtool.exe"
if not defined SIGNTOOL if exist "C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\signtool.exe" set "SIGNTOOL=C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\signtool.exe"
if not defined SIGNTOOL for /f "delims=" %%K in ('dir /b /o-n "C:\Program Files (x86)\Windows Kits\10\bin\10.0.*" 2^>nul') do (
    if not defined SIGNTOOL if exist "C:\Program Files (x86)\Windows Kits\10\bin\%%K\x64\signtool.exe" set "SIGNTOOL=C:\Program Files (x86)\Windows Kits\10\bin\%%K\x64\signtool.exe"
)
if not defined SIGNTOOL for /f "delims=" %%S in ('where signtool 2^>nul') do if not defined SIGNTOOL set "SIGNTOOL=%%S"
if not defined SIGNTOOL (
    echo   [sign] signtool.exe not found; install Windows SDK.
    exit /b 1
)

if defined SMONITOR_PFX (
    if not exist "%SMONITOR_PFX%" (
        echo   [sign] SMONITOR_PFX set but file not found: "%SMONITOR_PFX%"
        exit /b 1
    )
    if defined SMONITOR_PFX_PASSWORD (
        "%SIGNTOOL%" sign /fd SHA256 /f "%SMONITOR_PFX%" /p "%SMONITOR_PFX_PASSWORD%" /tr "http://timestamp.digicert.com" /td SHA256 smonitor.exe
    ) else (
        "%SIGNTOOL%" sign /fd SHA256 /f "%SMONITOR_PFX%" /tr "http://timestamp.digicert.com" /td SHA256 smonitor.exe
    )
    if errorlevel 1 (
        echo   [sign] signing with pfx failed.
        exit /b 1
    )
    echo   [sign] signed with pfx "%SMONITOR_PFX%"
    exit /b 0
)

rem Store certs: CurrentUser first, then LocalMachine.
rem usebackq backticks allow quotes inside the powershell command.
set "CERT_TP="
for /f "usebackq delims=" %%T in (`powershell -NoProfile -Command "(Get-ChildItem 'Cert:\CurrentUser\My' -CodeSigningCert -ErrorAction SilentlyContinue | Sort-Object NotAfter -Descending | Select-Object -First 1).Thumbprint"`) do set "CERT_TP=%%T"
if defined CERT_TP goto :sign_store

for /f "usebackq delims=" %%T in (`powershell -NoProfile -Command "(Get-ChildItem 'Cert:\LocalMachine\My' -CodeSigningCert -ErrorAction SilentlyContinue | Sort-Object NotAfter -Descending | Select-Object -First 1).Thumbprint"`) do set "CERT_TP=%%T"

:sign_store
if defined CERT_TP (
    "%SIGNTOOL%" sign /fd SHA256 /sha1 "%CERT_TP%" /tr "http://timestamp.digicert.com" /td SHA256 smonitor.exe
    if errorlevel 1 (
        echo   [sign] signing with store certificate failed.
        exit /b 1
    )
    echo   [sign] signed with store certificate thumbprint %CERT_TP%
    exit /b 0
)

exit /b 1
