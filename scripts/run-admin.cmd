@echo off
setlocal
title Admin Frontend :5174

if exist "C:\Program Files\nodejs\node.exe" set "PATH=C:\Program Files\nodejs;%PATH%"

where node >nul 2>&1
if errorlevel 1 (
    echo [ERROR] node not found. Install Node.js or add it to PATH.
    goto :END
)

if not exist "node_modules" (
    echo [admin] installing deps ...
    call npm install
    if errorlevel 1 goto :END
)

echo [admin] starting on :5174 ...
call npm run dev

:END
echo.
pause
endlocal
