# PowerShell script for development environment

# Function to print colored text
function Write-ColorOutput {
    param(
        [string]$Color,
        [string]$ColoredMessage,
        [string]$UncoloredMessage
    )
    Write-Host -NoNewline -ForegroundColor $Color $ColoredMessage
    Write-Host $UncoloredMessage
}

# Function to print section headers
function Write-SectionHeader {
    param(
        [string]$Color,
        [string]$Message
    )
    $terminalWidth = $Host.UI.RawUI.WindowSize.Width
    if (-not $terminalWidth) { $terminalWidth = 80 }
    $paddingWidth = [math]::Floor(($terminalWidth - $Message.Length - 2) / 2)
    $padding = "-" * $paddingWidth
    
    Write-ColorOutput $Color "$padding $Message $padding"
}

# Start the server
Write-SectionHeader "Cyan" "Starting server..."
$serverJob = Start-Job -ScriptBlock {
    Set-Location $using:PWD
    try {
        go run main.go server --dev-mode 2>&1 | ForEach-Object {
            Write-Host -ForegroundColor Cyan "[server] $_"
        }
    } catch {
        Write-Host -ForegroundColor Red "[server] Error: $_"
        Write-Host -ForegroundColor Red "[server] Stack trace: $($_.ScriptStackTrace)"
    }
}

# Wait for server to be ready
Write-SectionHeader "Cyan" "Waiting for server..."
$serverReady = $false
for ($i = 0; $i -lt 60; $i++) {
    # Check if the job has any output
    $serverOutput = Receive-Job $serverJob
    if ($serverOutput) {
        Write-Host -ForegroundColor Cyan "[server] $serverOutput"
    }
    
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8080/healthz" -UseBasicParsing -ErrorAction SilentlyContinue
        if ($response.StatusCode -eq 200) {
            $serverReady = $true
            Write-SectionHeader "Cyan" "Server ready!"
            break
        }
    } catch {}
    Start-Sleep -Seconds 1
}

if (-not $serverReady) {
    Write-SectionHeader "Red" "Timeout waiting for server to start"
    # Get any remaining output from the job
    $serverOutput = Receive-Job $serverJob
    if ($serverOutput) {
        Write-Host -ForegroundColor Red "[server] Final output: $serverOutput"
    }
    Stop-Job $serverJob
    Remove-Job $serverJob
    exit 1
}

# Start the admin UI
Write-SectionHeader "Green" "Starting admin UI..."
$adminUIJob = Start-Job -ScriptBlock {
    Set-Location $using:PWD
    Set-Location ui/admin
    pnpm i --ignore-scripts 2>&1 | ForEach-Object {
        Write-Host -ForegroundColor Green "[admin-ui](install) $_"
    }
    $env:VITE_API_IN_BROWSER = "true"
    npm run dev 2>&1 | ForEach-Object {
        Write-Host -ForegroundColor Green "[admin-ui] $_"
    }
}

# Wait for admin UI to be ready
Write-SectionHeader "Green" "Waiting for admin UI..."
$adminUIReady = $false
for ($i = 0; $i -lt 60; $i++) {
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8080/admin/" -UseBasicParsing -ErrorAction SilentlyContinue
        if ($response.StatusCode -eq 200) {
            $adminUIReady = $true
            Write-SectionHeader "Green" "Admin UI ready!"
            break
        }
    } catch {}
    Start-Sleep -Seconds 1
}

if (-not $adminUIReady) {
    Write-SectionHeader "Red" "Timeout waiting for admin UI to start"
    Stop-Job $adminUIJob
    Remove-Job $adminUIJob
    Stop-Job $serverJob
    Remove-Job $serverJob
    exit 1
}

# Start the user UI
Write-SectionHeader "Yellow" "Starting user UI..."
$userUIJob = Start-Job -ScriptBlock {
    Set-Location $using:PWD
    Set-Location ui/user
    pnpm i 2>&1 | ForEach-Object {
        Write-Host -ForegroundColor Yellow "[user-ui](install) $_"
    }
    pnpm run dev --port 5174 2>&1 | ForEach-Object {
        Write-Host -ForegroundColor Yellow "[user-ui] $_"
    }
}

# Wait for user UI to be ready
Write-SectionHeader "Yellow" "Waiting for user UI..."
$userUIReady = $false
for ($i = 0; $i -lt 60; $i++) {
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8080/favicon.ico" -UseBasicParsing -ErrorAction SilentlyContinue
        if ($response.StatusCode -eq 200) {
            $userUIReady = $true
            Write-SectionHeader "Yellow" "User UI ready!"
            break
        }
    } catch {}
    Start-Sleep -Seconds 1
}

if (-not $userUIReady) {
    Write-SectionHeader "Red" "Timeout waiting for user UI to start"
    Stop-Job $userUIJob
    Remove-Job $userUIJob
    Stop-Job $adminUIJob
    Remove-Job $adminUIJob
    Stop-Job $serverJob
    Remove-Job $serverJob
    exit 1
}

Write-SectionHeader "Green" "All components ready!"
Write-ColorOutput "Green" "UIs are accessible at: " "http://localhost:8080/"

# Open browser if requested
if ($args -contains "--open-uis") {
    Start-Process "http://localhost:8080/"
}

# Wait for all jobs to complete
Wait-Job $serverJob, $adminUIJob, $userUIJob | Out-Null

# Clean up
Stop-Job $serverJob, $adminUIJob, $userUIJob
Remove-Job $serverJob, $adminUIJob, $userUIJob 