# Руководство по прокси-сервисам для go-pcap2socks

Это руководство поможет вам выбрать и настроить прокси для работы с go-pcap2socks.

---

## Зачем нужен прокси?

Без прокси go-pcap2socks работает в режиме **direct** — трафик идёт напрямую. Это может не работать, если:

- Серверы PlayStation/Xbox/Nintendo заблокированы в вашем регионе
- Провайдер блокирует игровые серверы
- Нужен обход NAT для мультиплеера
- Требуется смена IP-адреса

---

## 1. Платные прокси-сервисы (рекомендуется)

### Bright Data
| Параметр | Значение |
|---|---|
| **Сайт** | https://brightdata.com |
| **Цена** | от $5/мес |
| **Типы** | SOCKS5, HTTP/HTTPS |
| **Плюсы** | Огромная сеть, высокая скорость, поддержка 24/7 |
| **Минусы** | Дорогой для одного пользователя |

**Настройка в config.json:**
```json
{
  "outbounds": [
    {
      "tag": "proxy-socks5",
      "socks5": {
        "address": "proxy-server.brightdata.com:1080",
        "username": "ваш_логин",
        "password": "ваш_пароль"
      }
    }
  ]
}
```

---

### Smartproxy
| Параметр | Значение |
|---|---|
| **Сайт** | https://smartproxy.com |
| **Цена** | от $7/мес |
| **Типы** | SOCKS5, HTTP/HTTPS |
| **Плюсы** | 65+ млн IP, простая настройка, быстрый отклик |
| **Минусы** | Ограничения по трафику на дешевых планах |

**Настройка:**
```json
{
  "outbounds": [
    {
      "tag": "proxy-socks5",
      "socks5": {
        "address": "gw.smartproxy.com:10001",
        "username": "ваш_логин",
        "password": "ваш_пароль"
      }
    }
  ]
}
```

---

### IPRoyal
| Параметр | Значение |
|---|---|
| **Сайт** | https://iproyal.com |
| **Цена** | от $3/мес |
| **Типы** | SOCKS5, HTTP/HTTPS |
| **Плюсы** | Доступная цена, нет ограничений по трафику, есть пожизненные прокси |
| **Минусы** | Меньше локаций, чем у конкурентов |

**Настройка:**
```json
{
  "outbounds": [
    {
      "tag": "proxy-socks5",
      "socks5": {
        "address": "socks.iproyal.com:11080",
        "username": "ваш_логин",
        "password": "ваш_пароль"
      }
    }
  ]
}
```

---

### Soax
| Параметр | Значение |
|---|---|
| **Сайт** | https://soax.com |
| **Цена** | от $9/мес |
| **Типы** | SOCKS5, HTTP/HTTPS |
| **Плюсы** | Резидентные IP, хорошая поддержка, удобный интерфейс |
| **Минусы** | Выше среднего по цене |

---

## 2. VPN с поддержкой SOCKS5

### NordVPN
| Параметр | Значение |
|---|---|
| **Сайт** | https://nordvpn.com |
| **Цена** | от $3.49/мес (при годовой подписке) |
| **SOCKS5** | Да, `nl.socks.nordvpn.com:1080` |
| **Плюсы** | Дешевле прокси-сервисов, встроенный SOCKS5, 6000+ серверов |

**Настройка:**
```json
{
  "outbounds": [
    {
      "tag": "nordvpn-socks5",
      "socks5": {
        "address": "nl.socks.nordvpn.com:1080",
        "username": "ваш_email_nordvpn",
        "password": "ваш_пароль_nordvpn"
      }
    }
  ]
}
```

---

### Surfshark
| Параметр | Значение |
|---|---|
| **Сайт** | https://surfshark.com |
| **Цена** | от $2.49/мес |
| **SOCKS5** | Да, серверы `nl.surfshark.com:1080` |
| **Плюсы** | Самый дешёвый, неограниченные устройства |

---

## 3. Свой прокси на VPS (для продвинутых)

### Шаг 1: Арендовать VPS

| Провайдер | Цена | Сайт |
|---|---|---|
| **Hetzner** | от €4/мес | https://hetzner.com |
| **DigitalOcean** | от $4/мес | https://digitalocean.com |
| **Vultr** | от $2.50/мес | https://vultr.com |
| **Aeza** | от 200₽/мес | https://aeza.net |

Рекомендации:
- **Локация:** Ближе к вам — меньше задержка
- **ОС:** Ubuntu 22.04 LTS
- **RAM:** минимум 512 МБ
- **Трафик:** не менее 1 ТБ/мес

---

### Шаг 2: Установить 3proxy (SOCKS5)

Подключитесь к VPS по SSH и выполните:

```bash
# Обновить пакеты
sudo apt update && sudo apt upgrade -y

# Установить зависимости
sudo apt install -y build-essential git

# Скачать и собрать 3proxy
cd /tmp
git clone https://github.com/z3APA3A/3proxy.git
cd 3proxy
make -f Makefile.Linux
sudo make -f Makefile.Linux install

# Создать конфигурацию
sudo nano /etc/3proxy/3proxy.cfg
```

