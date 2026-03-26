# Установка go-pcap2socks на Linux

## Требования

- Linux (Ubuntu 20.04+, Debian 11+, Fedora 35+, Arch Linux)
- Go 1.21+ (для сборки из исходников)
- Права root/sudo
- Сетевой интерфейс с поддержкой promiscuous mode

## 📦 Варианты установки

### 1. Установка из готового бинарника (рекомендуется)

```bash
# Скачайте архив для вашей архитектуры
wget https://github.com/QuadDarv1ne/go-pcap2socks/releases/latest/download/go-pcap2socks-linux-amd64.tar.gz

# Распакуйте
tar -xzf go-pcap2socks-linux-amd64.tar.gz

# Переместите в системную директорию
sudo mv go-pcap2socks-linux-amd64 /usr/local/bin/go-pcap2socks
sudo chmod +x /usr/local/bin/go-pcap2socks

# Проверка установки
go-pcap2socks --version
```

### 2. Сборка из исходников

```bash
# Установка Go (если не установлен)
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Клонируйте репозиторий
git clone https://github.com/QuadDarv1ne/go-pcap2socks.git
cd go-pcap2socks

# Сборка
go build -o go-pcap2socks .

# Установка
sudo mv go-pcap2socks /usr/local/bin/
sudo chmod +x /usr/local/bin/go-pcap2socks
```

### 3. Использование скрипта сборки

```bash
cd go-pcap2socks
chmod +x build.sh
./build.sh

# Бинарники будут в папке build/
sudo mv build/go-pcap2socks-linux-amd64 /usr/local/bin/go-pcap2socks
```

## 🔧 Настройка

### Создание директории конфигурации

```bash
sudo mkdir -p /etc/go-pcap2socks
sudo cp config.json /etc/go-pcap2socks/
sudo nano /etc/go-pcap2socks/config.json
```

### Пример конфигурации для Linux

```json
{
  "pcap": {
    "interfaceGateway": "192.168.1.1",
    "network": "192.168.1.0/24",
    "localIP": "192.168.1.1",
    "mtu": 1486
  },
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.1.100",
    "poolEnd": "192.168.1.200",
    "leaseDuration": 86400
  },
  "dns": {
    "servers": [
      {"address": "8.8.8.8:53"},
      {"address": "1.1.1.1:53"}
    ]
  },
  "outbounds": [
    {"tag": "", "direct": {}},
    {
      "tag": "socks-proxy",
      "socks": {"address": "proxy.example.com:1080"}
    }
  ],
  "routing": {
    "rules": [
      {"dstPort": "53", "outboundTag": "dns-out"},
      {"dstPort": "443", "outboundTag": "socks-proxy"}
    ]
  },
  "api": {
    "enabled": true,
    "port": 8080,
    "token": "${API_TOKEN}"
  }
}
```

## 🚀 Запуск

### Тестовый запуск

```bash
sudo go-pcap2socks
```

### Установка как сервис (systemd)

```bash
# Скопируйте файл сервиса
sudo cp go-pcap2socks.service /etc/systemd/system/

# Перезагрузите systemd
sudo systemctl daemon-reload

# Включите автозагрузку
sudo systemctl enable go-pcap2socks

# Запустите сервис
sudo systemctl start go-pcap2socks

# Проверка статуса
sudo systemctl status go-pcap2socks
```

### Просмотр логов

```bash
# Через journalctl
sudo journalctl -u go-pcap2socks -f

# Последние 50 строк
sudo journalctl -u go-pcap2socks -n 50
```

## 🔒 Настройка брандмауэра

### UFW (Ubuntu/Debian)

```bash
# Разрешить веб-интерфейс
sudo ufw allow 8080/tcp

# Разрешить DHCP (если используется)
sudo ufw allow 67/udp
sudo ufw allow 68/udp

# Перезапуск UFW
sudo ufw reload
```

### firewalld (Fedora/CentOS)

```bash
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=67/udp
sudo firewall-cmd --permanent --add-port=68/udp
sudo firewall-cmd --reload
```

### iptables

```bash
sudo iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
sudo iptables -A INPUT -p udp --dport 67 -j ACCEPT
sudo iptables -A INPUT -p udp --dport 68 -j ACCEPT

# Сохранение правил
sudo iptables-save > /etc/iptables/rules.v4
```

## 🌐 Настройка сетевого интерфейса

### Включение promiscuous mode

```bash
# Для интерфейса eth0
sudo ip link set eth0 promisc on

# Проверка
ip link show eth0
```

### Настройка IP forwarding

```bash
# Включить IP forwarding
sudo sysctl -w net.ipv4.ip_forward=1

# Постоянное включение
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

## 🔄 Обновление

```bash
# Остановка сервиса
sudo systemctl stop go-pcap2socks

# Скачивание новой версии
wget -O /tmp/go-pcap2socks.tar.gz \
    https://github.com/QuadDarv1ne/go-pcap2socks/releases/latest/download/go-pcap2socks-linux-amd64.tar.gz

# Замена бинарника
sudo tar -xzf /tmp/go-pcap2socks.tar.gz -C /usr/local/bin/ --overwrite

# Запуск
sudo systemctl start go-pcap2socks
sudo systemctl status go-pcap2socks
```

## 🐛 Диагностика

### Проверка работы сервиса

```bash
# Статус
sudo systemctl status go-pcap2socks

# Логи
sudo journalctl -u go-pcap2socks -n 100

# Проверка портов
sudo ss -tlnp | grep 8080
sudo ss -ulnp | grep 67
```

### Тест DHCP

```bash
# На клиентском устройстве
sudo dhclient -v eth0

# Проверка полученного IP
ip addr show eth0
```

### Тест прокси

```bash
# Проверка веб-интерфейса
curl http://localhost:8080/api/status

# С токеном
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:8080/api/status
```

### Отладка

```bash
# Запуск с debug логированием
export SLOG_LEVEL=debug
sudo go-pcap2socks

# Или через systemd
sudo systemctl edit go-pcap2socks
# Добавить: Environment="SLOG_LEVEL=debug"
sudo systemctl restart go-pcap2socks
```

## ⚠️ Возможные проблемы

### "Permission denied" / "Operation not permitted"

→ Запускайте от root или с sudo

### "Failed to open device"

→ Проверьте права на сетевой интерфейс:
```bash
sudo setcap cap_net_raw,cap_net_admin=eip /usr/local/bin/go-pcap2socks
```

### DHCP не работает

→ Проверьте, что нет другого DHCP сервера:
```bash
sudo systemctl stop dnsmasq
sudo systemctl stop dhcpd
```

### Веб-интерфейс недоступен

→ Проверьте брандмауэр:
```bash
sudo ufw status
sudo ss -tlnp | grep 8080
```

## 📚 Дополнительная документация

- [README.md](README.md) - основная документация
- [QUICK_START.md](QUICK_START.md) - быстрый старт
- [SETUP_RU.md](SETUP_RU.md) - настройка устройств
- [SECURITY.md](SECURITY.md) - безопасность

## 📞 Поддержка

При проблемах откройте [Issue](https://github.com/QuadDarv1ne/go-pcap2socks/issues) или проверьте [Troubleshooting](TROUBLESHOOTING.md).
