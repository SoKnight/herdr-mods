# Time Tracker

A panel and a status chip for tracking where the working day went, per project, with a report that exports to Markdown or CSV. It has two parts: a Go collector that runs in the background on Windows (`herdr-time.exe`), and a Herdr mod that only displays what the collector answers. Windows only.

## What it runs and writes

Say all of this before installing:

- The collector polls Windows every 5 seconds (`poll_seconds`): time since the last input, the foreground window (exe and title; the folder comes only from the signals Herdr and the IDE send), whether the screen is locked, and which desktop apps hold the microphone (`HKCU\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone\NonPackaged`, read only).
- It reads Minecraft's `latest.log` (`minecraft_logs` in `settings.json`, read only) to learn the server, and runs `hc events` (Herdr's command line: `hc` on `PATH` first, then `%LOCALAPPDATA%\Programs\herdr-core\hc.exe`) to follow agents.
- It serves HTTP on `127.0.0.1:47613` only. The mod and the `herdr-time` command talk to it; nothing listens on another address and nothing is sent anywhere. It also answers CORS preflights from any `moz-extension://` origin (for a future Firefox extension), so any Firefox extension you install could read the collector's data.
- It writes `%LOCALAPPDATA%\herdr-time\`: raw samples, corrections, closed periods, sessions (`sessions.json`), `history.json` (rule changes), `projects.json` (projects and rules; a file from before rules is converted once and kept as `projects.v1.json`), optional `settings.json`, the spans still open (`open.json`; one that can't be read is moved to `open.json.bad`) and its log, `collector.log` (started afresh once over 1 MiB). With a broken `projects.json` or `settings.json` at start it keeps collecting on the defaults, and the panel shows the error until the file is fixed.
- "Перезапустить" in the panel and `herdr-time restart` start the collector again, detached from the caller.
- The installer adds a `herdr-time` value under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` so the collector starts with Windows, and starts it.
- The mod runs `herdr-time.exe` with `--json` for every read and change, and opens the system folder picker for the export. A saved report is a file in the folder you pick.

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
- История правок: corrections and rule changes by day or month, each with "Отменить"; a rule changed again since can't be undone from the older entry (CLI: `history`, `history undo`).
- Chip: today's time and the project in focus, отошёл, a play icon when no session runs, нет данных; a card on hover, the panel on click. With no data, "Перезапустить" (or `herdr-time restart`) starts the collector detached.
- Проекты: projects with colour and archive, one ordered list of rules (folder, app, title, Figma, Minecraft server; first match wins), the rule editor with matches and "Поднять выше", Не работа and Досуг, "+ Новый проект…" (CLI: `rules`, `rule add|set|rm|preview`, `project add|set`, `hints`).
- Неделя, Месяц: projects by activity with "Моё" and "Агенты", and days below; a day opens in День.
- Отчёт: project, period, rounding (15 by default), "Показать время агентов (для справки)" (off), preview. "Сохранить CSV" / "Сохранить Markdown" first warns "Период по проекту закроется". Closed periods are listed with "Переоткрыть".

## Watch out

- A closed period can't be exported again until it is reopened (the collector answers 409 `closed`), nor can its minutes go into another project's export. Its figures stay as exported; a rule change afterwards shows as a difference on the period, and the day totals may differ from the sum of the exports until it is reopened.
- While a call is active (Telemost holds the microphone) the time counts as the call's project and activity even if you type in another app: fix it with a correction.
- `herdr-time` output in PowerShell or cmd can come after the prompt (a windowless build): add `| Out-String` or use Git Bash.
- `herdr-app check` draws only the empty state, so look at the running app too.
- Packaged (MSIX) apps aren't seen as calls: only desktop apps in the microphone consent store.

## Done when

- Node tests, Go tests and vet (the installer's included), and `herdr-app check` pass.
- With the collector stopped, the panel and chip say it isn't running and how to start it.
- After a day of use, День shows a strip that matches what you did, and a correction changes the totals at once.
- Exporting a project for a week as Markdown with agents on writes a file with minutes rounded to the step, the agents' line apart and not in the total.
- A rule that moves part of that week to another project shows a difference on the period in Отчёт, and "Переоткрыть" removes it.
