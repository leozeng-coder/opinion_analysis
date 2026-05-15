@echo off
setlocal
title Frontend :5173

if exist "C:\Program Files\nodejs\node.exe" set "PATH=C:\Program Files\nodejs;%PATH%"

echo [frontend] cwd=%CD%
echo [frontend] node:
where node 2>nul
if errorlevel 1 (
    echo [ERROR] node not found. Install Node.js or add it to PATH.
    goto :END
)
node -v
echo [frontend] npm:
where npm 2>nul
if errorlevel 1 (
    echo [ERROR] npm not found.
    goto :END
)

echo [frontend] npm run dev ...
call npm run dev
echo [frontend] npm exited with code %ERRORLEVEL%

:END
echo.
pause
endlocal
