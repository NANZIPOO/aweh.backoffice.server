@echo off
REM Direct Deploy to Production Server via SSH
REM This script pushes code directly to the server, bypassing GitHub

echo ========================================
echo   DIRECT SERVER DEPLOYMENT
echo ========================================
echo.

echo Checking git status...
git status --short
echo.

set /p CONFIRM="Push to production server? (Y/N): "
if /i not "%CONFIRM%"=="Y" (
    echo Deployment cancelled.
    exit /b 0
)

echo.
echo 1. Pushing to GitHub (backup)...
git push origin main
if errorlevel 1 (
    echo WARNING: GitHub push failed, continuing with server deploy...
)
echo.

echo 2. Pushing directly to production server...
git push server main
if errorlevel 1 (
    echo ERROR: Server push failed!
    pause
    exit /b 1
)
echo.

echo ========================================
echo   Deployment Successful!
echo ========================================
echo.
echo The server post-receive hook will:
echo   - Pull latest code
echo   - Rebuild Docker container
echo   - Restart gateway service
echo.
echo Wait ~30 seconds, then test: http://100.100.206.41:8081/api/v1/version
echo.

pause
