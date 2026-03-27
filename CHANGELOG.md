# Changelog

Все заметные изменения в этом проекте будут задокументированы в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и этот проект придерживается [Semantic Versioning](https://semver.org/lang/ru/).

## [3.19.19+] - 2026-03-27

### Добавлено
- **deps/README.md** - полная документация по зависимостям (Npcap, WinDivert)
- **deps/.gitignore** - игнорирование бинарных файлов драйверов
- **windivert/dhcp_server.go** - Smart DHCP поддержка через WithSmartDHCP()
- **dhcp/server.go** - Smart DHCP Manager с определением устройств по MAC
- **auto/smart_dhcp.go** - распределение IP по типам устройств (PS4/PS5/Xbox/Switch)
- **npcap_dhcp/simple_server.go** - расширенные логи DHCP (payload, options, message types)
- **main.go** - checkWindowsICSConflict() для обнаружения конфликта с Windows ICS
- **main.go** - findAvailablePort() для авто-выбора свободного порта

### Исправлено
- **dhcp_server_windows.go** - переключено на WinDivert DHCP сервер (вместо Npcap)
- **npcap_dhcp/simple_server.go** - парсинг DHCP опций с проверкой magic cookie
- **npcap_dhcp/simple_server.go** - обработка messageType=0 (без Option 53)
- **npcap_dhcp/simple_server.go** - отправка DHCP OFFER на unicast IP (вместо broadcast)
- **main.go** - порт 8080 теперь проверяется на занятость

### Изменено
- **dhcp/server.go** - добавлен smartDHCP и deviceProfiles в структуру Server
- **dhcp/server.go** - allocateIP() использует Smart DHCP для определения IP по типу устройства
- **windivert/dhcp_server.go** - NewDHCPServer() с параметром enableSmartDHCP
- **.gitignore** - игнорирование deps/*.exe, deps/*.zip, deps/*/WinDivert*.sys

### Улучшения DHCP
- ✅ Определение устройства по MAC (OUI база: Sony PS4/PS5, Microsoft Xbox, Nintendo Switch)
- ✅ Smart DHCP: PS4/PS5 (.100-.119), Xbox (.120-.139), Switch (.140-.149), PC (.150-.199)
- ✅ Расширенное логирование: messageType, vendorClass, hostname, options
- ✅ WinDivert для отправки пакетов (уровень ядра, максимальная совместимость)
- ✅ Проверка Windows ICS и рекомендации по отключению

### Технические детали
- **WinDivert**: Отправка DHCP пакетов через ядро (вместо Npcap userspace)
- **Smart DHCP**: Авто-распределение IP по типам устройств
- **Logging**: Детальная трассировка DHCP (payload, options, send/receive)
- **Port selection**: Авто-выбор порта если 8080 занят

---

## [3.19.12+] - 2026-03-26

### Исправлено (10 критических ошибок)
| # | Ошибка | Файл | Статус |
|---|--------|------|--------|
| 1 | Toast уведомления (PowerShell XML errors) | notify/notify.go | ✅ |
| 2 | Лишние уведомления от команд службы | main.go | ✅ |
| 3 | Обработка ошибок инициализации | main.go | ✅ |
| 4 | Graceful shutdown | main.go | ✅ |
| 5 | Защита от panic | main.go | ✅ |
| 6 | Обработка ошибок DHCP | npcap_dhcp/simple_server.go | ✅ |
| 7 | Восстановление packetLoop | npcap_dhcp/simple_server.go | ✅ |
| 8 | Защита от DHCP flood | npcap_dhcp/simple_server.go | ✅ |
| 9 | Чтение DHCP опций | npcap_dhcp/simple_server.go | ✅ |
| 10 | Утечки ресурсов при shutdown | main.go | ✅ |

### Добавлено (10 новых возможностей)
| # | Возможность | Файл | Описание |
|---|-------------|------|----------|
| 1 | Чтение DHCP Option 12 | npcap_dhcp/simple_server.go | Host Name |
| 2 | Чтение DHCP Option 53 | npcap_dhcp/simple_server.go | Message Type |
| 3 | Чтение DHCP Option 55 | npcap_dhcp/simple_server.go | Parameter Request List |
| 4 | Чтение DHCP Option 60 | npcap_dhcp/simple_server.go | Vendor Class Identifier |
| 5 | Чтение DHCP Option 61 | npcap_dhcp/simple_server.go | Client Identifier |
| 6 | Сохранение имён хостов | npcap_dhcp/simple_server.go | В Lease структуре |
| 7 | API для имён хостов | stats/store.go | Метод SetHostname |
| 8 | Авто-восстановление DHCP | npcap_dhcp/simple_server.go | При max errors |
| 9 | Улучшенный packetLoop | npcap_dhcp/simple_server.go | С обработкой ошибок |
| 10 | Логирование DHCP | npcap_dhcp/simple_server.go | С именами хостов |

### Улучшения инфраструктуры
- **run.bat** — улучшенный запуск с проверками Npcap и прав администратора
- **build-clean.bat** — скрипт чистой сборки с оптимизацией размера (~17.4 MB)
- **Улучшено логирование** — version, pid при запуске
- **Расширена Lease структура** — Hostname, VendorClass, ParameterList

### Статистика изменений
- **Изменено файлов:** 8
- **Добавлено строк:** ~300
- **Изменено строк:** ~200
- **Удалено строк:** ~50
- **Размер бинарника:** 24.6 MB → 17.4 MB (-29%)

### Результаты тестирования
- ✅ Компиляция без ошибок
- ✅ Все тесты проходят (auto, dhcp, proxy, api)
- ✅ Graceful shutdown работает
- ✅ DHCP server восстанавливается при ошибках
- ✅ Toast уведомления не вызывают ошибок

---

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
