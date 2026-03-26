# 🤖 Roadmap: Ультимативная Автоматизация go-pcap2socks

## 📋 Философия

**Принцип**: Пользователь запускает программу — всё работает автоматически.

**Цель**: 100% автоматизация без необходимости ручной настройки конфигурации.

---

## 🎯 Уровень 1: Базовая автоматизация (✅ УЖЕ РЕАЛИЗОВАНО)

### Что уже работает:
- ✅ `auto-config` команда — авто-определение сетевого интерфейса
- ✅ Авто-определение IP/MAC/MTU/сети
- ✅ Авто-генерация config.json
- ✅ Системные DNS серверы
- ✅ DHCP пул адресов
- ✅ WinDivert/Npcap движки

### Ограничения:
- ❌ Ручной выбор движка (WinDivert vs Npcap)
- ❌ Нет определения типа устройств
- ❌ Статические параметры без адаптации
- ❌ Нет failover при ошибках

---

## 🚀 Уровень 2: Smart Device Detection (В РАЗРАБОТКЕ)

### 2.1 Автоматическое определение типа устройства

**Задача**: По MAC-адресу определять тип устройства и применять оптимизации.

```go
// auto/device_detector.go
package auto

// DeviceType - тип устройства
type DeviceType string

const (
    DevicePS4       DeviceType = "ps4"
    DevicePS5       DeviceType = "ps5"
    DeviceXbox      DeviceType = "xbox"
    DeviceXboxOne   DeviceType = "xbox_one"
    DeviceXboxSX    DeviceType = "xbox_series"
    DeviceSwitch    DeviceType = "switch"
    DevicePC        DeviceType = "pc"
    DevicePhone     DeviceType = "phone"
    DeviceTablet    DeviceType = "tablet"
    DeviceRobot     DeviceType = "robot"  // IoT, пылесосы и т.д.
    DeviceUnknown   DeviceType = "unknown"
)

// DeviceProfile - профиль устройства с оптимизациями
type DeviceProfile struct {
    Type           DeviceType
    Manufacturer   string
    RequiredPorts  []int           // Порты для UPnP
    MTU            int             // Рекомендуемый MTU
    TCPQuirks      bool            // Специфичные TCP настройки
    UDPQuirks      bool            // Специфичные UDP настройки
    ProxyMode      string          // Рекомендуемый режим прокси
    Priority       int             // Приоритет трафика
}

// OUI Database - первые 3 байта MAC адреса
var ouiDatabase = map[string]DeviceProfile{
    // Sony
    "00:9D:6B": {Type: DevicePS4, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}},
    "00:D9:D1": {Type: DevicePS4, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}},
    "34:CD:66": {Type: DevicePS5, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}, MTU: 1473},
    
    // Microsoft
    "E8:4E:22": {Type: DeviceXboxSX, Manufacturer: "Microsoft", RequiredPorts: []int{3074, 3075}},
    "B4:7C:9C": {Type: DeviceXboxOne, Manufacturer: "Microsoft", RequiredPorts: []int{3074, 3075}},
    "00:25:5C": {Type: DeviceXbox, Manufacturer: "Microsoft", RequiredPorts: []int{3074}},
    
    // Nintendo
    "F8:89:32": {Type: DeviceSwitch, Manufacturer: "Nintendo", RequiredPorts: []int{12400, 12401, 12402}},
    "04:94:53": {Type: DeviceSwitch, Manufacturer: "Nintendo", RequiredPorts: []int{12400, 12401, 12402}},
    
    // PC производители
    "A4:BB:6D": {Type: DevicePC, Manufacturer: "Dell"},
    "B8:07:51": {Type: DevicePC, Manufacturer: "HP"},
    "DC:EA:38": {Type: DevicePC, Manufacturer: "Intel"},
    
    // Телефоны
    "A4:83:E7": {Type: DevicePhone, Manufacturer: "Apple"},
    "F4:5C:89": {Type: DevicePhone, Manufacturer: "Apple"},
    "2C:54:CF": {Type: DevicePhone, Manufacturer: "Samsung"},
}

// DetectByMAC определяет тип устройства по MAC адресу
func DetectByMAC(mac string) DeviceProfile {
    // Нормализация MAC (верхний регистр, двоеточия)
    mac = strings.ToUpper(strings.ReplaceAll(mac, "-", ":"))
    
    // Первые 8 символов (3 байта OUI)
    if len(mac) >= 8 {
        oui := mac[:8]
        if profile, ok := ouiDatabase[oui]; ok {
            return profile
        }
    }
    
    return DeviceProfile{Type: DeviceUnknown}
}

// AutoApplyProfile применяет профиль устройства
func AutoApplyProfile(profile DeviceProfile, config *cfg.Config) {
    // Применение MTU
    if profile.MTU > 0 {
        config.PCAP.MTU = uint32(profile.MTU)
    }
    
    // Применение UPnP портов
    if len(profile.RequiredPorts) > 0 {
        if config.UPnP == nil {
            config.UPnP = &cfg.UPnPConfig{Enabled: true}
        }
        config.UPnP.Enabled = true
        config.UPnP.AutoForward = true
        // Добавление портов в пресеты
    }
    
    // Применение режима прокси
    if profile.ProxyMode != "" {
        // Настройка routing rules
    }
}
```

