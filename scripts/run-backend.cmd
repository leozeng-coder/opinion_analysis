@echo off
setlocal EnableExtensions
title Backend :8080

for %%I in ("%~dp0..\backend") do set "BACKEND_ROOT=%%~fI"

cd /d "%BACKEND_ROOT%"
if errorlevel 1 goto :ERR_CD

if not exist "%BACKEND_ROOT%\go.mod" goto :ERR_MOD

if exist "C:\Program Files\Go\bin\go.exe" set "PATH=C:\Program Files\Go\bin;%PATH%"

echo [backend] cwd=%CD%
echo [backend] go:
where go 2>nul
if errorlevel 1 goto :ERR_GO
go version

echo [backend] ensuring database (go run ./cmd/createdb) ...
go run ./cmd/createdb
if errorlevel 1 echo [WARN] createdb failed. Check MySQL and backend\config\config.yaml DSN.

echo [backend] starting API (go run ./cmd/server) ...
go run ./cmd/server
echo [backend] process exited with code %ERRORLEVEL%
goto :END

:ERR_CD
echo [ERROR] Cannot cd to backend: %BACKEND_ROOT%
goto :END

:ERR_MOD
echo [ERROR] go.mod missing at %BACKEND_ROOT%
echo [ERROR] cwd=%CD%
goto :END

:ERR_GO
echo [ERROR] go not found. Install Go or add its bin folder to PATH.
goto :END

:END
echo.
pause
endlocal
