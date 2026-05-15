@echo off
setlocal
title Backend :8080

if exist "C:\Program Files\Go\bin\go.exe" set "PATH=C:\Program Files\Go\bin;%PATH%"

echo [backend] cwd=%CD%
echo [backend] go:
where go 2>nul
if errorlevel 1 (
    echo [ERROR] go not found. Install Go or add its bin folder to PATH, then reopen this window.
    goto :END
)
go version

echo [backend] ensuring database (go run ./cmd/createdb) ...
go run ./cmd/createdb
if errorlevel 1 (
    echo [WARN] createdb failed. Check MySQL and backend\config\config.yaml DSN.
)

echo [backend] starting API (go run ./cmd/server) ...
go run ./cmd/server
echo [backend] process exited with code %ERRORLEVEL%

:END
echo.
pause
endlocal
