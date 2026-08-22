# Windows Setup Guide

This guide covers running loomWatch on Windows and configuring its providers.

loomWatch is a cluster product: it is delivered as a container image and a Helm
chart, and it publishes no binaries. Upstream's Windows installers are not part
of this fork - the PowerShell one-liner and `install.bat` that older copies of
this guide offered installed `onllm-dev/onwatch`, which is a different product
with a different feature set. If a laptop application is what you want, use
upstream directly.

What remains supported on Windows is building the binary from this source tree
and running it yourself. Everything below - the configuration file, the provider
tokens, the dashboard - applies to that.

---

## Step 1: Build the binary

Requires Go (the version in `go.mod`) and Git.

```powershell
git clone https://github.com/concordloom/loomwatch
cd loomwatch
go build -o "$env:USERPROFILE\.onwatch\bin\onwatch.exe" .
mkdir "$env:USERPROFILE\.onwatch\data" -Force
```

### Step 2: Create the Configuration File

Create a `.env` file at `%USERPROFILE%\.onwatch\.env` with your API keys.

**Using PowerShell:**

```powershell
@"
# onWatch Configuration

# At least one provider is required. Uncomment and configure the ones you use.

# Synthetic API key (https://synthetic.new/settings/api)
# SYNTHETIC_API_KEY=syn_your_key_here

# Z.ai API key (https://www.z.ai/api-keys)
# ZAI_API_KEY=your_key_here
# ZAI_BASE_URL=https://api.z.ai/api

# Anthropic token (Claude Code - see "Retrieving Tokens" below)
# ANTHROPIC_TOKEN=your_token_here

# Codex OAuth token (see "Retrieving Tokens" below)
# CODEX_TOKEN=your_token_here

# GitHub Copilot PAT with copilot scope
# COPILOT_TOKEN=ghp_your_token_here

# Antigravity (Windsurf) - auto-detected from local process
# ANTIGRAVITY_ENABLED=true

# Dashboard credentials
ONWATCH_ADMIN_USER=admin
ONWATCH_ADMIN_PASS=changeme

# Polling interval in seconds (10-3600)
ONWATCH_POLL_INTERVAL=120

# Dashboard port
ONWATCH_PORT=9211
"@ | Set-Content "$env:USERPROFILE\.onwatch\.env"
```

**Or using Notepad:**

```powershell
notepad "$env:USERPROFILE\.onwatch\.env"
```

Then paste the configuration above and save.

### Step 3: Configure Providers (Optional at Startup)

onWatch can start with no providers configured. You can enable providers later in **Settings -> Providers**.

If you want to pre-configure providers in `.env`, set any of the following:

#### Synthetic
```env
SYNTHETIC_API_KEY=syn_your_key_here
```
Get your key at: https://synthetic.new/settings/api

#### Z.ai
```env
ZAI_API_KEY=your_key_here
ZAI_BASE_URL=https://api.z.ai/api
```
Get your key at: https://www.z.ai/api-keys