### 2.2 Интеграция в main.go

```go
// В функции autoConfigure():
deviceProfile := auto.DetectByMAC(interfaceConfig.MAC)
if deviceProfile.Type != auto.DeviceUnknown {
    slog.Info("Устройство определено", 
        "type", deviceProfile.Type,
        "manufacturer", deviceProfile.Manufacturer)
    auto.AutoApplyProfile(deviceProfile, config)
}
```

---

## 🔧 Уровень 3: Адаптивный выбор движка (ENGINE AUTO-SELECT)

### 3.1 Система оценки движков

**Задача**: Автоматически выбирать лучший движок для текущей системы.

```go
// auto/engine_selector.go
package auto

// EngineType - тип движка
type EngineType string

const (
    EngineWinDivert EngineType = "windivert"
    EngineNpcap     EngineType = "npcap"
    EngineNative    EngineType = "native"  // Встроенные возможности ОС
)

// EngineScore - оценка движка
type EngineScore struct {
    Type       EngineType
    Score      int
    Available  bool
    Latency    time.Duration
    Throughput int64
    Stability  float64  // 0.0-1.0
}

// SelectBestEngine выбирает лучший движок
func SelectBestEngine() EngineType {
    scores := []EngineScore{}
    
    // Оценка WinDivert
    if score := evaluateWinDivert(); score.Available {
        scores = append(scores, score)
    }
    
    // Оценка Npcap
    if score := evaluateNpcap(); score.Available {
        scores = append(scores, score)
    }
    
    // Оценка Native
    if score := evaluateNative(); score.Available {
        scores = append(scores, score)
    }
    
    // Выбор движка с максимальным score
    var best EngineScore
    for _, s := range scores {
        if s.Score > best.Score {
            best = s
        }
    }
    
    return best.Type
}

// evaluateWinDivert оценивает WinDivert
func evaluateWinDivert() EngineScore {
    score := EngineScore{Type: EngineWinDivert, Score: 0}
    
    // Проверка доступности драйвера
    if !windivert.IsAvailable() {
        return score
    }
    score.Available = true
    score.Score += 100
    
    // Проверка прав администратора
    if !svc.IsAdmin() {
        score.Score -= 50  // Требуется админ
    }
    
    // Тест производительности (быстрый benchmark)
    latency := windivert.BenchmarkLatency()
    score.Latency = latency
    if latency < 1*time.Millisecond {
        score.Score += 50
    } else if latency < 5*time.Millisecond {
        score.Score += 30
    }
    
    // Стабильность (история ошибок)
    score.Stability = getWinDivertStabilityHistory()
    score.Score += int(score.Stability * 100)
    
    return score
}

// evaluateNpcap оценивает Npcap
func evaluateNpcap() EngineScore {
    score := EngineScore{Type: EngineNpcap, Score: 0}
    
    // Проверка доступности
    if !npcap.IsAvailable() {
        return score
    }
    score.Available = true
    score.Score += 100
    
    // Проверка режима (promiscuous)
    if npcap.SupportsPromiscuous() {
        score.Score += 20
    }
    
    // Тест производительности
    throughput := npcap.BenchmarkThroughput()
    score.Throughput = throughput
    if throughput > 500_000_000 {  // 500 Mbps
        score.Score += 50
    }
    
    return score
}
```

