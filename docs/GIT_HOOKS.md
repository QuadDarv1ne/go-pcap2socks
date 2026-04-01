# Git Hooks для go-pcap2socks

## Установка

### Windows (PowerShell)

```powershell
# Копирование хуков
Copy-Item .githooks\pre-commit .git\hooks\pre-commit

# Сделать исполняемым (для Git Bash)
chmod +x .git/hooks/pre-commit
```

### Linux/macOS

```bash
# Копирование хуков
cp .githooks/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

### Автоматическая установка

```powershell
# PowerShell
.\install-hooks.ps1
```

## Доступные хуки

### pre-commit

Запускается перед каждым коммитом:

- ✅ `go fmt` — форматирование кода
- ✅ `go vet` — проверка на ошибки
- ✅ `go test -short` — быстрые тесты
- ✅ PowerShell синтаксис
- ✅ Trailing whitespace
- ✅ Проверка размера файлов (>1MB)

### commit-msg

Проверяет сообщение коммита:

- Минимальная длина 10 символов
- Наличие описания изменений

### pre-push

Запускается перед push:

- Полные тесты
- Проверка сборки

## Отключение хуков

### Временное отключение

```bash
git commit --no-verify -m "Your message"
```

### Отключение конкретного хука

```bash
# Для pre-commit
git config hooks.pre-commit false
```

## Настройка

### Пропуск проверок для определённых файлов

Создайте `.pre-commit-config.yaml`:

```yaml
skip:
  - "*.gen.go"
  - "*.pb.go"
  - "vendor/**"
```

### Изменение строгости

Отредактируйте `.githooks/pre-commit`:

```bash
# Отключить тесты для быстрых коммитов
# Закомментируйте строку с go test
```

## Устранение проблем

### Хук не запускается

```bash
# Проверьте права
ls -la .git/hooks/pre-commit

# Сделайте исполняемым
chmod +x .git/hooks/pre-commit
```

### Хук падает с ошибкой

```bash
# Запустите проверки вручную
go fmt ./...
go vet ./...
go test -short ./...

# Проверьте PowerShell
powershell -Command "& { \$e=\$null; [System.Management.Automation.Language.Parser]::ParseFile('script.ps1', [ref]\$null, [ref]\$e); if (\$e.Count -gt 0) { \$e } }"
```

### Ложные срабатывания

Если хук срабатывает ложно:

1. Проверьте вывод хука
2. Исправьте проблему
3. Используйте `--no-verify` для экстренных случаев

## Best Practices

### Частые коммиты

Делайте небольшие коммиты с понятными сообщениями:

```bash
# ✅ Хорошо
git commit -m "feat: добавить валидацию токенов"

# ❌ Плохо
git commit -m "fix"
```

### Pre-commit локально

Всегда запускайте pre-commit локально перед push:

```bash
git commit
# Хуки запустятся автоматически
```

### CI/CD интеграция

Хуки дополняют CI/CD, но не заменяют:

```yaml
# GitHub Actions всё равно нужен
name: Tests
on: push
jobs:
  test:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      - run: go test ./...
```

## Скрипт установки

Создайте `install-hooks.ps1`:

```powershell
# install-hooks.ps1
$hooksDir = ".git\hooks"
$sourceDir = ".githooks"

if (!(Test-Path $hooksDir)) {
    New-Item -ItemType Directory -Path $hooksDir
}

Copy-Item "$sourceDir\pre-commit" "$hooksDir\pre-commit"
Copy-Item "$sourceDir\commit-msg" "$hooksDir\commit-msg"

Write-Host "Git hooks installed successfully!" -ForegroundColor Green
```

## Ссылки

- [Git Hooks Documentation](https://git-scm.com/book/en/v2/Customizing-Git-Git-Hooks)
- [Pre-commit Framework](https://pre-commit.com/)

---

**Обновлено:** 1 апреля 2026 г.
**Версия:** 3.30.0+
