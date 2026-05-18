@echo off
setlocal
title RAG embedding service Milvus Lite :5055

cd /d "%~dp0..\crawler"
if not exist "rag_service\server.py" (
  echo [ERROR] crawler\rag_service\server.py not found.
  goto END
)

REM Prefer 3.11, then 3.12 / 3.13（与 run-crawler 一致）
set "PYRUN="
py -3.11 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.11"
if not defined PYRUN py -3.12 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.12"
if not defined PYRUN py -3.13 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.13"
if not defined PYRUN (
  where python >nul 2>&1
  if errorlevel 1 (
    echo [ERROR] No usable Python. Install 3.11-3.13 or add python/py to PATH.
    goto END
  )
  set "PYRUN=python"
)

echo [rag] crawler dir=%CD%
echo [rag] Python: %PYRUN%
%PYRUN% --version

if not exist ".venv\Scripts\python.exe" (
  echo [rag] creating crawler\.venv ^(needed for rag_service^) ...
  %PYRUN% -m venv .venv
  if errorlevel 1 goto END
)

call .venv\Scripts\activate.bat
where python

cd /d "%~dp0..\crawler\rag_service"

python -c "import pymilvus, milvus_lite" >nul 2>&1
if errorlevel 1 (
  echo [rag] installing rag_service deps ^(sentence-transformers / torch / milvus-lite — may take a while^) ...
  if not defined PIP_INDEX_URL set "PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple"
  if not defined PIP_TRUSTED_HOST set "PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn"
  pip install -q --no-cache-dir --default-timeout=120 -i "%PIP_INDEX_URL%" --trusted-host "%PIP_TRUSTED_HOST%" -r requirements-rag.txt
  if errorlevel 1 (
    echo [ERROR] rag pip install failed. Try PIP_INDEX_URL official or VPN.
    goto END
  )
)

REM 与 run-crawler 一致；可在系统环境变量中覆盖
if not defined CRAWLER_DB_HOST set "CRAWLER_DB_HOST=127.0.0.1"
if not defined CRAWLER_DB_PORT set "CRAWLER_DB_PORT=3306"
if not defined CRAWLER_DB_USER set "CRAWLER_DB_USER=root"
if not defined CRAWLER_DB_PASSWORD set "CRAWLER_DB_PASSWORD=123456"
if not defined CRAWLER_DB_NAME set "CRAWLER_DB_NAME=opinion_analysis"

echo [rag] listening http://127.0.0.1:5055 ^(set RAG_PORT to override default^)
echo [rag] enable RAG in backend\config\config.yaml: rag.enabled: true ; embedding_service_url: "http://127.0.0.1:5055"
python server.py

:END
echo.
pause
endlocal
