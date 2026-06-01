@echo off
setlocal EnableExtensions
REM ASCII only: UTF-8/emoji in .cmd breaks Chinese Windows CMD (CP936).

title opinion-analysis launcher
cd /d "%~dp0"

if exist "C:\Program Files\Go\bin\go.exe"     set "PATH=C:\Program Files\Go\bin;%PATH%"
if exist "C:\Program Files\nodejs\node.exe"   set "PATH=C:\Program Files\nodejs;%PATH%"

REM Do not use cmd /k "cd ... & call ..." here; ampersand breaks some CMD builds.
REM Each scripts\run-*.cmd cds using %%~fI from %%dp0.

echo Starting backend :8080 ...
start "opinion-backend-8080" cmd.exe /k call "%~dp0scripts\run-backend.cmd"

if "%SKIP_RAG%"=="1" (
    echo [launcher] SKIP_RAG=1 - RAG service not started.
    goto :AFTER_RAG
)
REM Check if Go backend manages RAG (auto_start: true in config.yaml).
REM If so, skip manual RAG launch to avoid port conflict.
findstr /C:"auto_start: true" "%~dp0backend\config\config.yaml" >nul 2>&1
if errorlevel 1 (
    echo Starting RAG :5055 - set SKIP_RAG=1 to skip
    start "opinion-rag-5055" cmd.exe /k call "%~dp0scripts\run-rag-service.cmd"
) else (
    echo [launcher] RAG managed by backend auto_start - skipping manual launch.
)
:AFTER_RAG

echo Starting frontend :5173 ...
start "opinion-front-5173" cmd.exe /k call "%~dp0scripts\run-frontend.cmd"

echo Starting admin :5174 ...
start "opinion-admin-5174" cmd.exe /k call "%~dp0scripts\run-admin.cmd"

if "%SKIP_CRAWLER%"=="1" (
    echo [launcher] SKIP_CRAWLER=1 - MediaCrawler API not started.
) else (
    echo Starting MediaCrawler API :8085 - set SKIP_CRAWLER=1 to skip
    start "opinion-crawler-8085" cmd.exe /k call "%~dp0scripts\run-crawler.cmd"
)

timeout /t 6 /nobreak >nul
start "" "http://localhost:5173"
start "" "http://localhost:5174"

echo.
echo   Frontend : http://localhost:5173
echo   Admin    : http://localhost:5174
echo   API      : http://localhost:8080
if not "%SKIP_RAG%"=="1" echo   RAG      : http://127.0.0.1:5055
if not "%SKIP_CRAWLER%"=="1" echo   Crawler  : http://127.0.0.1:8085
echo.
pause
endlocal
