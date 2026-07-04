# Протокол Panel ↔ Daemon (wingsd)

## 1. Транспорт

Два независимых канала, оба поднимает **демон**, слушая на ноде:

| Канал | Транспорт | Направление | Назначение |
|---|---|---|---|
| Control-plane | gRPC, mTLS | Panel → Node (унарные вызовы) и Node → Panel (server-streaming) | CRUD серверов, команды питания, список файлов, бэкапы |
| Data-plane | WebSocket (внутри того же TLS) | двунаправленный, стримы | консоль (stdout/stdin), live-метрики CPU/RAM/Disk, прогресс установки |

Оба транспорта аутентифицируются одним и тем же `daemon_token` (см.
`nodes.daemon_token_hash`), который передаётся как `Authorization: Bearer
<token>` в gRPC metadata / WS-хэндшейке. Дополнительно рекомендуется mTLS
(клиентский сертификат демона, подписанный внутренним CA панели) для защиты
от подмены ноды.

Демон **не принимает входящих соединений от браузера** — панель является
единственным клиентом gRPC/WS API демона. Браузер получает консоль/метрики
через собственный WS-эндпоинт панели (`/ws/servers/{uuid}/console`), которая
внутри себя проксирует поток от демона. Так демон никогда не выставляется в
публичный интернет напрямую.

## 2. gRPC-контракт (`daemon/proto/daemon.proto`)

Файл — источник истины; ниже перечислены основные RPC и их назначение.

### Управление жизненным циклом сервера
- `CreateServer(CreateServerRequest) → ServerOperationResponse` — создаёт
  контейнер (образ, env, лимиты cgroups), запускает install-script во
  временном контейнере, стримит прогресс через `StreamInstallLog`.
- `StartServer` / `StopServer` / `RestartServer` / `KillServer`
  (`ServerActionRequest{server_uuid}`) → `ServerOperationResponse`.
- `DeleteServer(ServerActionRequest) → Empty` — останавливает и удаляет
  контейнер + данные на диске.
- `UpdateServerLimits(UpdateLimitsRequest) → Empty` — применяет новые
  cgroups-лимиты (memory/cpu/disk quota) без пересоздания контейнера.

### Реалтайм
- `StreamResourceStats(ServerActionRequest) → stream ResourceStats` —
  раз в секунду отдаёт `{cpu_percent, memory_bytes, disk_bytes, network_rx,
  network_tx, uptime_seconds, state}`.
- `StreamConsole(ServerActionRequest) → stream ConsoleLine` — построчный
  вывод stdout/stderr контейнера.
- `SendConsoleCommand(ConsoleCommandRequest{server_uuid, line}) → Empty` —
  запись в stdin процесса (аналог `docker attach`).
- `StreamInstallLog(ServerActionRequest) → stream ConsoleLine` — вывод
  установочного скрипта.

### Файловый менеджер (используется редактором в браузере)
- `ListDirectory(FileRequest{server_uuid, path}) → FileListResponse`
- `ReadFile(FileRequest) → stream FileChunk` (чанки по 64 КБ, для больших файлов)
- `WriteFile(stream FileChunk) → Empty` (client-streaming upload)
- `DeleteFile`, `CreateDirectory`, `RenameFile`, `CompressFiles`, `DecompressFile`

### Бэкапы
- `CreateBackup(BackupRequest) → BackupOperationResponse` — архивирует данные
  сервера (tar.zst), стримит статус завершения обратно панели через
  webhook/gRPC callback `ReportBackupComplete`.
- `RestoreBackup`, `DeleteBackup`

### Служебное
- `Register(RegisterRequest{node_uuid, token, agent_version}) →
  RegisterResponse` — первичное рукопожатие при старте демона; панель
  отвечает конфигом (TLS-параметры, лимиты по умолчанию).
- `Heartbeat(HeartbeatRequest{node_uuid, load}) → Empty` — раз в 10с,
  обновляет `nodes.last_seen_at`; отсутствие heartbeat > 30с помечает ноду
  offline в UI.

