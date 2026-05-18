@echo off
setlocal EnableExtensions
title Frontend :5173

REM Absolute package dir (never rely on cwd being scripts\)
for %%I in ("%~dp0..\frontend") do set "PKGROOT=%%~fI"

cd /d "%PKGROOT%"
if errorlevel 1 goto :ERR_CD

if not exist "%PKGROOT%\package.json" goto :ERR_PKG

if exist "C:\Program Files\nodejs\node.exe" set "PATH=C:\Program Files\nodejs;%PATH%"

where node >nul 2>&1
if errorlevel 1 goto :ERR_NODE

if exist "node_modules" goto :RUN_DEV
echo [frontend] installing deps ...
call npm --prefix "%PKGROOT%" install
if errorlevel 1 goto :END

:RUN_DEV
echo [frontend] cwd=%CD%
echo [frontend] starting on :5173 ...
REM Must use: npm --prefix DIR run dev (not: npm run dev --prefix DIR)
call npm --prefix "%PKGROOT%" run dev
goto :END

:ERR_CD
echo [ERROR] Cannot cd to %PKGROOT%
goto :END

:ERR_PKG
echo [ERROR] package.json missing at %PKGROOT%
echo [ERROR] cwd=%CD%
goto :END

:ERR_NODE
echo [ERROR] node not found. Install Node.js or add it to PATH.
goto :END

:END
echo.
pause
endlocal
