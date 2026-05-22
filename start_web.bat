@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo   MediaCrawler Web UI Startup Script
echo ========================================
echo.

REM Check if uv is installed
uv --version >nul 2>&1
if errorlevel 1 (
    echo [WARNING] uv not detected, using standard Python virtual environment
    set USE_UV=0
) else (
    echo [INFO] Using uv package manager
    set USE_UV=1
)

REM Check if Python is installed
python --version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Python not detected, please install Python 3.8+
    echo.
    pause
    exit /b 1
)

REM Switch to MediaCrawler directory
cd /d "%~dp0MediaCrawler"
if not exist "%~dp0MediaCrawler" (
    echo [ERROR] MediaCrawler directory does not exist
    echo Current directory: %CD%
    echo.
    pause
    exit /b 1
)

if "%USE_UV%"=="1" (
    echo.
    echo [1/2] Syncing dependencies with uv...
    uv sync
    if errorlevel 1 (
        echo [ERROR] Failed to sync dependencies
        echo.
        pause
        exit /b 1
    )
    echo [SUCCESS] Dependencies synced
) else (
    echo [1/3] Checking virtual environment...
    if not exist "venv" (
        echo [INFO] Virtual environment does not exist, creating...
        python -m venv venv
        if errorlevel 1 (
            echo [ERROR] Failed to create virtual environment
            echo.
            pause
            exit /b 1
        )
        echo [SUCCESS] Virtual environment created
    ) else (
        echo [SUCCESS] Virtual environment exists
    )

    REM Activate virtual environment
    echo.
    echo [2/3] Activating virtual environment...
    call venv\Scripts\activate.bat
    if errorlevel 1 (
        echo [ERROR] Failed to activate virtual environment
        echo.
        pause
        exit /b 1
    )
    echo [SUCCESS] Virtual environment activated

    REM Install dependencies
    echo.
    echo [3/3] Checking and installing dependencies...
    pip show fastapi >nul 2>&1
    if errorlevel 1 (
        echo [INFO] Installing Python dependencies...
        pip install -r requirements.txt -i https://pypi.tuna.tsinghua.edu.cn/simple
        if errorlevel 1 (
            echo [ERROR] Failed to install dependencies
            echo.
            pause
            exit /b 1
        )
    ) else (
        echo [SUCCESS] Dependencies already installed
    )
)

REM Start Web UI
echo.
echo ========================================
echo   Starting Web UI...
echo ========================================
echo.
echo [INFO] Web UI URL: http://localhost:8080
echo [INFO] API Docs URL: http://localhost:8080/docs
echo [INFO] Press Ctrl+C to stop the service
echo.

REM Wait 2 seconds and automatically open browser
start "" cmd /c "timeout /t 2 /nobreak >nul && start http://localhost:8080"

REM Start uvicorn server
if "%USE_UV%"=="1" (
    uv run uvicorn api.main:app --host 0.0.0.0 --port 8080 --reload
) else (
    python -m uvicorn api.main:app --host 0.0.0.0 --port 8080 --reload
)

REM Check execution result
if errorlevel 1 (
    echo.
    echo [ERROR] Web UI startup failed
    echo.
    pause
    exit /b 1
)

echo.
echo Press any key to exit...
pause >nul