### 3.2 Конфигурация с авто-выбором

```go
// config.json
{
  "engine": {
    "mode": "auto",  // "auto", "windivert", "npcap", "native"
    "fallback": true,  // Переключаться при ошибках
    "preferences": ["windivert", "npcap"]  // Порядок предпочтения
  }
}
```

---

## ⚡ Уровень 4: Динамическая оптимизация параметров

### 4.1 Auto-Tuning буферов и таймаутов

```go
// auto/tuner.go
package auto

// SystemResources - текущие ресурсы системы
type SystemResources struct {
    CPUCount       int
    TotalMemory    uint64
    AvailableMemory uint64
    NetworkSpeed   int64  // Mbps
    ActiveConnections int
}

// TuningConfig - оптимальные настройки
type TuningConfig struct {
    TCPBufferSize    int
    UDPBufferSize    int
    MaxConnections   int
    ConnectionTimeout time.Duration
    GCPressure       string  // "low", "medium", "high"
}

// AutoTune подбирает оптимальные параметры
func AutoTune() TuningConfig {
    resources := detectSystemResources()
    
    config := TuningConfig{}
    
    // Расчёт TCP буфера на основе памяти
    if resources.AvailableMemory > 8*GB {
        config.TCPBufferSize = 65536  // 64KB
    } else if resources.AvailableMemory > 4*GB {
        config.TCPBufferSize = 32768  // 32KB
    } else {
        config.TCPBufferSize = 16384  // 16KB
    }
    
    // Расчёт UDP буфера на основе скорости сети
    if resources.NetworkSpeed > 1000 {  // 1 Gbps
        config.UDPBufferSize = 65536
    } else if resources.NetworkSpeed > 100 {  // 100 Mbps
        config.UDPBufferSize = 32768
    } else {
        config.UDPBufferSize = 16384
    }
    
    // Максимум подключений на основе CPU
    config.MaxConnections = resources.CPUCount * 100
    
    // Таймаут на основе нагрузки
    if resources.ActiveConnections > config.MaxConnections/2 {
        config.ConnectionTimeout = 30 * time.Second
    } else {
        config.ConnectionTimeout = 60 * time.Second
    }
    
    return config
}
```

### 4.2 Адаптивный MTU

```go
// auto/mtu_optimizer.go

// OptimizeMTU подбирает оптимальный MTU
func OptimizeMTU(interfaceName string) int {
    baseMTU := getInterfaceMTU(interfaceName)
    
    // Тестирование PMTUD (Path MTU Discovery)
    bestMTU := baseMTU
    
    // Тест с разными размерами пакетов
    testSizes := []int{1500, 1492, 1480, 1473, 1454}
    for _, mtu := range testSizes {
        if mtu <= baseMTU && testMTU(interfaceName, mtu) {
            bestMTU = mtu
            break
        }
    }
    
    return bestMTU - 14  // Вычитаем Ethernet заголовок
}

// testMTU проверяет работу с данным MTU
func testMTU(iface string, mtu int) bool {
    // Отправка тестового пакета
    // Проверка на фрагментацию
    // Возврат true если успешно
}
```

---

## 🔄 Уровень 5: Failover движков

### 5.1 Автоматическое переключение при ошибках

