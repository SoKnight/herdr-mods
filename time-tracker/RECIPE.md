# Time Tracker

A panel and a status chip for tracking where the working day went, per project, with a report that exports to Markdown or CSV. It has two parts: a Go collector that runs in the background on Windows (`herdr-time.exe`), and a Herdr mod that only displays what the collector answers. Windows only.

## What it runs and writes

Say all of this before installing:

- The collector polls Windows every 5 seconds (`poll_seconds`): time since the last input, the foreground window (exe, title, folder), whether the screen is locked, and which desktop apps hold the microphone (`HKCU\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone\NonPackaged`, read only).
- It runs `hc events` (Herdr's command line, from `%LOCALAPPDATA%\Programs\herdr-core\hc.exe`) to follow agents.
- It serves HTTP on `127.0.0.1:47613` only. The mod and the `herdr-time` command talk to it; nothing listens on another address and nothing is sent anywhere. It also answers CORS preflights from any `moz-extension://` origin (for a future Firefox extension), so any Firefox extension you install could read the collector's data.
- It writes `%LOCALAPPDATA%\herdr-time\`: raw samples, corrections, closed periods, `projects.json`, optional `settings.json`.
- The installer adds a `herdr-time` value under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` so the collector starts with Windows, and starts it.
- The mod runs `herdr-time.exe` with `--json` for every read and change, and opens the system folder picker for the export. A saved report is a file in the folder you pick.

## Install

From `time-tracker/`:

```sh
go -C helper run ./deploy            # builds, copies the mod into Herdr, adds autostart, starts the collector
go -C helper run ./deploy --update   # later: replaces herdr-time.exe while Herdr runs, keeps your data
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

- День: lanes for power, me, app, project, activity and agents. Drag over a range for a menu: project, activity, leave out, add time, note. A saved correction says "Правка #n сохранена." with "Отменить".
- Неделя, Месяц: projects by activity with "Моё" and "Агенты", and days below; a day opens in День.
- Отчёт: project, period, rounding (15 by default), "Показать время агентов (для справки)" (off), preview. "Сохранить CSV" / "Сохранить Markdown" first warns "Период по проекту закроется". Closed periods are listed with "Переоткрыть".

## Watch out

- A closed period can't be exported again until it is reopened (the collector answers 409 `closed`). Its figures stay as exported; a rule change afterwards shows as a difference on the period.
- `herdr-app check` draws only the empty state, so look at the running app too.
- Packaged (MSIX) apps aren't seen as calls: only desktop apps in the microphone consent store.

## Done when

- Node tests, Go tests and vet (the installer's included), and `herdr-app check` pass.
- With the collector stopped, the panel and chip say it isn't running and how to start it.
- After a day of use, День shows lanes that match what you did, and a correction changes the totals at once.
- Exporting a project for a week as Markdown with agents on writes a file with minutes rounded to the step, the agents' line apart and not in the total.
- A rule that moves part of that week to another project shows a difference on the period in Отчёт, and "Переоткрыть" removes it.
