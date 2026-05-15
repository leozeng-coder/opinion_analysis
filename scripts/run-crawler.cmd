@echo off
setlocal
title opinion-crawler

REM GORM-style DSN (same as backend config database.dsn, query string optional).
REM Override before run: set DATABASE_DSN=user:pass@tcp(127.0.0.1:3306)/opinion_analysis
if not defined DATABASE_DSN (
    set "DATABASE_DSN=root:123456@tcp(127.0.0.1:3306)/opinion_analysis"
)

cd /d "%~dp0..\crawler"
if not exist "scheduler.py" (
    echo [ERROR] crawler\scheduler.py not found.
    goto END
)

REM Prefer stable Python (Scrapy/twisted/sqlalchemy): try py -3.13, then 3.12, 3.11, else plain python.
set "PYRUN="
py -3.13 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.13"
if not defined PYRUN py -3.12 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.12"
if not defined PYRUN py -3.11 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.11"
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
echo [crawler] DATABASE_DSN is set (host/db must match MySQL).
echo [crawler] If you changed Python version, delete folder .venv once then rerun.

if not exist ".venv\Scripts\python.exe" (
    echo [crawler] creating .venv ...
    %PYRUN% -m venv .venv
    if errorlevel 1 goto END
)

call .venv\Scripts\activate.bat
echo [crawler] pip sync (upgrades SQLAlchemy etc. after git pull) ...
REM --no-cache-dir: avoid corrupt pip cache after Python upgrade.
REM Default: Tsinghua mirror + 120s timeout (files.pythonhosted.org often times out from CN).
REM Use official PyPI: set PIP_USE_OFFICIAL=1 before running this script.
REM Custom mirror: set PIP_INDEX_URL=... and PIP_TRUSTED_HOST=hostname
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
