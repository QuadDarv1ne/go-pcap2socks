# Настройка Telegram и Discord уведомлений

## Telegram Бот

### Создание бота

1. Откройте [@BotFather](https://t.me/BotFather) в Telegram
2. Отправьте команду `/newbot`
3. Введите имя бота (например, `go-pcap2socks Bot`)
4. Введите username бота (должен заканчиваться на `bot`, например `go_pcap2socks_bot`)
5. BotFather выдаст токен бота (выглядит как `1234567890:ABCdefGHIjklMNOpqrsTUVwxyz`)

### Получение Chat ID

1. Откройте вашего бота в Telegram и нажмите `/start`
2. Отправьте сообщение [@userinfobot](https://t.me/userinfobot) или перейдите на https://t.me/getmyid_bot
3. Скопируйте ваш Chat ID (выглядит как `123456789`)

### Настройка в go-pcap2socks

Откройте `config.json` и добавьте:

```json
{
  "telegram": {
    "token": "1234567890:ABCdefGHIjklMNOpqrsTUVwxyz",
    "chat_id": "123456789"
  }
}
```

### Команды бота

| Команда | Описание |
|---------|----------|
| `/start` | Начать работу с ботом |
| `/help` | Показать список команд |
| `/status` | Показать текущий статус сервиса |
| `/start_service` | Запустить сервис |
| `/stop_service` | Остановить сервис |
| `/traffic` | Показать статистику трафика |
| `/devices` | Показать подключенные устройства |

### Уведомления

Бот автоматически отправляет уведомления о:
- ✅ Запуске сервиса
- 🔴 Остановке сервиса
- 📱 Подключении новых устройств
- ⚠️ Ошибках и предупреждениях

---

## Discord Webhook

### Создание webhook

1. Откройте настройки Discord сервера
2. Перейдите в канал, куда хотите отправлять уведомления
3. Нажмите ⚙️ (Edit Channel)
4. Выберите **Integrations** → **Webhooks** → **New Webhook**
5. Настройте webhook:
   - Name: `go-pcap2socks`
   - Avatar: (опционально)
6. Нажмите **Copy Webhook URL**

URL выглядит как:
```
https://discord.com/api/webhooks/123456789012345678/ABCdefGHIjklMNOpqrsTUVwxyz123456789
```

### Настройка в go-pcap2socks

Откройте `config.json` и добавьте:

```json
{
  "discord": {
    "webhook_url": "https://discord.com/api/webhooks/123456789012345678/ABCdefGHIjklMNOpqrsTUVwxyz123456789"
  }
}
```

### Уведомления Discord

Discord получает красивые embed-сообщения о:
- 🚀 Запуске сервиса
- 📊 Статистике трафика
- 📱 Подключении устройств (ARP monitor)
- ⚠️ Ошибках и предупреждениях
- 📝 Логах (критические события)

### Примеры уведомлений

**Запуск сервиса:**
```
🟢 Запущен
Устройств: 2
Трафик: 1.5 GB
```

**Новое устройство:**
```
✅ Устройство connected
IP адрес: 192.168.137.100
MAC адрес: 78:c8:81:4e:55:15
```

---

## Горячие клавиши

Горячие клавиши работают глобально (даже когда окно не активно).

### Комбинации по умолчанию

| Комбинация | Действие |
|------------|----------|
| `Ctrl+Alt+P` | Вкл/Выкл прокси |
| `Ctrl+Alt+R` | Перезапуск сервиса |
| `Ctrl+Alt+S` | Остановка сервиса |
| `Ctrl+Alt+L` | Показать/скрыть логи |

### Отключение горячих клавиш

В `config.json`:

```json
{
  "hotkey": {
    "enabled": false
  }
}
```

### Изменение комбинаций

В `config.json`:

```json
{
  "hotkey": {
    "enabled": true,
    "toggle": "Ctrl+Shift+P"
  }
}
```

---

## Полный пример config.json

```json
{
  "pcap": {
    "interfaceGateway": "192.168.137.1",
    "network": "192.168.137.0/24",
    "localIP": "192.168.137.1",
    "mtu": 1472
  },
  "dns": {
    "servers": [
      {"address": "8.8.8.8:53"},
      {"address": "1.1.1.1:53"}
    ]
  },
  "routing": {
    "rules": [
      {"dstPort": "53", "outboundTag": "dns-out"}
    ]
  },
  "outbounds": [
    {"socks": {"address": "127.0.0.1:10808"}},
    {"tag": "dns-out", "dns": {}}
  ],
  "telegram": {
    "token": "1234567890:ABCdefGHIjklMNOpqrsTUVwxyz",
    "chat_id": "123456789"
  },
  "discord": {
    "webhook_url": "https://discord.com/api/webhooks/123456789012345678/ABCdefGHIjklMNOpqrsTUVwxyz123456789"
  },
  "hotkey": {
    "enabled": true,
    "toggle": "Ctrl+Alt+P"
  },
  "language": "ru"
}
```

---

## Тестирование

### Проверка Telegram бота

```bash
# Запустите go-pcap2socks
.\go-pcap2socks.exe

# В Telegram отправьте боту /start
# Должно прийти приветственное сообщение

# Проверьте /help для списка команд
```

### Проверка Discord webhook

```bash
# При запуске go-pcap2socks должно прийти уведомление
# в Discord канал
```

### Проверка горячих клавиш

```bash
# Нажмите Ctrl+Alt+P
# Прокси должно включиться/выключиться
```

---

## Решение проблем

### Telegram бот не отвечает

1. Проверьте токен в `config.json`
2. Проверьте Chat ID (должен быть строкой в кавычках)
3. Убедитесь, что вы отправили `/start` боту
4. Проверьте логи go-pcap2socks

### Discord webhook не работает

1. Проверьте URL webhook (должен начинаться с `https://discord.com/api/webhooks/`)
2. Убедитесь, что webhook не удалён
3. Проверьте права доступа к каналу

### Горячие клавиши не работают

1. Проверьте, что `hotkey.enabled: true`
2. Запустите go-pcap2socks от имени администратора
3. Проверьте, не заняты ли комбинации другими программами

---

## Безопасность

⚠️ **Важно!**

- Никогда не публикуйте токен бота или webhook URL
- Не коммитьте `config.json` с реальными токенами в git
- Используйте `.gitignore` для исключения чувствительных данных

### Рекомендуемые практики

1. Создайте отдельный `.env` файл для токенов
2. Используйте переменные окружения:
   ```bash
   TELEGRAM_TOKEN=...
   TELEGRAM_CHAT_ID=...
   DISCORD_WEBHOOK_URL=...
   ```
3. Ограничьте права бота только необходимыми функциями
