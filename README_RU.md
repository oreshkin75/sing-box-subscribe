# sing-box-subscribe

> [!IMPORTANT]
> Этот проект сгенерирован нейросетью GPT-5.6-Sol.

[English version](README.md)

Небольшой HTTP-сервис, который получает URL plain-text подписки в запросе, загружает её, преобразует proxy URI в outbounds sing-box и возвращает готовый JSON.

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
go run .
```

Все параметры запуска опциональны. Для их настройки через файл `.env` используйте шаблон [`.env.example`](.env.example):

```sh
cp .env.example .env
set -a
. ./.env
set +a
go run .
```

После запуска передайте ссылку на подписку в обязательном query-параметре `url`:

```sh
curl --get \
  --data-urlencode 'url=https://example.com/path/to/plain/config/' \
  http://localhost:8080/outbounds.json
```

Рекомендуется использовать `--data-urlencode`, поскольку ссылки на подписки часто содержат токены и собственные query-параметры. Отсутствующий, повторяющийся, некорректный или не HTTP(S) параметр `url` возвращает HTTP 400. Ошибка загрузки или преобразования подписки возвращает HTTP 502.

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
| `LISTEN_ADDR` | нет | `:8080` | Адрес HTTP-сервера |
| `OUTPUT_PATH` | нет | `/outbounds.json` | Путь выдачи JSON |
| `FETCH_TIMEOUT` | нет | `15s` | Тайм-аут загрузки подписки |
| `CACHE_TTL` | нет | `5m` | Время хранения результата отдельно для каждой подписки; `0s` отключает свежий кэш |
| `MAX_SUBSCRIPTION_BYTES` | нет | `8388608` | Максимальный размер ответа upstream |
| `GENERATE_URLTEST` | нет | `false` | Генерировать группы `urltest` для каждой страны и каждого протокола |
| `URLTEST_URL` | нет | пусто | URL для проверки соединения; пустое значение использует стандартный URL sing-box |
| `URLTEST_INTERVAL` | нет | `30s` | Интервал проверки URLTest |
| `URLTEST_TOLERANCE` | нет | `500` | Допуск URLTest в миллисекундах |
| `URLTEST_IDLE_TIMEOUT` | нет | `24h` | Тайм-аут неактивности URLTest |
| `URLTEST_INTERRUPT_EXIST_CONNECTIONS` | нет | `false` | Прерывать существующие соединения при смене выбранного outbound |
| `GENERATE_SELECTOR` | нет | `false` | Генерировать группы `selector` для каждой страны и каждого протокола |
| `SELECTOR_INTERRUPT_EXIST_CONNECTIONS` | нет | `false` | Прерывать существующие соединения при смене выбранного selector outbound |

Проверка живости доступна на `/healthz`. Результаты кэшируются отдельно для каждой ссылки; в памяти хранится не более 128 подписок. Если обновление upstream не удалось, но для этой ссылки есть предыдущая версия, сервис отдаёт её с HTTP-заголовком `Warning`.

> [!WARNING]
> Сервис загружает URL, переданные клиентами. Не публикуйте его в недоверенной сети без аутентификации или ограничения доступа на сетевом уровне.

### Генерируемые группы

`GENERATE_URLTEST=true` добавляет `urltest` для каждой обнаруженной страны и каждого протокола. Его параметры настраиваются переменными `URLTEST_*`. Со значениями по умолчанию каждый URLTest получает:

```json
{
  "url": "",
  "interval": "30s",
  "tolerance": 500,
  "idle_timeout": "24h",
  "interrupt_exist_connections": false
}
```

Переменные `URLTEST_*` читаются и проверяются только при `GENERATE_URLTEST=true`. Когда генерация отключена, они ни на что не влияют. Заданный `URLTEST_URL` должен быть абсолютным HTTP(S)-адресом, длительности должны быть положительными, а tolerance — неотрицательным.

`GENERATE_SELECTOR=true` добавляет `selector` для тех же групп по странам и протоколам. `SELECTOR_INTERRUPT_EXIST_CONNECTIONS` управляет соответствующим полем во всех сгенерированных selector’ах и читается только при включённой генерации selector. Флаги генерации независимы и могут быть включены одновременно. Outbound без кода страны включается в группу своего протокола, но не включается в страновую группу. `UK` и `GB` объединяются в `🇬🇧 GB`.

## Docker

```sh
docker build -t sing-box-subscribe .
docker run --rm -p 8080:8080 \
  --env-file .env \
  sing-box-subscribe
```

Минимальный запуск без файла `.env`:

```sh
docker run --rm -p 8080:8080 sing-box-subscribe
```

Запуск с обоими типами генерируемых групп:

```sh
docker run --rm -p 8080:8080 \
  -e 'GENERATE_URLTEST=true' \
  -e 'URLTEST_URL=https://www.gstatic.com/generate_204' \
  -e 'URLTEST_INTERVAL=30s' \
  -e 'URLTEST_TOLERANCE=500' \
  -e 'URLTEST_IDLE_TIMEOUT=24h' \
  -e 'URLTEST_INTERRUPT_EXIST_CONNECTIONS=false' \
  -e 'GENERATE_SELECTOR=true' \
  -e 'SELECTOR_INTERRUPT_EXIST_CONNECTIONS=false' \
  sing-box-subscribe
```

## Проверка

```sh
go test ./...
curl --fail --show-error --get \
  --data-urlencode 'url=https://example.com/path/to/plain/config/' \
  -o outbounds.json \
  http://localhost:8080/outbounds.json
```

Некорректные и неподдерживаемые строки пропускаются и выводятся в лог. Если валидных поддерживаемых ссылок нет, клиент получает HTTP 502.
