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
) else (
    echo Starting RAG :5055 - set SKIP_RAG=1 to skip
    start "opinion-rag-5055" cmd.exe /k call "%~dp0scripts\run-rag-service.cmd"
)

echo Starting frontend :5173 ...
start "opinion-front-5173" cmd.exe /k call "%~dp0scripts\run-frontend.cmd"

echo Starting admin :5174 ...
start "opinion-admin-5174" cmd.exe /k call "%~dp0scripts\run-admin.cmd"

echo Starting crawler ...
start "opinion-crawler" cmd.exe /k call "%~dp0scripts\run-crawler.cmd"

timeout /t 6 /nobreak >nul
start "" "http://localhost:5173"
start "" "http://localhost:5174"

echo.
echo   Frontend : http://localhost:5173
echo   Admin    : http://localhost:5174
echo   API      : http://localhost:8080
if not "%SKIP_RAG%"=="1" echo   RAG      : http://127.0.0.1:5055
echo.
pause
endlocal