```go
// auto/failover.go

// EngineFailover управляет переключением движков
type EngineFailover struct {
    mu           sync.Mutex
    currentEngine EngineType
    healthChecks  map[EngineType]*HealthStatus
    switchCount   int
}

// HealthStatus - статус здоровья движка
type HealthStatus struct {
    LastError     error
    ErrorCount    int
    SuccessCount  int
    LastCheck     time.Time
    IsHealthy     bool
}

// CheckAndSwitch проверяет здоровье и переключает при необходимости
func (f *EngineFailover) CheckAndSwitch() {
    f.mu.Lock()
    defer f.mu.Unlock()
    
    currentHealth := f.healthChecks[f.currentEngine]
    
    // Если текущий движок нездоров
    if !currentHealth.IsHealthy {
        slog.Warn("Engine unhealthy, searching fallback",
            "current", f.currentEngine,
            "errors", currentHealth.ErrorCount)
        
        // Поиск здорового движка
        for engine, health := range f.healthChecks {
            if health.IsHealthy && engine != f.currentEngine {
                slog.Info("Switching engine",
                    "from", f.currentEngine,
                    "to", engine)
                
                f.switchEngine(engine)
                return
            }
        }
        
        // Нет здоровых движков
        slog.Error("No healthy engines available!")
    }
}

// switchEngine переключает движок
func (f *EngineFailover) switchEngine(newEngine EngineType) {
    // Остановка текущего движка
    stopCurrentEngine()
    
    // Запуск нового
    startEngine(newEngine)
    
    f.currentEngine = newEngine
    f.switchCount++
    
    // Уведомление пользователя
    notify.Show("Движок переключен", 
        fmt.Sprintf("Переключено на %s", newEngine),
        notify.NotifyInfo)
}
```

### 5.2 Интеграция в main.go

```go
// В main():
failover := auto.NewEngineFailover()

// Периодическая проверка здоровья
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        failover.CheckAndSwitch()
    }
}()

// Запуск с failover
err = runWithFailover(config, failover)
```

---

## 🧠 Уровень 6: Smart DHCP

### 6.1 Статические IP для известных устройств

```go
// auto/smart_dhcp.go

// StaticLease - статическая аренда
type StaticLease struct {
    MAC        string
    IP         string
    DeviceName string
    DeviceType string
}

// SmartDHCPManager управляет умным DHCP
type SmartDHCPManager struct {
    mu           sync.RWMutex
    staticLeases map[string]*StaticLease
    knownDevices map[string]*DeviceProfile
    leaseHistory map[string][]time.Time
}

// GetIPForDevice возвращает IP для устройства
func (m *SmartDHCPManager) GetIPForDevice(mac string) string {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    // Проверка статической аренды
    if lease, ok := m.staticLeases[mac]; ok {
        return lease.IP
    }
    
    // Проверка известных устройств
    if profile, ok := m.knownDevices[mac]; ok {
        // Выделение IP из диапазона для типа устройства
        return m.allocateIPForType(profile.Type)
    }
    
    // Динамическое выделение
    return m.allocateDynamicIP()
}

// allocateIPForType выделяет IP по типу устройства
func (m *SmartDHCPManager) allocateIPForType(deviceType DeviceType) string {
    // PS4/PS5: 192.168.137.100-119
    // Xbox: 192.168.137.120-139
    // Switch: 192.168.137.140-149
    // PC: 192.168.137.150-199
    // Другие: 192.168.137.200-250
}
```

### 6.2 Авто-обнаружение игровых консолей

```go
// Обнаружение по поведению
func detectGamingConsole(mac string, trafficPattern TrafficPattern) DeviceType {
    // PS4/PS5: Активное использование портов 3478-3480 (UDP/TCP)
    // Xbox: Порт 3074 (UDP/TCP)
    // Switch: Порты 12400-12402 (UDP)
    
    if trafficPattern.UsesPort(3074) {
        return DeviceXbox
    }
    if trafficPattern.UsesPorts(3478, 3479, 3480) {
        return DevicePS5  // Или PS4
    }
    if trafficPattern.UsesPorts(12400, 12401, 12402) {
        return DeviceSwitch
    }
    
    return DeviceUnknown
}
```

---

## 🌐 Уровень 7: Авто-определение режима прокси

### 7.1 Smart Proxy Selection