Содержимое файла конфигурации:
```
# 3proxy SOCKS5 configuration
daemon
log /var/log/3proxy/3proxy.log D
nserver 8.8.8.8
nserver 1.1.1.1
nscache 65536
timeouts 1 5 30 60 180 1800 15 60

# SOCKS5 прокси
socks -p1080 -a

# Авторизация по логину/паролю
users ваш_пользователь:CL:ваш_пароль

# Разрешить все соединения для авторизованных пользователей
allow ваш_пользователь
```

Запуск:
```bash
sudo mkdir -p /var/log/3proxy
sudo systemctl enable 3proxy
sudo systemctl start 3proxy
sudo systemctl status 3proxy
```

**Настройка в config.json:**
```json
{
  "outbounds": [
    {
      "tag": "my-vps-proxy",
      "socks5": {
        "address": "IP_ВАШЕГО_VPS:1080",
        "username": "ваш_пользователь",
        "password": "ваш_пароль"
      }
    }
  ]
}
```

---

### Альтернатива: Dante SOCKS5

```bash
sudo apt install -y dante-server

sudo nano /etc/danted.conf
```

```
logoutput: /var/log/danted.log

internal: eth0 port = 1080
external: eth0

method: username

client pass {
    from: 0.0.0.0/0 to: 0.0.0.0/0
}

pass {
    from: 0.0.0.0/0 to: 0.0.0.0/0
    protocol: tcp udp
}
```

```bash
sudo systemctl enable danted
sudo systemctl start danted
```

---

## 4. Бесплатные прокси (НЕ рекомендуется для игр)

⚠️ **Предупреждение:** Бесплатные прокси нестабильны, медленны, могут перехватывать ваш трафик.

Источники списков:
- https://spys.one
- https://free-proxy-list.net
- https://www.proxy-list.download

Если всё же хотите попробовать:

```json
{
  "outbounds": [
    {
      "tag": "free-proxy",
      "socks5": {
        "address": "IP:PORT",
        "username": "",
        "password": ""
      }
    }
  ]
}
```

---

## Полная настройка go-pcap2socks с прокси

### Пример config.json с SOCKS5 прокси:

```json
{
  "pcap": {
    "interfaceGateway": "192.168.100.1",
    "mtu": 1472,
    "network": "192.168.100.0/24",
    "localIP": "192.168.100.1",
    "localMAC": "b0:25:aa:65:67:bb"
  },
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.100.101",
    "poolEnd": "192.168.100.201",
    "leaseDuration": 86400
  },
  "dns": {
    "servers": [
      { "address": "8.8.8.8:53" },
      { "address": "1.1.1.1:53" }
    ],
    "useSystemDNS": false
  },
  "routing": {
    "rules": [
      {
        "dstPort": "53",
        "outboundTag": "dns-out"
      },
      {
        "outboundTag": "proxy-socks5"
      }
    ]
  },
  "outbounds": [
    {
      "direct": {},
      "tag": "direct"
    },
    {
      "dns": {},
      "tag": "dns-out"
    },
    {
      "socks5": {
        "address": "proxy.example.com:1080",
        "username": "username",
        "password": "password"
      },
      "tag": "proxy-socks5"
    }
  ],
  "windivert": {
    "enabled": true
  },
  "language": "ru"
}
```

### Ключевые изменения:
1. Добавлен новый outbound `proxy-socks5`
2. В routing добавлено правило: весь трафик → `proxy-socks5`
3. DNS-трафик идёт отдельно через `dns-out` (чтобы DNS разрешался напрямую)

---

## Проверка работы прокси

После настройки запустите программу и проверьте логи:

```bash
.\pcap2socks.exe
```

Ищите в логах:
- ✅ `TCP connection created` — соединения создаются через прокси
- ✅ `proxy connection established` — прокси подключён
- ❌ `proxy connect failed` — ошибка подключения к прокси
- ❌ `authentication failed` — неверный логин/пароль

---

## Часто задаваемые вопросы

### Какой тип прокси выбрать?
- **SOCKS5** — универсальный, поддерживает TCP и UDP, рекомендуется для игр
- **HTTP/HTTPS** — только HTTP-трафик, не подходит для всех игровых протоколов

### Нужна ли авторизация?
Да, всегда используйте логин/пароль. Открытые прокси могут быть перехвачены.

### Можно ли использовать несколько прокси?
Да, добавьте несколько outbound'ов и настройте правила маршрутизации.

### Какой сервер выбрать?
Выбирайте сервер **ближе к вам** — это снизит задержку. Для игр важна пинг < 100мс.

---

## Поддержка

Если возникли проблемы:
- Проверьте логи: `go-pcap2socks.log`
- Убедитесь, что прокси доступен: `Test-NetConnection proxy.example.com -Port 1080`
- Проверьте логин/пароль
- Попробуйте другой сервер
