# Time Tracker

A panel and a status chip for tracking where the working day went, per project, with a month report that copies out as text or saves as CSV. It has two parts: a Go collector that runs in the background on Windows (`herdr-time.exe`), and a Herdr mod that only displays what the collector answers. Windows only.

## What it runs and writes

Say all of this before installing:

- The collector polls Windows every 5 seconds (`poll_seconds`): time since the last input, the foreground window (exe and title; the folder comes only from the signals Herdr and the IDE send), whether the screen is locked, and which desktop apps hold the microphone (`HKCU\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone\NonPackaged`, read only).
- It reads Minecraft's `latest.log` (`minecraft_logs` in `settings.json`, read only) to learn the server, and runs `hc events` (Herdr's command line: `hc` on `PATH` first, then `%LOCALAPPDATA%\Programs\herdr-core\hc.exe`) to follow agents.
- It serves HTTP on `127.0.0.1:47613` only. The mod and the `herdr-time` command talk to it; nothing listens on another address and nothing is sent anywhere. It also answers CORS preflights from any `moz-extension://` origin (for a future Firefox extension), so any Firefox extension you install could read the collector's data.
- It writes `%LOCALAPPDATA%\herdr-time\`: raw samples, corrections, closed periods and month records (`periods.json`), bonuses (`bonuses.json`), sessions (`sessions.json`), `history.json` (rule, goal, pay and month changes), `calendar.json` (production calendar, copied once, yours to edit), `daysoff.json`, `projects.json` (projects, goals, pay and rules; a file from before rules is converted once and kept as `projects.v1.json`), optional `settings.json`, the spans still open (`open.json`; one that can't be read is moved to `open.json.bad`) and its log, `collector.log` (started afresh once over 1 MiB). With a broken `projects.json`, `settings.json` or `calendar.json` at start it keeps collecting on the defaults, and the panel shows the error until the file is fixed.
- It asks Windows which apps have an audible session on the default playback device (read only), and runs `git log` (hidden, 5 s per repo and 10 s per report or export, branches, tags and remotes) in the repos at or one level under the folder rules to list my commits: once a minute in the background for the last 48 hours, and on demand for a month; it never writes to a repo. A repo without `user.email` lists every author's commits. Agent waiting comes from the terminal states in `hc events`: the snapshot it starts with, then each terminal's status change (the wait starts when the change arrives); a new or restarted terminal makes it start `hc events` again for a fresh snapshot. Nothing is installed into `~/.claude`.
- When the collector is silent for 10 minutes, or last month has my time and isn't closed, the mod shows a toast (once a day per Herdr start for the month).
- "Перезапустить" in the panel and `herdr-time restart` start the collector again, detached from the caller.
- The installer adds a `herdr-time` value under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` so the collector starts with Windows, and starts it. With `"autostart": false` in `settings.json` it leaves the value out. The Настройки toggle adds or removes the same value.
- The mod runs `herdr-time.exe` with `--json` for every read and change, and saves a CSV into `Downloads` (`{project}_{YYYY-MM}.csv`) from Отчёт and from «Отчёт текстом», which also copies to the clipboard.

## Install

From `time-tracker/`:

```sh
go -C helper run ./deploy   # builds, copies the mod into Herdr, adds autostart, starts the collector
                            # again later: stops the collector, replaces herdr-time.exe, starts it; keeps your data
```

The installer is in `deploy/`, not `install/`: Windows asks for an administrator before running a program named install, setup or update. It doesn't edit layouts. Then `herdr-app check ~/.config/herdr-app/mods/time-tracker`.

## In a layout

`mod.json` has `place: none`, so nothing docks by itself. In a layout you own, add the chip to a status bar and the panel to a stack:

```json
{ "id": "time-status", "type": "block", "content": { "mod": "time-tracker", "props": { "view": "status" } } }
```

```json
{ "id": "time-panel", "type": "block", "content": { "mod": "time-tracker" } }
```

Keep the chip shown all the time: it is what passes Herdr's signals to the collector, and the panel alone doesn't. The mod also opens as a popup with `herdr-app do mod.open --name time-tracker`.

## Using it

