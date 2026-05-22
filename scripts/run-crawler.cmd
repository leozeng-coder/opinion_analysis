@echo off
setlocal EnableExtensions
REM ASCII-only for Chinese Windows CMD.

title opinion-crawler

if not defined CRAWLER_DB_HOST set "CRAWLER_DB_HOST=127.0.0.1"
if not defined CRAWLER_DB_PORT set "CRAWLER_DB_PORT=3306"
if not defined CRAWLER_DB_USER set "CRAWLER_DB_USER=root"
if not defined CRAWLER_DB_PASSWORD set "CRAWLER_DB_PASSWORD=123456"
if not defined CRAWLER_DB_NAME set "CRAWLER_DB_NAME=opinion_analysis"

for %%I in ("%~dp0..\crawler") do set "CRAWLER_ROOT=%%~fI"
cd /d "%CRAWLER_ROOT%"
if errorlevel 1 goto ERR_CD

if not exist "scheduler.py" goto ERR_SCHED

set "PYRUN="
py -3.11 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.11"
if not defined PYRUN py -3.12 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.12"
if not defined PYRUN py -3.13 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.13"
if defined PYRUN goto HAVE_PYRUN
where python >nul 2>&1
if errorlevel 1 goto ERR_PYTHON
set "PYRUN=python"
:HAVE_PYRUN

echo [crawler] cwd=%CD%
echo [crawler] Python: %PYRUN%
%PYRUN% --version

if exist ".venv\Scripts\python.exe" goto HAVE_VENV
echo [crawler] creating .venv ...
%PYRUN% -m venv .venv
if errorlevel 1 goto END
:HAVE_VENV

call "%CRAWLER_ROOT%\.venv\Scripts\activate.bat"
echo [crawler] pip sync ...
set "PYTHONUTF8=1"
if not defined PIP_INDEX_URL set "PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple"
if not defined PIP_TRUSTED_HOST set "PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn"
if "%PIP_USE_OFFICIAL%"=="1" goto PIP_OFFICIAL
python -m pip install -q --upgrade --no-cache-dir --default-timeout=120 -i "%PIP_INDEX_URL%" --trusted-host "%PIP_TRUSTED_HOST%" pip
pip install -q --upgrade --no-cache-dir --default-timeout=120 -i "%PIP_INDEX_URL%" --trusted-host "%PIP_TRUSTED_HOST%" -r requirements.txt
goto PIP_DONE
:PIP_OFFICIAL
python -m pip install -q --upgrade --no-cache-dir --default-timeout=120 pip
pip install -q --upgrade --no-cache-dir --default-timeout=120 -r requirements.txt
:PIP_DONE
if errorlevel 1 goto ERR_PIP

echo [crawler] starting scheduler (Ctrl+C to stop)
python scheduler.py
echo [crawler] exit %ERRORLEVEL%
goto END

:ERR_CD
echo [ERROR] Cannot cd to crawler: %CRAWLER_ROOT%
goto END

:ERR_SCHED
echo [ERROR] crawler\scheduler.py not found
goto END

:ERR_PYTHON
echo [ERROR] No Python 3.11-3.13. Install Python or add py/python to PATH.
goto END

:ERR_PIP
echo [ERROR] pip install failed
goto END

:END
echo.
pause
endlocal
