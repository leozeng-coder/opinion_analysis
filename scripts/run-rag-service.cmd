@echo off
setlocal
title RAG service Milvus Lite :5055

cd /d "%~dp0..\crawler\rag_service"

if exist "C:\Program Files\nodejs\node.exe" set "PATH=C:\Program Files\nodejs;%PATH%"

set "PYRUN="
py -3.11 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.11"
if not defined PYRUN py -3.12 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.12"
if not defined PYRUN py -3.13 -c "import sys" >nul 2>&1 && set "PYRUN=py -3.13"
if not defined PYRUN set "PYRUN=python"

if not exist "..\.venv\Scripts\python.exe" (
  echo [ERROR] crawler\.venv not found. Run scripts\run-crawler.cmd once first.
  goto END
)

REM 复用 crawler venv，但 rag 依赖可能未装
call ..\.venv\Scripts\activate.bat
where python
python -c "import pymilvus, milvus_lite" >nul 2>&1
if errorlevel 1 (
  echo [rag] installing rag_service deps ^(may download torch / milvus-lite^) ...
  pip install -q -r requirements-rag.txt -i https://pypi.tuna.tsinghua.edu.cn/simple --trusted-host pypi.tuna.tsinghua.edu.cn
  if errorlevel 1 goto END
)

REM 与 run-crawler.cmd 一致的数据库环境（按需改）
if not defined CRAWLER_DB_HOST set "CRAWLER_DB_HOST=127.0.0.1"
if not defined CRAWLER_DB_PORT set "CRAWLER_DB_PORT=3306"
if not defined CRAWLER_DB_USER set "CRAWLER_DB_USER=root"
if not defined CRAWLER_DB_PASSWORD set "CRAWLER_DB_PASSWORD=123456"
if not defined CRAWLER_DB_NAME set "CRAWLER_DB_NAME=opinion_analysis"

echo [rag] starting http://127.0.0.1:5055 ...
python server.py

:END
echo.
pause
endlocal
