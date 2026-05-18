@echo off
setlocal
title opinion-crawler

REM crawler config: set DB credentials before running
REM These must match backend\config\config.yaml DSN values
if not defined CRAWLER_DB_HOST set "CRAWLER_DB_HOST=127.0.0.1"
if not defined CRAWLER_DB_PORT set "CRAWLER_DB_PORT=3306"
if not defined CRAWLER_DB_USER set "CRAWLER_DB_USER=root"
if not defined CRAWLER_DB_PASSWORD set "CRAWLER_DB_PASSWORD=123456"
if not defined CRAWLER_DB_NAME set "CRAWLER_DB_NAME=opinion_analysis"

cd /d "%~dp0..\crawler"
if not exist "scheduler.py" (
    echo [ERROR] crawler\scheduler.py not found. Ensure you are running from the repo root.
    goto END
)

REM Prefer 3.11 (project default), then 3.12/3.13; Py3.14+ may lack wheels for some pins.
set "PYRUN="
py -3.11 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.11"
if not defined PYRUN py -3.12 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.12"
if not defined PYRUN py -3.13 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.13"
if not defined PYRUN (
    where python >nul 2>&1
    if errorlevel 1 (
        echo [ERROR] No usable Python. Install 3.11-3.13 or add python/py launcher to PATH.
        goto END
    )
    set "PYRUN=python"
)

echo [crawler] cwd=%CD%
echo [crawler] Python launcher: %PYRUN%
%PYRUN% --version

if not exist ".venv\Scripts\python.exe" (
    echo [crawler] creating .venv ...
    %PYRUN% -m venv .venv
    if errorlevel 1 goto END
)

call .venv\Scripts\activate.bat
echo [crawler] pip sync ...
REM Default: Tsinghua mirror + 120s timeout (files.pythonhosted.org often times out from CN).
REM Use official PyPI: set PIP_USE_OFFICIAL=1 before running this script.
if not defined PIP_INDEX_URL set "PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple"
if not defined PIP_TRUSTED_HOST set "PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn"
if "%PIP_USE_OFFICIAL%"=="1" (
    echo [crawler] pip: official PyPI, timeout 120s
    python -m pip install -q --upgrade --no-cache-dir --default-timeout=120 pip
    pip install -q --upgrade --no-cache-dir --default-timeout=120 -r requirements.txt
) else (
    echo [crawler] pip: %PIP_INDEX_URL% ^(set PIP_USE_OFFICIAL=1 for pypi.org^)
    python -m pip install -q --upgrade --no-cache-dir --default-timeout=120 -i "%PIP_INDEX_URL%" --trusted-host "%PIP_TRUSTED_HOST%" pip
    pip install -q --upgrade --no-cache-dir --default-timeout=120 -i "%PIP_INDEX_URL%" --trusted-host "%PIP_TRUSTED_HOST%" -r requirements.txt
)
if errorlevel 1 (
    echo [ERROR] pip install failed.
    goto END
)

echo [crawler] starting scheduler (Ctrl+C to stop) ...
python scheduler.py
echo [crawler] exited with code %ERRORLEVEL%

:END
echo.
pause
endlocal
