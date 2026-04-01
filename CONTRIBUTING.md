# Contributing to go-pcap2socks

Благодарим за интерес к внесению вклада в go-pcap2socks! Это руководство поможет вам начать.

## 📋 Содержание

- [Как я могу помочь?](#как-я-могу-помочь)
- [Начало работы](#начало-работы)
- [Стандарты кода](#стандарты-кода)
- [Pull Request процесс](#pull-request-процесс)
- [Тестирование](#тестирование)
- [Документация](#документация)

## 🤝 Как я могу помочь?

Есть много способов внести вклад:

### Код
- Исправление багов
- Новые функции
- Оптимизация производительности
- Улучшение обработки ошибок

### Документация
- Исправление опечаток
- Улучшение существующих руководств
- Добавление примеров использования
- Переводы на другие языки

### Тестирование
- Сообщение о багах
- Написание тестов
- Проверка PR перед слиянием

### Поддержка
- Ответы на вопросы в Issues
- Помощь новым пользователям

## 🚀 Начало работы

### 1. Fork репозитория

Нажмите **Fork** на GitHub для создания копии репозитория.

### 2. Клонируйте fork

```powershell
git clone https://github.com/YOUR_USERNAME/go-pcap2socks.git
cd go-pcap2socks
```

### 3. Настройте upstream

```powershell
git remote add upstream https://github.com/QuadDarv1ne/go-pcap2socks.git
git fetch upstream
```

### 4. Создайте ветку

```powershell
# Для новой функции
git checkout -b feature/amazing-feature

# Для исправления бага
git checkout -b fix/bug-fix
```

## 📝 Стандарты кода

### Go код

- Следуйте [Effective Go](https://golang.org/doc/effective_go.html)
- Используйте `go fmt` перед коммитом
- Избегайте глобальных переменных (используйте globals.go)
- Добавляйте обработку ошибок
- Используйте контекст для отмены операций

```go
// ✅ Хорошо
func (s *Server) handleRequest(ctx context.Context, req *Request) error {
    if err := s.validate(req); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    // ...
}

// ❌ Плохо
func (s *Server) handleRequest(req *Request) {
    // Нет контекста, нет возврата ошибки
}
```

### Именование

- Используйте понятные имена на английском
- Экспортируемые функции: `CamelCase`
- Внутренние функции: `camelCase`
- Константы: `PascalCase` или `SCREAMING_SNAKE_CASE`

```go
// ✅ Хорошо
func validateTokenStrength(token string) int { }
var ErrConnectionClosed = errors.New("connection closed")

// ❌ Плохо
func vldTk(t string) int { }
var err = errors.New("error")
```

### Комментарии

- Документируйте экспортируемые функции
- Объясняйте **почему**, а не **что**
- Используйте godoc формат

```go
// ✅ Хорошо
// validateTokenStrength проверяет сложность API токена.
// Возвращает оценку от 1 (очень слабый) до 5 (очень сильный).
func validateTokenStrength(token string) int { }

// ❌ Плохо
// Эта функция проверяет токен и возвращает число
func validateTokenStrength(token string) int { }
```

### PowerShell скрипты

- Используйте `Verb-Noun` соглашение
- Добавляйте `param()` блок
- Обрабатывайте ошибки

```powershell
# ✅ Хорошо
function Start-ServiceWithRetry {
    param(
        [string]$ServiceName,
        [int]$MaxRetries = 3
    )
    # ...
}

# ❌ Плохо
function start($name) {
    # ...
}
```

## 🔀 Pull Request процесс

### 1. Перед отправкой

- [ ] Код следует стандартам
- [ ] Добавлены тесты для новых функций
- [ ] Все тесты проходят
- [ ] Документация обновлена
- [ ] Коммиты имеют понятные сообщения

### 2. Сообщение коммита

```
# ✅ Хорошо
feat: добавить адаптивный memory limit

- Автоматический расчёт на основе доступной RAM
- Логирование установленного лимита
- Настройка через GOMEMLIMIT

Fixes #123

# ❌ Плохо
fix stuff
```

### 3. Создание PR

1. Откройте PR на GitHub
2. Заполните шаблон PR
3. Добавьте описание изменений
4. Свяжите с Issue (если есть)

### 4. Code Review

- Ответьте на комментарии ревьюеров
- Внесите запрошенные изменения
- После approval PR будет merged

## 🧪 Тестирование

### Запуск тестов

```powershell
# Быстрые тесты
go test -short ./...

# Полные тесты
go test ./...

# С race detector
go test -race ./...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Написание тестов

```go
func TestValidateTokenStrength(t *testing.T) {
    tests := []struct {
        name     string
        token    string
        expected int
    }{
        {
            name:     "very strong",
            token:    "aB3$xY9@mN2&kL7!",
            expected: 5,
        },
        // ...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := validateTokenStrength(tt.token)
            if got != tt.expected {
                t.Errorf("got %d, want %d", got, tt.expected)
            }
        })
    }
}
```

### PowerShell тестирование

```powershell
# Проверка синтаксиса
$e = $null
[System.Management.Automation.Language.Parser]::ParseFile(
    'script.ps1', [ref]$null, [ref]$e
)
if ($e.Count -gt 0) {
    throw "Syntax errors: $($e.Count)"
}
```

## 📚 Документация

### Формат Markdown

- Используйте заголовки `#`, `##`, `###`
- Добавляйте таблицы для сравнений
- Используйте code blocks с указанием языка

### Обновление документации

При добавлении новой функции:

1. Обновите README.md
2. Добавьте/обновите документ в `docs/`
3. Обновите CHANGELOG.md
4. Добавьте примеры использования

## 📋 Чек-лист перед отправкой

- [ ] `go build` проходит без ошибок
- [ ] `go test ./...` проходит
- [ ] `go vet ./...` не показывает проблем
- [ ] Код отформатирован (`go fmt`)
- [ ] PowerShell скрипты проверены
- [ ] Документация обновлена
- [ ] CHANGELOG.md обновлён

## 🎯 Области для вклада

### Prioritetные задачи

Ищите issues с метками:
- `good first issue` — для новичков
- `help wanted` — требуется помощь
- `bug` — исправление багов
- `enhancement` — новые функции

### Идеи для улучшений

- Улучшение обработки ошибок
- Добавление метрик Prometheus
- Оптимизация производительности
- Улучшение документации
- Новые PowerShell утилиты
- Интеграции с внешними сервисами

## 💬 Вопросы

- **GitHub Issues** — для багов и feature requests
- **GitHub Discussions** — для вопросов и обсуждений
- **Telegram** — для быстрых вопросов (если есть канал)

## 📜 Лицензия

Внося код, вы соглашаетесь с лицензией проекта (MIT License).

---

**Спасибо за ваш вклад!** 🎉

Каждый PR делает go-pcap2socks лучше для всех пользователей.

---

**Обновлено:** 1 апреля 2026 г.
**Версия:** 3.30.0+
