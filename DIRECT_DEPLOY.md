# Direct Server Deployment Setup

This deployment method pushes code directly to the production server via SSH, bypassing GitHub Actions and Tailscale.

## One-Time Server Setup

Run these commands **on the production server** (SSH as pacha@100.100.206.41):

```bash
# 1. Navigate to project directory
cd ~/aweh.backoffice.server

# 2. Create post-receive hook
cat > .git/hooks/post-receive << 'EOF'
#!/bin/bash
# Auto-deploy on push

WORK_DIR="/home/pacha/aweh.backoffice.server"

echo "======================================"
echo "  Direct Deploy Hook Triggered"
echo "======================================"
echo ""

cd "$WORK_DIR" || exit 1
unset GIT_DIR

echo "📥 Pulling latest code..."
git reset --hard origin/main
echo ""

echo "🐳 Rebuilding gateway container..."
docker compose down gateway
docker compose build --no-cache gateway
docker compose up -d gateway
echo ""

echo "⏳ Waiting for start..."
sleep 8
echo ""

echo "📊 Status:"
docker compose ps gateway
echo ""

echo "📋 Logs:"
docker compose logs --tail=15 gateway
echo ""

echo "✅ Deployment Complete!"
echo "======================================"
EOF

# 3. Make hook executable
chmod +x .git/hooks/post-receive

# 4. Verify hook exists
ls -lh .git/hooks/post-receive

# 5. Test git config allows receiving pushes
git config receive.denyCurrentBranch updateInstead

echo "✓ Server is ready to receive direct pushes!"
```

## One-Time Local Setup

Run these commands **on your Windows machine**:

```powershell
# Navigate to gateway directory
cd C:\Users\herna\aweh.pos\gateway

# Add server as a git remote named "server"
git remote add server pacha@100.100.206.41:~/aweh.backoffice.server

# Verify remote was added
git remote -v
```

Expected output:
```
origin  https://github.com/NANZIPOO/aweh.backoffice.server.git (fetch)
origin  https://github.com/NANZIPOO/aweh.backoffice.server.git (push)
server  pacha@100.100.206.41:~/aweh.backoffice.server (fetch)
server  pacha@100.100.206.41:~/aweh.backoffice.server (push)
```

## Daily Usage

### Option 1: Quick Launcher (Recommended)

```cmd
deploy-direct.bat
```

This will:
1. Show current git status
2. Ask for confirmation
3. Push to GitHub (backup)
4. Push to server (triggers auto-deploy)

### Option 2: Manual Commands

```powershell
# Make changes, commit as usual
git add .
git commit -m "Your changes"

# Push to both GitHub and server
git push origin main
git push server main
```

The server will auto-deploy immediately after receiving the push.

### Option 3: Push Only to Server (Skip GitHub)

```powershell
git push server main
```

## What Happens on Push

When you `git push server main`, this happens automatically on the server:

1. ✓ Git hook receives push
2. ✓ Pulls latest code
3. ✓ Stops gateway container
4. ✓ Rebuilds container with latest code
5. ✓ Starts gateway container
6. ✓ Shows logs for verification

Total time: ~20-30 seconds

## Verification

After pushing, wait 30 seconds then check:

```powershell
curl http://100.100.206.41:8081/api/v1/version
```

Should show latest build date.

## Troubleshooting

### "Permission denied (publickey)"

You need to add your SSH key to the server:

```powershell
# On Windows, copy your public key
Get-Content ~\.ssh\id_rsa.pub | clip

# Then SSH to server and add it
ssh pacha@100.100.206.41
echo "YOUR_PUBLIC_KEY" >> ~/.ssh/authorized_keys
```

### "refusing to update checked out branch"

Run on server:
```bash
cd ~/aweh.backoffice.server
git config receive.denyCurrentBranch updateInstead
```

### Hook not executing

Check permissions on server:
```bash
chmod +x ~/aweh.backoffice.server/.git/hooks/post-receive
```

### Container not rebuilding

SSH to server and check logs:
```bash
docker compose logs --tail=50 gateway
```

## Advantages

✓ No GitHub Actions needed  
✓ No Tailscale dependency  
✓ Instant deployment (30 seconds total)  
✓ See deployment output immediately  
✓ Works even if GitHub is down  
✓ Still backs up to GitHub  

## Disadvantages

✗ Requires SSH access to server  
✗ No deployment history (unless you check git log)  
✗ Server must be reachable from your machine  

## Comparison with Other Methods

| Method | Deploy Time | Dependency | Reliability |
|--------|-------------|------------|-------------|
| Direct SSH Push | 30 sec | SSH only | ★★★★★ |
| GitHub Actions + Tailscale | 2-3 min | GitHub, Tailscale | ★★☆☆☆ |
| Manual SSH + Docker | 1-2 min | SSH only | ★★★★☆ |

## Workflow Example

```powershell
# 1. Make changes to code
code internal/models/inventory.go

# 2. Commit changes
git add .
git commit -m "fix: correct BULKSELLINGPRICE handling"

# 3. Deploy directly
deploy-direct.bat

# 4. Wait 30 seconds

# 5. Test
curl http://100.100.206.41:8081/api/v1/version
```

Done! Your changes are live.
