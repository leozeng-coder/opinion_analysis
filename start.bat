@echo off
setlocal
REM Do not use chcp 65001 or non-ASCII here (GBK on CN Windows).

title opinion-analysis launcher
cd /d "%~dp0"

if exist "C:\Program Files\Go\bin\go.exe"     set "PATH=C:\Program Files\Go\bin;%PATH%"
if exist "C:\Program Files\nodejs\node.exe"   set "PATH=C:\Program Files\nodejs;%PATH%"

echo Starting backend  :8080 ...
start "Backend  :8080"   /D "%~dp0backend"        cmd /k call "%~dp0scripts\run-backend.cmd"

if "%SKIP_RAG%"=="1" (
    echo [launcher] SKIP_RAG=1 - not starting RAG embedding service (Milvus Lite).
) else (
    echo Starting RAG      :5055 ^(set SKIP_RAG=1 to disable^) ...
    start "RAG      :5055" /D "%~dp0"                cmd /k call "%~dp0scripts\run-rag-service.cmd"
)

echo Starting frontend :5173 ...
start "Frontend :5173"   /D "%~dp0frontend"        cmd /k call "%~dp0scripts\run-frontend.cmd"

echo Starting admin    :5174 ...
start "Admin    :5174"   /D "%~dp0frontend-admin"  cmd /k call "%~dp0scripts\run-admin.cmd"

echo Starting crawler  ...
start "Crawler"          /D "%~dp0"                cmd /k call "%~dp0scripts\run-crawler.cmd"

timeout /t 6 /nobreak >nul
start "" "http://localhost:5173"
start "" "http://localhost:5174"

echo.
echo   Frontend : http://localhost:5173
echo   Admin    : http://localhost:5174
echo   API      : http://localhost:8080
if not "%SKIP_RAG%"=="1" echo   RAG embed: http://localhost:5055 ^(backend: rag.enabled in config.yaml^)
echo.
pause
endlocal
