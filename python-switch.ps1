# Python Version Switcher для Windows
# Использование: .\python-switch.ps1 [version]
# Примеры:
#   .\python-switch.ps1          - показать текущую версию и доступные
#   .\python-switch.ps1 3.14     - переключить на Python 3.14
#   .\python-switch.ps1 3.13     - переключить на Python 3.13
#   .\python-switch.ps1 3.11     - переключить на Python 3.11
#   .\python-switch.ps1 list     - показать все доступные версии
#   .\python-switch.ps1 default  - использовать версию по умолчанию

param(
    [Parameter(Position=0)]
    [string]$Version
)

$ErrorActionPreference = "Stop"

# Определение доступных версий Python
$PythonVersions = @{
    "3.14" = @{
        "Path" = "C:\Users\maksi\AppData\Local\Programs\Python\Python314"
        "Scripts" = "C:\Users\maksi\AppData\Local\Programs\Python\Python314\Scripts"
        "Executable" = "C:\Users\maksi\AppData\Local\Programs\Python\Python314\python.exe"
    }
    "3.13" = @{
        "Path" = "C:\laragon\bin\python\python-3.13"
        "Scripts" = "C:\laragon\bin\python\python-3.13\Scripts"
        "Executable" = "C:\laragon\bin\python\python-3.13\python.exe"
    }
    "3.11" = @{
        "Path" = "py -3.11"
        "Scripts" = ""
        "Executable" = "py -3.11"
        "UseLauncher" = $true
    }
}

function Show-CurrentVersion {
    Write-Host "`n🐍 Текущая версия Python:" -ForegroundColor Cyan
    try {
        $version = & python --version 2>&1
        Write-Host "   $version" -ForegroundColor Green
    } catch {
        Write-Host "   ❌ Python не найден в PATH" -ForegroundColor Red
    }
    
    Write-Host "`n📋 Доступные версии:" -ForegroundColor Cyan
    foreach ($v in $PythonVersions.Keys | Sort-Object) {
        $path = $PythonVersions[$v]["Executable"]
        if ($PythonVersions[$v]["UseLauncher"]) {
            try {
                $ver = & py "-$v" --version 2>&1
                Write-Host "   ✓ Python $v (через py launcher)" -ForegroundColor Green
            } catch {
                Write-Host "   ✗ Python $v (не найден)" -ForegroundColor Red
            }
        } else {
            if (Test-Path $path) {
                try {
                    $ver = & $path --version 2>&1
                    Write-Host "   ✓ $ver" -ForegroundColor Green
                } catch {
                    Write-Host "   ✓ Python $v" -ForegroundColor Green
                }
            } else {
                Write-Host "   ✗ Python $v (путь не найден: $path)" -ForegroundColor Red
            }
        }
    }
    Write-Host ""
}

function Show-ListVersions {
    Write-Host "`n📋 Доступные версии Python:" -ForegroundColor Cyan
    foreach ($v in $PythonVersions.Keys | Sort-Object) {
        $path = $PythonVersions[$v]["Executable"]
        if ($PythonVersions[$v]["UseLauncher"]) {
            try {
                $ver = & py "-$v" --version 2>&1
                Write-Host "   🐍 $ver - $($PythonVersions[$v]['Path'])" -ForegroundColor Green
            } catch {
                Write-Host "   ❌ Python $v - не найден" -ForegroundColor Red
            }
        } else {
            if (Test-Path $path) {
                try {
                    $ver = & $path --version 2>&1
                    Write-Host "   🐍 $ver - $($PythonVersions[$v]['Path'])" -ForegroundColor Green
                } catch {
                    Write-Host "   🐍 Python $v - $($PythonVersions[$v]['Path'])" -ForegroundColor Yellow
                }
            } else {
                Write-Host "   ❌ Python $v - путь не найден" -ForegroundColor Red
            }
        }
    }
    
    Write-Host "`n💡 Совет: Windows Python Launcher (py) позволяет запускать любую версию:" -ForegroundColor Yellow
    Write-Host "   py -3.14 script.py" -ForegroundColor White
    Write-Host "   py -3.13 script.py" -ForegroundColor White
    Write-Host "   py -3.11 script.py" -ForegroundColor White
    Write-Host ""
}

function Switch-PythonVersion {
    param([string]$TargetVersion)
    
    if (-not $PythonVersions.ContainsKey($TargetVersion)) {
        Write-Host "`n❌ Версия $TargetVersion не найдена!" -ForegroundColor Red
        Write-Host "Доступные версии: $($PythonVersions.Keys -join ', ')" -ForegroundColor Yellow
        return
    }
    
    $versionInfo = $PythonVersions[$TargetVersion]
    
    if ($versionInfo["UseLauncher"]) {
        Write-Host "`n⚠️  Версия $TargetVersion доступна только через Windows Python Launcher (py)" -ForegroundColor Yellow
        Write-Host "Используйте: py -$TargetVersion your_script.py" -ForegroundColor Cyan
        Write-Host ""
        return
    }
    
    Write-Host "`n🔄 Переключение на Python $TargetVersion..." -ForegroundColor Cyan
    
    # Проверка существования путей
    if (-not (Test-Path $versionInfo["Path"])) {
        Write-Host "❌ Путь не найден: $($versionInfo['Path'])" -ForegroundColor Red
        return
    }
    
    Write-Host "✅ Python $TargetVersion готов к использованию!" -ForegroundColor Green
    Write-Host "   Основной путь: $($versionInfo['Path'])" -ForegroundColor White
    Write-Host "   Scripts путь: $($versionInfo['Scripts'])" -ForegroundColor White
    
    # Показываем команды для добавления в PATH
    Write-Host "`n📌 Для временного добавления в PATH (текущая сессия):" -ForegroundColor Yellow
    Write-Host "`$env:Path = `"$($versionInfo['Path']);$($versionInfo['Scripts']);`$env:Path`"" -ForegroundColor White
    
    Write-Host "`n📌 Для постоянного изменения PATH используйте:" -ForegroundColor Yellow
    Write-Host "[Environment]::SetEnvironmentVariable('Path', ..." -ForegroundColor White
    
    Write-Host ""
}

# Основная логика
if (-not $Version) {
    Show-CurrentVersion
} elseif ($Version -eq "list") {
    Show-ListVersions
} elseif ($Version -eq "default") {
    Switch-PythonVersion -TargetVersion "3.14"
} else {
    Switch-PythonVersion -TargetVersion $Version
}