#### Anthropic (Claude Code)
```env
ANTHROPIC_TOKEN=your_token_here
```
See [Retrieving Anthropic Token](#retrieving-anthropic-token) below.

#### Codex
```env
CODEX_TOKEN=your_token_here
```
See [Retrieving Codex Token](#retrieving-codex-token) below.

#### GitHub Copilot
```env
COPILOT_TOKEN=ghp_your_token_here
```
Create a Personal Access Token (classic) with `copilot` scope at: https://github.com/settings/tokens

#### Antigravity (Windsurf)
```env
ANTIGRAVITY_ENABLED=true
```
Requires Windsurf to be running. onWatch auto-detects the local server.

### Step 4: Add to PATH (Optional)

Add onWatch to your user PATH for easy access:

```powershell
$path = [Environment]::GetEnvironmentVariable("Path", "User")
$onwatchPath = "$env:USERPROFILE\.onwatch"
if ($path -notlike "*$onwatchPath*") {
    [Environment]::SetEnvironmentVariable("Path", "$onwatchPath;$path", "User")
}
```

Restart your terminal for PATH changes to take effect.

### Step 5: Run onWatch

Navigate to the installation directory and run:

```powershell
cd "$env:USERPROFILE\.onwatch"
.\bin\onwatch.exe
```

Or if you added it to PATH:

```powershell
onwatch
```

Open your browser to **http://localhost:9211** and log in with your configured credentials.

---

## Retrieving Tokens

### Retrieving Anthropic Token

If you have Claude Code installed, your token is stored in the credentials file:

```powershell
(Get-Content "$env:USERPROFILE\.claude\.credentials.json" | ConvertFrom-Json).claudeAiOauth.accessToken
```

Copy the output and paste it as `ANTHROPIC_TOKEN` in your `.env` file.

### Retrieving Codex Token

If you have Codex CLI installed, your token is stored in the auth file:

```powershell
(Get-Content "$env:USERPROFILE\.codex\auth.json" | ConvertFrom-Json).tokens.access_token
```

If you use a custom `CODEX_HOME`:

```powershell
(Get-Content "$env:CODEX_HOME\auth.json" | ConvertFrom-Json).tokens.access_token
```

Copy the output and paste it as `CODEX_TOKEN` in your `.env` file.

---

## Running onWatch

### Start (Background)

```powershell
cd "$env:USERPROFILE\.onwatch"
.\bin\onwatch.exe
```

onWatch runs in the background. Access the dashboard at http://localhost:9211.

### Start (Foreground/Debug)

```powershell
cd "$env:USERPROFILE\.onwatch"
.\bin\onwatch.exe --debug
```

Logs are printed to the console. Useful for troubleshooting.

### Stop

```powershell
cd "$env:USERPROFILE\.onwatch"
.\bin\onwatch.exe stop
```

### Check Status

```powershell
cd "$env:USERPROFILE\.onwatch"
.\bin\onwatch.exe status
```

---

## Creating a Desktop Shortcut

To create a shortcut for easy access:

1. Right-click on your Desktop
2. Select **New > Shortcut**
3. Enter the location:
   ```
   %USERPROFILE%\.onwatch\bin\onwatch.exe
   ```
4. Click **Next** and name it "onWatch"
5. Click **Finish**

To run in debug mode (shows console window):
- Right-click the shortcut > **Properties**
- In **Target**, append ` --debug`:
  ```
  %USERPROFILE%\.onwatch\bin\onwatch.exe --debug
  ```

---

## Running as a Windows Service (Advanced)

For production deployments, you can run onWatch as a Windows Service using [NSSM](https://nssm.cc/) (Non-Sucking Service Manager):

1. Download NSSM from https://nssm.cc/download
2. Extract and open Command Prompt as Administrator
3. Install the service:

```cmd
nssm install onwatch "%USERPROFILE%\.onwatch\bin\onwatch.exe"
nssm set onwatch AppDirectory "%USERPROFILE%\.onwatch"
nssm set onwatch AppParameters "--debug"
nssm set onwatch DisplayName "onWatch API Quota Tracker"
nssm set onwatch Description "Tracks AI API quota usage"
nssm set onwatch Start SERVICE_AUTO_START
nssm start onwatch
```

Manage the service:
```cmd
nssm status onwatch
nssm stop onwatch
nssm restart onwatch
nssm remove onwatch confirm
```

---

## Troubleshooting

### Windows Defender False Positive

Windows Defender may flag `onwatch.exe` as `Program:Win32/Wacapew.A!ml`. **This is a false positive.**

**Why this happens:** Go binaries are statically compiled, bundling the entire runtime into a single ~15MB executable. This structure - combined with the binary's network operations (HTTP server, API polling) - triggers machine learning heuristics designed to detect packed or obfuscated malware. This is a [known issue](https://go.dev/doc/faq#virus) affecting many legitimate Go applications.

**Our status:** We have submitted onwatch.exe to Microsoft for analysis and whitelisting. This process typically takes 1-2 weeks.

**Workaround:** Add an exclusion in Windows Defender:
1. Open **Windows Security** → **Virus & threat protection**
2. Under "Virus & threat protection settings", click **Manage settings**
3. Scroll to **Exclusions** → **Add or remove exclusions**
4. Click **Add an exclusion** → **Folder**
5. Add: `C:\Users\<your-username>\.onwatch`

The source code is fully auditable at [github.com/concordloom/loomwatch](https://github.com/concordloom/loomwatch) (GPL-3.0).

### "No provider data appears in dashboard"

If the app starts but no quota cards update:

1. Ensure `.env` exists at `%USERPROFILE%\.onwatch\.env`
2. Configure a provider key (or use auto-detect providers)
3. Open **Settings -> Providers** and enable telemetry for that provider
4. Restart or click provider reload in Settings

### Binary Flashes and Closes Immediately

This happens when double-clicking the binary without configuration. Create the configuration file described above and start the binary from a
terminal, where the error is visible.

### Port Already in Use

If port 9211 is taken, change it in `.env`:

```env
ONWATCH_PORT=9212
```

Check what's using a port:
```powershell
netstat -ano | findstr :9211
```

### Cannot Find onwatch Command

If `onwatch` isn't recognized after adding to PATH:
1. Restart your terminal/PowerShell
2. Or use the full path: `$env:USERPROFILE\.onwatch\bin\onwatch.exe`

### Token Auto-Detection Not Working

If Claude Code or Codex tokens aren't auto-detected:
1. Ensure the respective tool is installed and you've logged in
2. Check that credential files exist:
   - Claude Code: `%USERPROFILE%\.claude\.credentials.json`
   - Codex: `%USERPROFILE%\.codex\auth.json`
3. Manually retrieve and paste the token (see [Retrieving Tokens](#retrieving-tokens))

---

## Updating loomWatch

There is no self-update: this fork ships no binaries for one to download, and
the feature was removed rather than left offering a download that cannot exist.

1. Stop loomWatch: `.\bin\onwatch.exe stop`
2. Pull this repository and rebuild, as in Step 1
3. Start it again: `.\bin\onwatch.exe`

---

## Uninstalling

To completely remove onWatch:

```powershell
# Stop onWatch
cd "$env:USERPROFILE\.onwatch"
.\bin\onwatch.exe stop

# Remove installation directory
Remove-Item -Recurse -Force "$env:USERPROFILE\.onwatch"

# Remove from PATH (optional)
$path = [Environment]::GetEnvironmentVariable("Path", "User")
$newPath = ($path -split ';' | Where-Object { $_ -notlike "*\.onwatch*" }) -join ';'
[Environment]::SetEnvironmentVariable("Path", $newPath, "User")
```

---

## Support

- [GitHub Issues](https://github.com/concordloom/loomwatch/issues)
- [README](../README.md)
- [Development Guide](DEVELOPMENT.md)
#### MiniMax
```env
MINIMAX_API_KEY=sk-cp_your_key_here
```
Get your key at: https://platform.minimax.io
