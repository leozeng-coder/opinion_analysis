@echo off
setlocal EnableExtensions
REM ASCII-only: avoid UTF-8 in .cmd on Chinese Windows CMD (mangles lines).

title RAG service :5055

for %%I in ("%~dp0..\rag") do set "RAG_ROOT=%%~fI"
cd /d "%RAG_ROOT%"
if errorlevel 1 goto ERR_RAG_CD

if not exist "server.py" goto ERR_NO_SERVER

set "PYRUN="
py -3.11 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.11"
if not defined PYRUN py -3.12 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.12"
if not defined PYRUN py -3.13 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.13"
if defined PYRUN goto HAVE_PYRUN
where python >nul 2>&1
if errorlevel 1 goto ERR_PYTHON
set "PYRUN=python"
:HAVE_PYRUN

echo [rag] dir=%CD%
echo [rag] Python: %PYRUN%
%PYRUN% --version

if exist "%RAG_ROOT%\.venv\Scripts\python.exe" goto HAVE_VENV
echo [rag] creating rag\.venv ...
%PYRUN% -m venv "%RAG_ROOT%\.venv"
if errorlevel 1 goto END
:HAVE_VENV

set "VPY=%RAG_ROOT%\.venv\Scripts\python.exe"
if not exist "%VPY%" goto ERR_NO_VENV

"%VPY%" -m pip --version >nul 2>&1
if errorlevel 1 goto RECREATE_VENV

set "PYTHONUTF8=1"
"%VPY%" -c "import pymilvus, milvus_lite" >nul 2>&1
if not errorlevel 1 goto DEPS_OK

echo [rag] pip installing rag deps (slow first time) ...
echo [rag] upgrading pip first ...
"%VPY%" -m pip install --upgrade pip
if errorlevel 1 goto RECREATE_VENV

if "%PIP_USE_OFFICIAL%"=="1" goto PIP_OFFICIAL
if not defined PIP_INDEX_URL set "PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple"
if not defined PIP_TRUSTED_HOST set "PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn"
"%VPY%" -m pip install --no-cache-dir --default-timeout=300 -i "%PIP_INDEX_URL%" --trusted-host "%PIP_TRUSTED_HOST%" -r requirements.txt
if not errorlevel 1 goto VERIFY_DEPS
echo [rag] mirror install failed, retrying official PyPI ...
:PIP_OFFICIAL
"%VPY%" -m pip install --no-cache-dir --default-timeout=300 -r requirements.txt
if errorlevel 1 goto PIP_FAILED
:VERIFY_DEPS
"%VPY%" -c "import pymilvus, milvus_lite" >nul 2>&1
if errorlevel 1 goto PIP_FAILED
goto DEPS_OK

:PIP_FAILED
echo [ERROR] pip install failed and RAG deps are not importable.
echo [HINT] Close this window, delete folder: %RAG_ROOT%\.venv
echo [HINT] Then re-run start.bat or: scripts\fix-rag-venv.cmd
goto END

:RECREATE_VENV
echo [rag] venv or pip is broken; recreating rag\.venv ...
if exist "%RAG_ROOT%\.venv" rd /s /q "%RAG_ROOT%\.venv"
%PYRUN% -m venv "%RAG_ROOT%\.venv"
if errorlevel 1 goto END
set "VPY=%RAG_ROOT%\.venv\Scripts\python.exe"
"%VPY%" -m pip install --upgrade pip
if "%PIP_USE_OFFICIAL%"=="1" goto PIP_OFFICIAL
if not defined PIP_INDEX_URL set "PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple"
if not defined PIP_TRUSTED_HOST set "PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn"
"%VPY%" -m pip install --no-cache-dir --default-timeout=300 -i "%PIP_INDEX_URL%" --trusted-host "%PIP_TRUSTED_HOST%" -r requirements.txt
if errorlevel 1 goto PIP_OFFICIAL
"%VPY%" -c "import pymilvus, milvus_lite" >nul 2>&1
if errorlevel 1 goto PIP_FAILED
goto DEPS_OK

:DEPS_OK

if not defined RAG_DB_HOST set "RAG_DB_HOST=127.0.0.1"
if not defined RAG_DB_PORT set "RAG_DB_PORT=3306"
if not defined RAG_DB_USER set "RAG_DB_USER=root"
if not defined RAG_DB_PASSWORD set "RAG_DB_PASSWORD=123456"
if not defined RAG_DB_NAME set "RAG_DB_NAME=opinion_analysis"

echo [rag] http://127.0.0.1:5055 - see backend config rag.embedding_service_url
"%VPY%" "%RAG_ROOT%\server.py"
goto END

:ERR_RAG_CD
echo [ERROR] Cannot cd to rag: %RAG_ROOT%
goto END

:ERR_NO_SERVER
echo [ERROR] Missing rag\server.py
goto END

:ERR_PYTHON
echo [ERROR] No Python 3.11-3.13. Install Python or add py/python to PATH.
goto END

:ERR_NO_VENV
echo [ERROR] venv python missing: %VPY%
goto END

:END
echo.
pause
endlocal