- Сессия: the current session, its stretches, switches and longest focus; "Завершить" / "Начать сессию" end or start it (`herdr-time session [start|end]`).
- День (06:00 to 06:00): one strip with an agents strip under it, then the sessions with their stretches. Drag over a range up to now for its menu: project (or "Без проекта"), kind of work, "Исключить — не работа", note, a rule from the window, "+ Новый проект…". A saved correction says "Правка #n сохранена." with "Отменить" for that last one. "+ вручную" adds time by hand to the open day (from–to, up to now; replaces recorded time it overlaps); the history icon opens История правок.
- Разбор ("разобрать «Другое» →"): the day's windows without a rule with a suggested project; Принять, Исключить or another project, "применить к прошлым дням", "Готово" (CLI: `inbox`, `inbox apply`).
- История правок: corrections, rule, goal and pay changes, days off and month closes by day or month, each with "Отменить"; a rule, goal or pay changed again since can't be undone from the older entry (CLI: `history`, `history undo`).
- Chip: today's time and the project in focus, отошёл, a play icon when no session runs, нет данных; a card on hover, the panel on click. With no data, "Перезапустить" (or `herdr-time restart`) starts the collector detached.
- Проекты: projects with colour and archive, one ordered list of rules (folder, app, title, Figma, Minecraft server; first match wins), the rule editor with matches and "Поднять выше", Не работа and Досуг, "+ Новый проект…" (CLI: `rules`, `rule add|set|rm|preview`, `project add|set`, `hints`).
- Цели: per project, не меньше / не больше N ч в день, неделю или месяц; day goals by weekdays, the production calendar and "выходные в зачёт"; "Действует с", "Сохранить с …" / "Убрать с …"; from the pencil in Проекты or "+ цель"; saving from a date replaces the versions after it (undo in История правок); the dialog sends only the side you changed (CLI: `goal set`, `goals`).
- Оплата: in the same dialog, фикс в месяц or почасовая, in whole rubles, from the month Отчёт shows and kept from then on; the rate is sum × 60 / the month's plan, hourly is my minutes × rate / 60 (CLI: `pay set`).
- Настройки: `session_idle_minutes`, `pause_minutes`, `day_start`, `video_activity`, `autostart`, `silent_warning`, `close_reminder` and `hidden_titles` (patterns, case-insensitive, `*` wildcard, matched against the exe or the title; time counts, the title isn't stored, new data only, and title rules can't match it, so the time may fall to Другое). Changing `video_activity`, `day_start`, `session_idle_minutes` or `pause_minutes` recounts the open month's past days and pay; closed months stay frozen. Other keys in `settings.json` are kept; a bad value answers 400 and changes nothing (CLI: `settings`, `settings set '<json>'`).
- Agent waiting: `ждёт Nм` on the chip, light segments on the agents strip, "за сегодня агенты ждали тебя Nм" on the chip's card and `ждали Nм` on the Сессия and День summary lines and a project's goal line come from the `blocked` terminal state (`done` is not waiting); one wait counts at most `session_idle_minutes`, only while the PC is on and unlocked, and isn't in a month record.
- Video with sound: with `video_activity`, the foreground app with an audible session counts as activity; background audio doesn't.
- Commits: on stretches (the last 48 hours, refreshed every minute), as export bullets (`export --commits`, the «коммиты» option) and `N за 7 дней` in Проекты; read live, also for closed months; a repo without `user.email` gives every author's (CLI: `commits --days N`).
- Неделя: seven day strips with agents, mine and the day goal's difference, totals, the week's goals and Досуг; a day opens in День.
- Месяц: calendar with an 8-hour stripe, goal tick, forecast and "праздник"; drag days for "Отметить дни" (отгул / отпуск / больничный, whose plan drops), undone in История правок (CLI: `dayoff add|rm`, `plan`). Minutes are rounded per project and day.
- Отчёт: project and month as a table by week or day (Раб. дн., Цель, Моё, Разница, Агенты) with totals, a footnote, the month's kinds of work and Оплата; "+ добавить" a bonus (title, sum, day; `×` removes it); "Скопировать текстом" (options: notes, pay, agents, goal) and "CSV" into Downloads for Excel (CLI: `month report`, `export`, `bonus add|rm`).
- Закрыть месяц: once a month is over, one close for every project after a checklist; it freezes time, goals, days off and pay, and refuses corrections, Разбор, days off and bonuses there; "открыть заново" from Отчёт or История правок, both undoable (CLI: `month state|close|reopen`; `periods`, `reopen <id>` for v1's).

## Watch out

- A closed month keeps its time, goals and pay as they were when it was closed; a rule, goal or pay change afterwards shows only on День as a difference, until it is reopened. A write it refuses answers 409 `closed`. v1's per-project periods still apply, read-only; nothing makes new ones.
- While a call is active (Telemost holds the microphone) the time counts as the call's project and activity even if you type in another app: fix it with a correction.
- `herdr-time` output in PowerShell or cmd can come after the prompt (a windowless build): add `| Out-String` or use Git Bash.
- `herdr-app check` draws only the empty state, so look at the running app too.
- Packaged (MSIX) apps aren't seen as calls: only desktop apps in the microphone consent store.
- Agents outside Herdr's terminals aren't seen, there is no sync with a Mac or another device, and sound is read from the default playback device only.

## Done when

- Node tests, Go tests and vet (the installer's included), and `herdr-app check` pass.
- With the collector stopped, the panel and chip say it isn't running and how to start it.
- After a day of use, День shows a strip that matches what you did, and a correction changes the totals at once.
- Отчёт for a month shows the table, the pay and the text and CSV with the same total; the CSV opens in Excel with Cyrillic and the pay rows.
- Настройки saves and applies at once, keeps unedited keys and refuses bad values; a terminal left `blocked` for days shows no waiting, a `done` one none at all; a video with sound in the foreground isn't idle; a hidden title isn't stored.
- A rule changed after a month is closed doesn't move its numbers; День shows the difference, and "открыть заново" brings the new numbers.
