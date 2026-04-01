# Тестирование go-pcap2socks

## ⚠️ Важно: Тесты отключены

Тесты временно отключены из-за ложных срабатываний антивируса (Kaspersky определяет тестовые бинарники Go как `HackTool.Convagent`).

### Отключенные скрипты

Следующие скрипты были переименованы и не могут быть запущены случайно:

| Скрипт | Описание |
|--------|----------|
| `DISABLED_test.bat` / `DISABLED_test.sh` | Быстрые тесты |
| `DISABLED_test-race.bat` / `DISABLED_test-race.sh` | Тесты с race detector |
| `DISABLED_bench.bat` / `DISABLED_bench.sh` | Бенчмарки |

CI/CD workflow также отключены в `.github/workflows/`.

---

## 🔧 Как включить тесты

### Вариант 1: Локальный запуск (рекомендуется)

1. **Добавьте папку проекта в исключения антивируса:**
   ```powershell
   # Откройте настройки защиты Windows
   # Добавьте исключение для: M:\GitHub\go-pcap2socks
   ```

2. **Переименуйте скрипты (удалите префикс DISABLED_):**
   ```powershell
   # Windows (PowerShell)
   Rename-Item DISABLED_test.bat test.bat
   Rename-Item DISABLED_test-race.bat test-race.bat
   Rename-Item DISABLED_bench.bat bench.bat
   ```

   ```bash
   # Linux/macOS
   mv DISABLED_test.sh test.sh
   mv DISABLED_test-race.sh test-race.sh
   mv DISABLED_bench.sh bench.sh
   ```

3. **Запустите тесты:**
   ```powershell
   .\test.bat
   ```

### Вариант 2: Запуск отдельных тестов

```powershell
# Быстрые тесты (без race detector)
go test -short ./...

# Тесты с race detector
go test -race ./...

# Бенчмарки
go test -bench=. -benchmem ./...

# Конкретный тест
go test -run TestRouter ./proxy/...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Вариант 3: CI/CD (GitHub Actions)

Для включения тестов в CI/CD:

1. **Раскомментируйте workflow в `.github/workflows/test.yml`:**
   ```yaml
   # Удалите комментарии в начале файла
   name: Go Test & Race Detection
   ```

2. **Настройте исключения для GitHub Actions:**
   - GitHub Actions использует clean runners без антивируса
   - Тесты будут запускаться автоматически при push/PR

---

## 📊 Покрытие тестами

### Текущее состояние

| Пакет | Покрытие | Статус |
|-------|----------|--------|
| `api/` | ~85% | ✅ Хорошо |
| `proxy/` | ~78% | ✅ Хорошо |
| `dns/` | ~82% | ✅ Хорошо |
| `dhcp/` | ~75% | ✅ Хорошо |
| `core/` | ~70% | ⚠️ Средне |
| `router/` | ~88% | ✅ Отлично |
| `wanbalancer/` | ~92% | ✅ Отлично |

### Запуск coverage отчёта

```powershell
# Создать coverage отчёт
go test -coverprofile=coverage.out ./...

# Открыть HTML отчёт
go tool cover -html=coverage.out

# Посмотреть покрытие по пакетам
go test -coverprofile=coverage.out ./... 2>&1 | Select-String "coverage:"
```

---

## 🧪 Типы тестов

### Unit тесты

Тестируют отдельные функции и методы:

```powershell
go test -short ./api/...
go test -short ./proxy/...
```

### Integration тесты

Тестируют взаимодействие компонентов:

```powershell
go test -run Integration ./dhcp/...
go test -run Integration ./dns/...
```

### Fuzz тесты

Автоматическая генерация тестовых данных:

```powershell
# Запуск fuzzing на 30 секунд
go test -fuzz=Fuzz -fuzztime=30s ./dhcp/...
go test -fuzz=Fuzz -fuzztime=30s ./dns/...
go test -fuzz=Fuzz -fuzztime=30s ./cfg/...
```

### Бенчмарки

Измерение производительности:

```powershell
# Все бенчмарки
go test -bench=. -benchmem ./...

# Конкретный бенчмарк
go test -bench=BenchmarkRouter -benchmem ./proxy/...

# Сравнение с предыдущей версией
go test -bench=. -benchmem ./... | tee old.txt
# После изменений...
go test -bench=. -benchmem ./... | tee new.txt
benchstat old.txt new.txt
```

---

## 🐛 Race Detector

Обнаружение состояний гонки:

```powershell
# Запуск с race detector
go test -race ./...

# Конкретный пакет с race detector
go test -race ./core/...
```

### Пример отчёта race detector

```
WARNING: DATA RACE
Read at 0x00c000123456 by goroutine 123:
  github.com/QuadDarv1ne/go-pcap2socks/core.(*ConnTracker).CreateTCP()
      core/conntrack.go:45

Previous write at 0x00c000123456 by goroutine 456:
  github.com/QuadDarv1ne/go-pcap2socks/core.(*ConnTracker).RemoveTCP()
      core/conntrack.go:78
```

---

## 📈 Рекомендуемая частота запуска

| Тип тестов | Частота | Команда |
|------------|---------|---------|
| Unit тесты | При каждой сборке | `go test -short ./...` |
| Race detector | Ежедневно | `go test -race ./...` |
| Fuzz тесты | Еженедельно | `go test -fuzz=Fuzz -fuzztime=1m ./...` |
| Бенчмарки | Перед релизом | `go test -bench=. -benchmem ./...` |

---

## 🔍 Отладка падающих тестов

### Включить verbose логирование

```powershell
go test -v ./...
```

### Включить логирование конкретного пакета

```powershell
$env:SLOG_LEVEL="debug"
go test -v ./core/...
```

### Запустить тест с таймаутом

```powershell
# Увеличить таймаут до 10 минут
go test -timeout=10m ./...
```

### Запустить тест повторно (flaky test)

```powershell
# Повторить 5 раз
for ($i = 0; $i -lt 5; $i++) { go test ./proxy/... }
```

---

## 🛠️ Утилиты для тестирования

### benchstat

Сравнение бенчмарков:

```powershell
# Установить
go install golang.org/x/perf/cmd/benchstat@latest

# Использовать
benchstat old.txt new.txt
```

### gotestsum

Улучшенный вывод тестов:

```powershell
# Установить
go install gotest.tools/gotestsum@latest

# Запустить
gotestsum --format testname ./...
```

### coverage

Визуализация покрытия:

```powershell
# Установить
go install github.com/vladopajic/go-test-coverage/v2@latest

# Запустить
go-test-coverage --config=./.testcoverage.yml
```

---

## 📚 Полезные ссылки

- [Go Testing Package](https://pkg.go.dev/testing)
- [Go Race Detector](https://go.dev/doc/articles/race_detector)
- [Go Fuzzing](https://go.dev/doc/fuzz/)
- [Testing Blog](https://go.dev/blog/advanced-testing)

---

**Обновлено:** 1 апреля 2026 г.
**Версия:** 3.29.0+
