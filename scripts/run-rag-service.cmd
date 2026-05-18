@echo off
setlocal EnableExtensions
REM ASCII-only: avoid UTF-8 in .cmd on Chinese Windows CMD (mangles lines).

title RAG service :5055

for %%I in ("%~dp0..\crawler") do set "CRAWLER_ROOT=%%~fI"
cd /d "%CRAWLER_ROOT%"
if errorlevel 1 goto ERR_CRAWLER_CD

if not exist "rag_service\server.py" goto ERR_NO_SERVER

set "PYRUN="
py -3.11 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.11"
if not defined PYRUN py -3.12 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.12"
if not defined PYRUN py -3.13 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.13"
if defined PYRUN goto HAVE_PYRUN
where python >nul 2>&1
if errorlevel 1 goto ERR_PYTHON
set "PYRUN=python"
:HAVE_PYRUN

echo [rag] crawler dir=%CD%
echo [rag] Python: %PYRUN%
%PYRUN% --version

if exist "%CRAWLER_ROOT%\.venv\Scripts\python.exe" goto HAVE_VENV
echo [rag] creating crawler\.venv ...
%PYRUN% -m venv "%CRAWLER_ROOT%\.venv"
if errorlevel 1 goto END
:HAVE_VENV

set "VPY=%CRAWLER_ROOT%\.venv\Scripts\python.exe"
if not exist "%VPY%" goto ERR_NO_VENV

for %%I in ("%~dp0..\crawler\rag_service") do set "RAG_DIR=%%~fI"
cd /d "%RAG_DIR%"
if errorlevel 1 goto ERR_RAG_CD

"%VPY%" -c "import pymilvus, milvus_lite" >nul 2>&1
if not errorlevel 1 goto DEPS_OK
echo [rag] pip installing rag deps (slow first time) ...
if not defined PIP_INDEX_URL set "PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple"
if not defined PIP_TRUSTED_HOST set "PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn"
"%VPY%" -m pip install -q --no-cache-dir --default-timeout=120 -i "%PIP_INDEX_URL%" --trusted-host "%PIP_TRUSTED_HOST%" -r requirements-rag.txt
if errorlevel 1 goto ERR_PIP
:DEPS_OK

if not defined CRAWLER_DB_HOST set "CRAWLER_DB_HOST=127.0.0.1"
if not defined CRAWLER_DB_PORT set "CRAWLER_DB_PORT=3306"
if not defined CRAWLER_DB_USER set "CRAWLER_DB_USER=root"
if not defined CRAWLER_DB_PASSWORD set "CRAWLER_DB_PASSWORD=123456"
if not defined CRAWLER_DB_NAME set "CRAWLER_DB_NAME=opinion_analysis"

echo [rag] http://127.0.0.1:5055 - see backend config rag.embedding_service_url
"%VPY%" "%RAG_DIR%\server.py"
goto END

:ERR_CRAWLER_CD
echo [ERROR] Cannot cd to crawler: %CRAWLER_ROOT%
goto END

:ERR_NO_SERVER
echo [ERROR] Missing crawler\rag_service\server.py
goto END

:ERR_PYTHON
echo [ERROR] No Python 3.11-3.13. Install Python or add py/python to PATH.
goto END

:ERR_NO_VENV
echo [ERROR] venv python missing: %VPY%
goto END

:ERR_RAG_CD
echo [ERROR] Cannot cd to rag_service: %RAG_DIR%
goto END

:ERR_PIP
echo [ERROR] pip install failed. Try official PyPI or set PIP_INDEX_URL.
goto END

:END
echo.
pause
endlocal
