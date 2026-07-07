# Протокол Panel ↔ Daemon (wingsd)

## 1. Транспорт (как есть сейчас)

Один канал, HTTP/JSON + WebSocket, который поднимает демон на ноде:

| Канал | Транспорт | Направление | Назначение |
|---|---|---|---|
| Control-plane | HTTPS (JSON) | Panel → Node, унарные вызовы | CRUD серверов, команды питания, файлы, домены, бэкапы |
| Data-plane | WebSocket (тот же порт, `Upgrade`) | двунаправленный, стримы | консоль (stdout/stdin), live-метрики CPU/RAM/Disk |

Оба аутентифицируются одним и тем же `daemon_token` (см.
`nodes.daemon_token_hash`/`daemon_token_encrypted`), который передаётся как
`Authorization: Bearer <token>`. Демон **не принимает входящих соединений от
браузера** — панель является единственным клиентом HTTP/WS API демона.
Браузер получает консоль/метрики через собственный WS-эндпоинт панели
(`/ws/servers/{uuid}` и `/ws/servers/{uuid}/console`), которая внутри себя
проксирует поток от демона. Демон никогда не выставляется в публичный
интернет напрямую (кроме порта 2022 для SFTP, который аутентифицируется
отдельно через `/internal/sftp/authenticate` на панели).

Единственное исключение из направления «панель → демон»: SFTP-сервер
демона сам стучится обратно на панель (`POST
/api/v1/internal/sftp/authenticate`), чтобы проверить ключ/права
подключающегося пользователя — см. `backend/internal/api/handlers/sftp_auth_handler.go`.

## 2. Реальные HTTP/WS-маршруты демона (`daemon/internal/api/router.go`)

| Операция | Метод | Путь на демоне |
|---|---|---|
| Health-check | `GET` | `/healthz` |
| Создать сервер | `POST` | `/api/v1/servers` |
| Питание (start/stop/restart/kill) | `POST` | `/api/v1/servers/{uuid}/power` |
| Отправить команду в консоль | `POST` | `/api/v1/servers/{uuid}/command` |
| Удалить сервер (контейнер + данные на диске) | `DELETE` | `/api/v1/servers/{uuid}` |
| Статистика (cpu/memory/disk/net/state) | `GET` | `/api/v1/servers/{uuid}/stats` |
| Файлы: список | `GET` | `/api/v1/servers/{uuid}/files?path=` |
| Файлы: читать/писать | `GET`/`PUT` | `/api/v1/servers/{uuid}/files/contents?path=` |
| Файлы: удалить | `DELETE` | `/api/v1/servers/{uuid}/files?path=` |
| Файлы: создать папку | `POST` | `/api/v1/servers/{uuid}/files/directory?path=` |
| Файлы: переименовать | `POST` | `/api/v1/servers/{uuid}/files/rename` |
| Домен: добавить (nginx+certbot) | `POST` | `/api/v1/servers/{uuid}/domains` |
| Домен: удалить | `DELETE` | `/api/v1/servers/{uuid}/domains/{domain}` |
| Бэкап: создать | `POST` | `/api/v1/servers/{uuid}/backups` |
| Бэкап: восстановить | `POST` | `/api/v1/servers/{uuid}/backups/{backup_uuid}/restore` |
| Бэкап: удалить | `DELETE` | `/api/v1/servers/{uuid}/backups/{backup_uuid}` |
| Бэкап: скачать | `GET` | `/api/v1/servers/{uuid}/backups/{backup_uuid}/download` |
| Консоль/метрики (WS) | `GET` (Upgrade) | `/ws/servers/{uuid}` |

Аутентификация — тот же bearer-токен ноды в заголовке `Authorization`,
кроме `/healthz` (без авторизации, для мониторинга).

Формат `stats`-ответа (`resourceStatsResponse` в
`daemon/internal/api/handlers.go`):

```json
{
  "server_uuid": "...",
  "cpu_percent": 12.5,
  "memory_bytes": 134217728,
  "disk_bytes": 0,
  "network_rx": 1024,
  "network_tx": 2048,
  "uptime_seconds": 0,
  "state": "running"
}
```

`state` — одно из `offline` / `starting` / `running` (демон определяет его
через `docker inspect`, см. `daemon/internal/docker/manager.go:
InspectState`).

## 3. gRPC — не реализовано, roadmap

В `daemon/proto/daemon.proto` есть черновой набросок gRPC-контракта
(унарные вызовы + server-streaming для метрик/консоли/файлов), написанный
как более "правильная" долгосрочная альтернатива HTTP/JSON — типизация,
двусторонний стриминг без ручного WS-протокола, встроенный keepalive и
mTLS вместо голого bearer-токена. Но `google.golang.org/grpc` не
подключён ни в одном модуле, `.pb.go` не сгенерированы, и ничего в
`backend/internal/daemonclient` или `daemon/internal/api` его не
использует — весь реальный обмен идёт через HTTP/JSON+WS, описанный выше
в разделе 2. Если миграция на gRPC когда-нибудь случится, `.proto` файл
уже задаёт контракт по составу операций; сам протокол панели для конечных
пользователей (REST-эндпоинты панели) при этом не поменяется — поменяется
только транспорт между панелью и демоном.