```go
// auto/proxy_selector.go

// ProxyMode - режим прокси
type ProxyMode string

const (
    ModeDirect   ProxyMode = "direct"
    ModeSocks5   ProxyMode = "socks5"
    ModeHTTP3    ProxyMode = "http3"
    ModeWireGuard ProxyMode = "wireguard"
)

// ProxyRecommendation - рекомендация прокси
type ProxyRecommendation struct {
    Mode       ProxyMode
    Confidence float64  // 0.0-1.0
    Reason     string
    Config     *cfg.Outbound
}

// RecommendProxy рекомендует режим прокси
func RecommendProxy(config *cfg.Config, networkStats *stats.NetworkStats) ProxyRecommendation {
    // Анализ доступных прокси
    availableProxies := detectAvailableProxies()
    
    // Тестирование скорости
    speeds := testProxySpeeds(availableProxies)
    
    // Выбор лучшего
    var best ProxyRecommendation
    
    for _, proxy := range availableProxies {
        rec := ProxyRecommendation{
            Confidence: calculateConfidence(proxy, speeds[proxy]),
            Config:     proxy.Config,
        }
        
        if rec.Confidence > best.Confidence {
            best = rec
        }
    }
    
    return best
}

// detectAvailableProxies обнаруживает доступные прокси
func detectAvailableProxies() []ProxyCandidate {
    candidates := []ProxyCandidate{}
    
    // Проверка SOCKS5 (локальный прокси-сервер)
    if socksAddr := checkLocalSocks5(); socksAddr != "" {
        candidates = append(candidates, ProxyCandidate{
            Mode: ModeSocks5,
            Address: socksAddr,
        })
    }
    
    // Проверка HTTP3 (QUIC)
    if http3Addr := checkLocalHTTP3(); http3Addr != "" {
        candidates = append(candidates, ProxyCandidate{
            Mode: ModeHTTP3,
            Address: http3Addr,
        })
    }
    
    // Проверка WireGuard
    if wgConfig := checkWireGuard(); wgConfig != nil {
        candidates = append(candidates, ProxyCandidate{
            Mode: ModeWireGuard,
            Config: wgConfig,
        })
    }
    
    return candidates
}
```

### 7.2 Авто-конфигурация прокси

```go
// В autoConfigure():
proxyRec := auto.RecommendProxy(config, networkStats)

if proxyRec.Confidence > 0.7 {
    config.Outbounds = append(config.Outbounds, proxyRec.Config)
    config.Routing.Rules = append(config.Routing.Rules, 
        cfg.Rule{
            DstPort: "53",
            OutboundTag: proxyRec.Config.Tag,
        })
    
    slog.Info("Proxy auto-configured",
        "mode", proxyRec.Mode,
        "confidence", proxyRec.Confidence,
        "reason", proxyRec.Reason)
}
```

---

## 📊 Сводная таблица автоматизации

| Функция | Статус | Приоритет | Сложность |
|---------|--------|-----------|-----------|
| Device Detection по MAC | 🔴 В работе | Высокий | Средняя |
| Engine Auto-Select | ⚪ Запланировано | Высокий | Высокая |
| Dynamic Buffer Tuning | ⚪ Запланировано | Средний | Средняя |
| Adaptive MTU | ⚪ Запланировано | Средний | Низкая |
| Engine Failover | ⚪ Запланировано | Высокий | Высокая |
| Smart DHCP (static IP) | ⚪ Запланировано | Средний | Средняя |
| Gaming Console Detection | ⚪ Запланировано | Низкий | Средняя |
| Proxy Auto-Selection | ⚪ Запланировано | Низкий | Высокая |

---

## 🎯 Итоговая цель

**Пользовательский опыт v4.0:**

1. Пользователь скачивает go-pcap2socks
2. Запускает `go-pcap2socks.exe` (без аргументов)
3. Программа автоматически:
   - Определяет лучший сетевой интерфейс
   - Выбирает оптимальный движок (WinDivert/Npcap)
   - Определяет тип устройства (PS4/PS5/Xbox/Switch)
   - Применяет профиль оптимизаций
   - Выделяет статический IP для консоли
   - Настраивает UPnP для нужных портов
   - Подбирает MTU/буферы/таймауты
   - Запускается с оптимальными параметрами

4. Всё работает **из коробки** 🎉

---

## 📝 Следующие шаги

1. **Реализовать Device Detection** (приоритет)
2. **Добавить Engine Auto-Select**
3. **Интегрировать в auto-config**
4. **Протестировать на реальных устройствах**
5. **Документировать для пользователя**
