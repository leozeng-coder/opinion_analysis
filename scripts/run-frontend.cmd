@echo off
setlocal
title Frontend :5173

if exist "C:\Program Files\nodejs\node.exe" set "PATH=C:\Program Files\nodejs;%PATH%"

where node >nul 2>&1
if errorlevel 1 (
    echo [ERROR] node not found. Install Node.js or add it to PATH.
    goto :END
)

if not exist "node_modules" (
    echo [frontend] installing deps ...
    call npm install
    if errorlevel 1 goto :END
)

echo [frontend] starting on :5173 ...
call npm run dev

:END
echo.
pause
endlocal
