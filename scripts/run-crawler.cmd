@echo off
setlocal EnableExtensions
REM ASCII-only: avoid UTF-8 in .cmd on Chinese Windows CMD (mangles lines).

title MediaCrawler API :8085

for %%I in ("%~dp0..\MediaCrawler") do set "MC_ROOT=%%~fI"
cd /d "%MC_ROOT%"
if errorlevel 1 goto ERR_CD

if not exist "api\main.py" goto ERR_NO_APP

set "VPY=%MC_ROOT%\.venv\Scripts\python.exe"
if exist "%VPY%" goto HAVE_VENV

echo [crawler] .venv not found, creating with uv sync ...
where uv >nul 2>&1
if errorlevel 1 goto ERR_NO_UV
uv sync
if errorlevel 1 goto ERR_SYNC
:HAVE_VENV

if not exist "%VPY%" goto ERR_NO_VENV

set "PYTHONUTF8=1"
echo [crawler] dir=%CD%
echo [crawler] http://127.0.0.1:8085 - proxied via Go backend
"%VPY%" -m uvicorn api.main:app --host 0.0.0.0 --port 8085
goto END

:ERR_CD
echo [ERROR] Cannot cd to MediaCrawler: %MC_ROOT%
goto END

:ERR_NO_APP
echo [ERROR] Missing MediaCrawler\api\main.py
goto END

:ERR_NO_UV
echo [ERROR] .venv missing and uv not found. Install uv or create .venv manually.
goto END

:ERR_SYNC
echo [ERROR] uv sync failed.
goto END

:ERR_NO_VENV
echo [ERROR] venv python missing: %VPY%
goto END

:END
echo.
pause
endlocal
