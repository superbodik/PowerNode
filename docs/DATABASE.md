# Схема базы данных

Полный DDL: [`backend/migrations/0001_init.sql`](../backend/migrations/0001_init.sql)
(PostgreSQL 15+). Ниже — пояснение сущностей и связей; Redis в схему не входит
(используется только для сессий, pub/sub live-метрик и rate limiting, без
персистентной схемы).

## Диаграмма связей (упрощённо)

```
users ──< user_roles >── roles ──< role_permissions >── permissions
  │                                                          
  ├──< ssh_keys                                              
  ├──< api_keys                                              
  └──< servers (owner_id)                                    
                │
                ├──< server_subusers >── users (общий доступ к серверу)
                ├──< server_variables >── egg_variables ── eggs
                ├──< server_databases
                ├──< server_backups
                ├──< server_schedules ──< schedule_tasks
                └──< allocations >── nodes ── locations
```

## Пользователи и RBAC

- **users** — учётные записи. `is_admin` — суперпользовательский флаг,
  обходящий RBAC полностью (не роль, а escape hatch для владельца панели).
- **roles / permissions / role_permissions / user_roles** — классический
  RBAC: у роли есть набор permission-кодов вида `server.control.start`,
  `node.create`, `user.manage`; пользователю назначается ≥1 роль.
- **server_subusers** — отдельный, более гранулярный уровень: сервер можно
  "расшарить" другому пользователю с конкретным набором прав именно на этот
  сервер (JSONB-массив кодов), не выдавая ему глобальную роль. Это то, что в
  Pterodactyl называется subuser.
- **ssh_keys** — публичные ключи для доступа к SFTP, встроенному в `wingsd`.
- **api_keys** — токены для программного доступа к REST API (хранится только
  sha256-хэш, сырой токен показывается один раз при создании).

## Ноды и размещение

- **locations** — регионы/дата-центры, чисто группировка для UI и авто-выбора
  ноды при деплое.
- **nodes** — одна запись на инстанс `wingsd`. `daemon_token_hash` — bcrypt от
  токена, которым демон аутентифицируется (сырой токен возвращается один раз
  при `POST /api/v1/nodes`, см. `NodeHandler.Create`). `memory_overallocate`/
  `disk_overallocate` позволяют осознанно продавать больше ресурсов, чем
  физически есть (в процентах), как это делает Pterodactyl.
- **allocations** — пул `ip:port` на ноде. Сервер занимает одну или несколько
  (через `servers.allocation_limit`), `server_id = NULL` значит "свободна и
  доступна для выдачи новому серверу".

## Модульная система (eggs = шаблоны)

- **eggs** — шаблон для деплоя: Docker-образ, команда запуска, install-script,
  regex для детекта "сервер поднялся". `category` разделяет игровые сервера,
  ботов, сайты и generic-контейнеры — это и есть "модульная система для
  быстрого деплоя" из требований.
- **egg_variables** — переменные окружения шаблона, редактируемые
  пользователем при создании сервера (например, версия Minecraft, порт бота).
  Валидация — простой rules-DSL в духе Laravel (`required|string|max:20`).

## Серверы

- **servers** — ядро схемы. Разделены два вида лимитов:
  - *cgroups-лимиты* (`memory_mb`, `swap_mb`, `disk_mb`, `io_weight`,
    `cpu_percent`, `threads_pinned`) — применяются демоном на уровне Docker/
    cgroups, физически ограничивают контейнер;
  - *feature-лимиты* (`allocation_limit`, `database_limit`, `backup_limit`) —
    квоты, которые проверяет только панель при создании ресурсов, cgroups их
    не касаются.
  `status` — enum состояния жизненного цикла; `container_id` заполняется
  после успешного `CreateServer` на демоне.
- **server_variables** — резолвленные значения `egg_variables` для конкретного
  сервера (то, что реально пойдёт в `environment` контейнера).
- **server_databases** — учётки БД, выданные серверу (например, MySQL для
  сайта); пароль хранится зашифрованным (AES-GCM, ключ — `PANEL_ENCRYPTION_KEY`),
  а не хэшированным, потому что панели нужно суметь показать/использовать его
  повторно, а не только сверить.
- **server_backups** — метаданные архивов (сам архив лежит на диске ноды или
  в объектном хранилище — путь не хранится в БД специально, чтобы не течь
  инфраструктурные детали в API-ответы; резолвится по `server_uuid`+`uuid`).
- **server_schedules / schedule_tasks** — cron-подобные расписания
  (power-действия, команды, бэкапы), задачи внутри расписания выполняются по
  `sequence_id` с задержкой `time_offset_seconds` — как цепочка шагов.

## Аудит

- **activity_logs** — единая лента событий (кто, что, когда, с какого IP).
  `metadata` — JSONB для произвольного контекста события (например, для
  `server:power.start` — какая нода обработала запрос). Индексируется по
  `created_at DESC` и отдельно по `(server_id, created_at DESC)` для вкладки
  Activity на странице сервера.
