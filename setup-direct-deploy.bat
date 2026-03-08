@echo off
REM One-time setup for direct server deployment

echo ========================================
echo   DIRECT DEPLOY SETUP
echo ========================================
echo.

echo This will configure direct push deployment to your server.
echo.
echo Prerequisites:
echo   - SSH access to pacha@100.100.206.41
echo   - SSH key authentication already working
echo.

pause

echo.
echo Step 1/2: Adding server as git remote...
echo.

git remote remove server 2>nul
git remote add server pacha@100.100.206.41:~/aweh.backoffice.server

echo Verifying remotes:
git remote -v
echo.

echo Step 2/2: Installing post-receive hook on server...
echo.
echo You'll need to run these commands on the server.
echo Opening SSH connection now...
echo.

echo Copy and paste these commands in the SSH session:
echo --------------------------------------------------
echo cd ~/aweh.backoffice.server
echo cat ^> .git/hooks/post-receive ^<^< 'EOF'
type deploy\post-receive-hook.sh
echo EOF
echo chmod +x .git/hooks/post-receive
echo git config receive.denyCurrentBranch updateInstead
echo echo "✓ Hook installed successfully!"
echo --------------------------------------------------
echo.

pause

ssh pacha@100.100.206.41

echo.
echo ========================================
echo   Setup Complete!
echo ========================================
echo.
echo You can now deploy using:
echo   deploy-direct.bat
echo.
echo OR manually with:
echo   git push server main
echo.

pause