## 3. Пример .proto (сокращённо)

```protobuf
syntax = "proto3";
package daemon.v1;
option go_package = "github.com/yourorg/panel-proto/daemon/v1;daemonv1";

service NodeDaemon {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (Empty);

  rpc CreateServer(CreateServerRequest) returns (ServerOperationResponse);
  rpc StartServer(ServerActionRequest) returns (ServerOperationResponse);
  rpc StopServer(ServerActionRequest) returns (ServerOperationResponse);
  rpc RestartServer(ServerActionRequest) returns (ServerOperationResponse);
  rpc KillServer(ServerActionRequest) returns (ServerOperationResponse);
  rpc DeleteServer(ServerActionRequest) returns (Empty);
  rpc UpdateServerLimits(UpdateLimitsRequest) returns (Empty);

  rpc StreamResourceStats(ServerActionRequest) returns (stream ResourceStats);
  rpc StreamConsole(ServerActionRequest) returns (stream ConsoleLine);
  rpc SendConsoleCommand(ConsoleCommandRequest) returns (Empty);
  rpc StreamInstallLog(ServerActionRequest) returns (stream ConsoleLine);

  rpc ListDirectory(FileRequest) returns (FileListResponse);
  rpc ReadFile(FileRequest) returns (stream FileChunk);
  rpc WriteFile(stream FileChunk) returns (Empty);
  rpc DeleteFile(FileRequest) returns (Empty);
  rpc CreateDirectory(FileRequest) returns (Empty);
  rpc RenameFile(RenameRequest) returns (Empty);

  rpc CreateBackup(BackupRequest) returns (BackupOperationResponse);
  rpc RestoreBackup(BackupRequest) returns (BackupOperationResponse);
  rpc DeleteBackup(BackupRequest) returns (Empty);
}

message ServerActionRequest { string server_uuid = 1; }

message ResourceStats {
  string server_uuid   = 1;
  double cpu_percent   = 2;
  int64  memory_bytes  = 3;
  int64  disk_bytes    = 4;
  int64  network_rx    = 5;
  int64  network_tx    = 6;
  int64  uptime_seconds = 7;
  string state         = 8; // offline|starting|running|stopping
}

message ConsoleLine {
  string server_uuid = 1;
  string stream      = 2; // stdout|stderr|daemon
  string line        = 3;
  int64  timestamp   = 4;
}
```

Полный `.proto` со всеми сообщениями лежит в
[`daemon/proto/daemon.proto`](../daemon/proto/daemon.proto). Стабы
генерируются стандартно:

```bash
protoc --go_out=. --go-grpc_out=. daemon/proto/daemon.proto
```

> В стартовом коде (`backend/internal/daemonclient`, `daemon/internal/api`)
> control-plane временно реализован через обычный HTTPS + WebSocket JSON API
> (см. `docs/PROTOCOL.md#4`) — это тот же контракт по составу операций, но
> без шага кодогенерации, чтобы репозиторий собирался `go build` без
> установленного `protoc`. Миграция на gRPC — замена клиента/сервера в этих
> двух пакетах на сгенерированные типы, домены и REST-хендлеры панели не
> меняются.

## 4. HTTP/WS-эквивалент (используется в стартовом коде)

| Операция | Метод | Путь на демоне |
|---|---|---|
| Регистрация/heartbeat | `POST` | `/api/v1/heartbeat` |
| Создать сервер | `POST` | `/api/v1/servers` |
| Питание (start/stop/restart/kill) | `POST` | `/api/v1/servers/{uuid}/power` |
| Удалить | `DELETE` | `/api/v1/servers/{uuid}` |
| Консоль + метрики | `GET` (Upgrade) | `/ws/servers/{uuid}` |
| Файлы: список | `GET` | `/api/v1/servers/{uuid}/files?path=` |
| Файлы: читать/писать | `GET`/`PUT` | `/api/v1/servers/{uuid}/files/contents?path=` |

Аутентификация — тот же bearer-токен ноды в заголовке `Authorization`.
