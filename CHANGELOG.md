# Changelog

Все заметные изменения в этом проекте будут задокументированы в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и этот проект придерживается [Semantic Versioning](https://semver.org/lang/ru/).

## [3.19.2] - 2026-03-24

### Исправлено
- **proxy/router.go** - routeCache.hits/misses теперь atomic.Uint64
- **proxy/router.go** - routeCache.stats() использует atomic.Load()
- **proxy/router.go** - routeCache.get() использует atomic.Add() для счётчиков
- **proxy/router_test.go** - TestRouteCache_Concurrency исправлен (cleanup отдельно)
- **proxy/group_test.go** - TestSelectProxy_Failover исправлен (atomic для activeIndex)

---

## [3.19.1] - 2026-03-24

### Исправлено
- **dhcp.Marshal()** - добавлен magic cookie (байты 236-239: 99,130,83,99)
- **dhcp.Marshal()** - детерминированный порядок опций (Message Type, Server ID, Subnet, Router, DNS, Lease Time)
- **dhcp.Marshal()** - правильная обработка ServerHostname и BootFileName
- **windivert.processPacket()** - исправлена проверка портов (srcPort=68 && dstPort=67)
- **windivert.sendDHCPResponse()** - правильный выбор destination IP (clientIP/yourIP/broadcast)

---

## [3.19.0] - 2026-03-24

### Добавлено
- **HTTP/3 UDP proxying** через QUIC datagrams (RFC 9221)
- **HTTP/3 TCP proxying** через CONNECT туннель над QUIC streams
- **proxy/http3_datagram.go** - net.PacketConn over QUIC datagrams
- **proxy/http3_conn.go** - net.Conn wrapper для QUIC streams
- **Интеграция с ProxyGroup** - Failover, RoundRobin, LeastLoad для HTTP/3
- **Unit-тесты для HTTP/3** - 8 тестов, все проходят
- **Пример конфигурации** config-http3.json

### Изменено
- Router Match: 5.896 ns/op (целевые <10ns) ✅
- Router DialContext: 99.47 ns/op (целевые <100ns) ✅
- Router Cache Hit: 155.3 ns/op (целевые <200ns) ✅
- Buffer GetPut: 47.64 ns/op (целевые <50ns) ✅
- Размер бинарника: 15.6MB (норма <25MB)

### Исправлено
- Обновлены метрики производительности в todo.md
- Синхронизированы ветки dev/main (66e5ed6)

---

## [Неопубликовано]

### Добавлено
- Интеграционные тесты для DHCP сервера
- Benchmark comparison в CI workflow
- Unit-тесты для service, discord, telegram модулей

### Изменено
- CI/CD workflow теперь запускает все тесты перед сборкой

---

## [3.18.0] - 2026-03-23

### Добавлено
- **Metadata pool** для снижения аллокаций в tunnel/proxy (2.8x быстрее)
- **gVisor stack tuning** - оптимизированы размеры TCP буферов
- **Async DNS resolver** с context timeout и non-blocking exchange
- **Connection tracking оптимизация** - sync.Pool для DeviceStats
- **Router DialContext оптимизация** - byte slice key (6→3 allocs/op)
- **Metrics Prometheus** - endpoint `/metrics` для мониторинга
- **HTTP/2 connection pooling** - shared transport в dialer
- **Adaptive buffer sizing** - пулы 512B/2KB/8KB
- **Zero-copy UDP** - DecodeUDPPacketInPlace в transport/socks5.go
- **DNS connection pooling** - dns/pool.go с кэшированием соединений
- **Ошибки без аллокаций** - ErrBlockedByMACFilter, ErrProxyNotFound
- **Rate limiting для логов** - ratelimit/limiter.go
- **Асинхронное логирование** - asynclogger/async_handler.go

### Изменено
- Router Match: 7.72 → 4.38 ns/op (**-43%**)
- Router DialContext: 143.1 → 96.93 ns/op (**-32%**)
- Router Cache Hit: 369.4 → 160.3 ns/op (**-57%**)
- Аллокации: 6 → 3 allocs/op (**-50%**)
- Metadata: 37.45 → 13.15 ns/op (**-65%**)

### Исправлено
- Дублирование кода в stats/store.go
- Указатель на dns.Conn в dns/pool.go
- Helper функции в api/server_test.go
- Импорты и методы в profiles/manager_test.go

---

## [3.17.0] - 2026-03-20

### Добавлено
- **LRU кэш маршрутизации** на 10,000 записей с TTL 60 сек
- **Бенчмарки** для router, tcp, dhcp, config
- **Оптимизация буферов DHCP** - pool.Get/Put
- **Замена panic на возврат ошибок** в cfg/config.go

### Изменено
- Ускорение маршрутизации в 4.4x при cache hit
- Снижение аллокаций DHCP на ~30%
- Приложение не падает при невалидной конфигурации

### Исправлено
- Ошибка "The parameter is incorrect" в WinDivert DHCP сервере
- Неправильная структура пакетов (теперь только IP+UDP+DHCP без Ethernet)

---

## [3.16.0] - 2026-03-15

### Добавлено
- **WinDivert DHCP сервер** - альтернативный режим работы
- **UPnP менеджер** с авто-пробросом портов для игровых консолей
- **Пресеты UPnP** для PS4, PS5, Xbox, Switch
- **Web UI** - панель управления на порту 8080
- **REST API** - endpoints для статуса, трафика, устройств, логов
- **WebSocket** для реального времени в Web UI
- **Телеграм бот** с командами /status, /traffic, /devices
- **Discord webhook** для уведомлений о событиях
- **Горячие клавиши** - Ctrl+Alt+P для переключения прокси
- **Менеджер профилей** - сохранение и загрузка конфигураций
- **MAC фильтр** - блокировка/разрешение устройств по MAC адресу
- **Асинхронный logger handler** для производительности

### Изменено
- Улучшен Web UI с тёмной/светлой темой
- Оптимизирован роутер с кэшированием маршрутов
- Улучшена обработка ошибок в service package

### Исправлено
- Утечка памяти в stats.Store
- Гонка данных в proxy.Router
- Ошибка закрытия устройства при shutdown

---

## [3.15.0] - 2026-03-10

### Добавлено
- **DHCP сервер** с пулом адресов и арендой
- **ARP монитор** для отслеживания устройств в сети
- **Статистика трафика** по устройствам
- **Auto-config команда** для автоматической настройки
- **Service package** для установки как сервис Windows
- **Install/uninstall/start/stop** команды для сервиса
- **Event log** интеграция для Windows сервиса

### Изменено
- Улучшена маршрутизация DNS трафика
- Оптимизировано потребление памяти при высокой нагрузке

---

## [3.14.0] - 2026-03-05

### Добавлено
- **SOCKS5 с fallback** - автоматический переход при ошибке
- **Proxy groups** с политиками failover, round-robin, least-load
- **Health checks** для проверки доступности прокси
- **Маршрутизация по правилам** - IP, порты, протоколы
- **DNS прокси** - отдельный outbound для DNS
- **Reject/Direct** режимы для блокировки/прямого подключения

### Изменено
- Улучшена обработка ошибок SOCKS5
- Оптимизировано переподключение при разрыве

---

## [3.13.0] - 2026-02-28

### Добавлено
- **gVisor стек** для работы с сетевыми пакетами
- **WinDivert** для перехвата пакетов на Windows
- **Базовая маршрутизация** трафика
- **Конфигурация JSON** с валидацией
- **Логирование** через slog

---

## [0.1.0] - 2024-XX-XX

### Добавлено
- Первый релиз go-pcap2socks
- Базовая функциональность прокси
- Поддержка SOCKS5
- Простая конфигурация

---

## Типы изменений

- **Добавлено** — новые функции
- **Изменено** — изменения в существующих функциях
- **Удалено** — удалённые функции
- **Исправлено** — исправления ошибок
- **Безопасность** — исправления уязвимостей

---

## Метрики производительности

### Версия 3.18.0
```
Router Match:         4.38 ns/op    0 B/op    0 allocs/op
Router DialContext:   96.93 ns/op   88 B/op   3 allocs/op
Router Cache Hit:     160.3 ns/op   88 B/op   3 allocs/op
Buffer GetPut:        42.74 ns/op   24 B/op   1 allocs/op
DNS Cache Get:        98.49 ns/op   0 B/op    0 allocs/op
Metrics Record:       8.88 ns/op    0 B/op    0 allocs/op
Metadata Pool:        13.15 ns/op   16 B/op   1 allocs/op
```

### Версия 3.17.0
```
Router Match:         7.72 ns/op    0 B/op    0 allocs/op
Router DialContext:   143.1 ns/op   136 B/op  6 allocs/op
Router Cache Hit:     369.4 ns/op   168 B/op  5 allocs/op
```

---

*Последнее обновление: 23 марта 2026 г.*
