# sing-box-subscribe

> [!IMPORTANT]
> Этот проект сгенерирован нейросетью GPT-5.6-Sol.

[English version](README.md)

Небольшой HTTP-сервис, который загружает plain-text подписку, преобразует proxy URI в outbounds sing-box и отдаёт JSON по HTTP.

Если тег outbound начинается с двухбуквенного кода страны в верхнем регистре, сервис добавляет перед ним соответствующий флаг. Например, `US-SLC` превращается в `🇺🇸 US-SLC`; для `UK` корректно используется флаг с региональным кодом `GB` (`🇬🇧`).

Поддерживаемые ссылки:

- `hysteria2://` и `hy2://`;
- `ss://`, включая SIP003 `v2ray-plugin`;
- `trojan://`;
- `vless://`;
- `vmess://`.

## Запуск

Требуется Go 1.26.5 или новее.

```sh
export SUBSCRIPTION_URL='https://example.com/path/to/plain/config/'
go run .
```

Либо через файл `.env` (шаблон — [`.env.example`](.env.example)):

```sh
cp .env.example .env
# Отредактируйте SUBSCRIPTION_URL в .env.
set -a
. ./.env
set +a
go run .
```

После запуска результат доступен по адресу:

```text
http://localhost:8080/outbounds.json
```

Ответ имеет вид:

```json
{
  "outbounds": [
    {
      "type": "hysteria2",
      "tag": "🇺🇸 US-SLC / Hysteria2",
      "server": "192.0.2.1",
      "server_port": 443,
      "password": "...",
      "tls": {
        "enabled": true,
        "server_name": "example.com"
      }
    }
  ]
}
```

## ENV-параметры

| Переменная | Обязательна | Значение по умолчанию | Описание |
|---|---:|---|---|
| `SUBSCRIPTION_URL` | да | — | URL исходной plain-text подписки |
| `LISTEN_ADDR` | нет | `:8080` | Адрес HTTP-сервера |
| `OUTPUT_PATH` | нет | `/outbounds.json` | Путь выдачи JSON |
| `FETCH_TIMEOUT` | нет | `15s` | Тайм-аут загрузки подписки |
| `CACHE_TTL` | нет | `5m` | Время хранения результата; `0s` отключает свежий кэш |
| `MAX_SUBSCRIPTION_BYTES` | нет | `8388608` | Максимальный размер ответа upstream |
| `GENERATE_URLTEST` | нет | `false` | Генерировать группы `urltest` для каждой страны и каждого протокола |
| `GENERATE_SELECTOR` | нет | `false` | Генерировать группы `selector` для каждой страны и каждого протокола |

Проверка живости доступна на `/healthz`. Если обновление upstream не удалось, но в памяти есть предыдущая версия, сервис отдаёт её с HTTP-заголовком `Warning`.

### Генерируемые группы

`GENERATE_URLTEST=true` добавляет `urltest` для каждой обнаруженной страны и каждого протокола. Каждый URLTest получает следующие параметры:

```json
{
  "interval": "30s",
  "tolerance": 500,
  "idle_timeout": "24h",
  "interrupt_exist_connections": false
}
```

`GENERATE_SELECTOR=true` добавляет `selector` для тех же групп по странам и протоколам. Флаги независимы и могут быть включены одновременно. Outbound без кода страны включается в группу своего протокола, но не включается в страновую группу. `UK` и `GB` объединяются в `🇬🇧 GB`.

## Docker

```sh
docker build -t sing-box-subscribe .
docker run --rm -p 8080:8080 \
  --env-file .env \
  sing-box-subscribe
```

Минимальный запуск без файла `.env`:

```sh
docker run --rm -p 8080:8080 \
  -e 'SUBSCRIPTION_URL=https://example.com/path/to/plain/config/' \
  sing-box-subscribe
```

Запуск с обоими типами генерируемых групп:

```sh
docker run --rm -p 8080:8080 \
  -e 'SUBSCRIPTION_URL=https://example.com/path/to/plain/config/' \
  -e 'GENERATE_URLTEST=true' \
  -e 'GENERATE_SELECTOR=true' \
  sing-box-subscribe
```

## Проверка

```sh
go test ./...
curl http://localhost:8080/outbounds.json
```

Некорректные и неподдерживаемые строки пропускаются и выводятся в лог. Если валидных поддерживаемых ссылок нет, клиент получает HTTP 502.
