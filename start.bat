@echo off
setlocal
REM Do not use chcp 65001 or non-ASCII here: CMD parses this file as system ANSI (GBK on CN Windows).
REM UTF-8 .bat without BOM breaks lines; UTF-8 Chinese is misread as garbage commands.
REM Optional: set MYSQL_BIN to full path of mysql.exe before running this script, e.g.
REM   set MYSQL_BIN=C:\Program Files\MySQL\MySQL Server 8.0\bin\mysql.exe

title opinion-analysis dev launcher

cd /d "%~dp0"
if not exist "backend\go.mod" (
    echo [ERROR] backend\go.mod not found. Run this .bat from the repo root.
    goto :ERR
)

REM Explorer-launched CMD sometimes misses User PATH; prepend common install dirs.
if exist "C:\Program Files\Go\bin\go.exe" set "PATH=C:\Program Files\Go\bin;%PATH%"
if exist "C:\Program Files\nodejs\node.exe" set "PATH=C:\Program Files\nodejs;%PATH%"

echo ==========================================
echo   opinion-analysis - one-click dev start
echo ==========================================
echo Working dir: %CD%
echo.

where go >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go not found in PATH.
    goto :ERR
)
where node >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Node.js not found in PATH.
    goto :ERR
)
where npm >nul 2>&1
if errorlevel 1 (
    echo [ERROR] npm not found in PATH.
    goto :ERR
)

echo [1/5] MySQL (optional)...
where mysql >nul 2>&1
if errorlevel 1 (
    echo       mysql.exe not found; skipped. Ensure MySQL matches backend\config\config.yaml DSN.
) else (
    mysql -u root -p123456 -e "SELECT 1;" >nul 2>&1
    if errorlevel 1 (
        echo [WARN] Cannot connect as root/123456; continuing. Fix credentials or start MySQL/docker-compose if backend fails.
    ) else (
        mysql -u root -p123456 -e "CREATE DATABASE IF NOT EXISTS opinion_analysis CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" >nul 2>&1
        echo       MySQL OK; database opinion_analysis checked.
    )
)
echo.

echo [2/5] Frontend deps...
if not exist "frontend\node_modules" (
    echo       Running npm install ...
    pushd frontend
    call npm install
    if errorlevel 1 (
        popd
        echo [ERROR] npm install failed.
        goto :ERR
    )
    popd
) else (
    echo       node_modules exists; skip install.
)
echo.

echo [3/5] Starting backend :8080 (new window)...
if not exist "%~dp0scripts\run-backend.cmd" (
    echo [ERROR] scripts\run-backend.cmd missing.
    goto :ERR
)
start "Backend :8080" /D "%~dp0backend" cmd /k call "%~dp0scripts\run-backend.cmd"

timeout /t 5 /nobreak >nul

echo [4/5] Starting frontend :5173 (new window)...
if not exist "%~dp0scripts\run-frontend.cmd" (
    echo [ERROR] scripts\run-frontend.cmd missing.
    goto :ERR
)
start "Frontend :5173" /D "%~dp0frontend" cmd /k call "%~dp0scripts\run-frontend.cmd"

timeout /t 2 /nobreak >nul

echo [5/5] Starting MindSpider crawler (new window)...
if not exist "%~dp0scripts\run-crawler.cmd" (
    echo [WARN] scripts\run-crawler.cmd missing; skip crawler.
) else (
    start "MindSpider Crawler" /D "%~dp0" cmd /k call "%~dp0scripts\run-crawler.cmd"
)

timeout /t 4 /nobreak >nul
start "" "http://localhost:5173"

echo.
echo ==========================================
echo   Launched
echo   Frontend: http://localhost:5173
echo   Backend:  http://localhost:8080
echo   Crawler:  MindSpider scheduler (see Crawler window)
echo ==========================================
echo Closing this window does not stop servers; use the other windows.
echo.
pause
exit /b 0

:ERR
echo.
pause
exit /b 1
