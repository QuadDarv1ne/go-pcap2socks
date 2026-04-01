# Install Git Hooks for go-pcap2socks
# Установка git hooks для проекта

$ErrorActionPreference = "Stop"

Write-Host "Installing git hooks..." -ForegroundColor Cyan

$hooksDir = ".git\hooks"
$sourceDir = ".githooks"

# Create hooks directory if it doesn't exist
if (!(Test-Path $hooksDir)) {
    New-Item -ItemType Directory -Path $hooksDir | Out-Null
    Write-Host "Created $hooksDir directory" -ForegroundColor Green
}

# Copy pre-commit hook
if (Test-Path "$sourceDir\pre-commit") {
    Copy-Item "$sourceDir\pre-commit" "$hooksDir\pre-commit" -Force
    Write-Host "✓ Installed pre-commit hook" -ForegroundColor Green
} else {
    Write-Host "⚠ pre-commit not found in $sourceDir" -ForegroundColor Yellow
}

# Make hooks executable (for Git Bash on Windows)
if (Get-Command chmod -ErrorAction SilentlyContinue) {
    chmod +x "$hooksDir\pre-commit" 2>$null
}

Write-Host ""
Write-Host "Git hooks installed successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "To verify:" -ForegroundColor White
Write-Host "  git commit --allow-empty -m 'Test commit'"
Write-Host "  git reset --soft HEAD~1  # Undo test commit"
Write-Host ""
