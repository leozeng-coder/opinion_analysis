@echo off
setlocal EnableExtensions
REM Rebuild rag\.venv (close opinion-rag-5055 window first).

for %%I in ("%~dp0..\rag") do set "RAG_ROOT=%%~fI"

echo [fix-rag] Close the RAG CMD window before continuing.
pause

if exist "%RAG_ROOT%\.venv" (
    echo [fix-rag] removing %RAG_ROOT%\.venv ...
    rd /s /q "%RAG_ROOT%\.venv"
)

call "%~dp0run-rag-service.cmd"
endlocal
