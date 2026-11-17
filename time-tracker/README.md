# Time Tracker

Where your working day went, per project, for [Herdr Desktop](https://herdr.dev/) on Windows. A small collector watches your keyboard and mouse activity, the foreground window, the lock screen, calls and your Herdr agents, and a panel shows the day as lanes you can correct by selecting time. A week, a month and a report with a Markdown or CSV file for a client follow from the same numbers.

<!-- preview.png: added once the panel is checked on a live day -->

- **Session** (Сессия): the current working session. It starts after 45 minutes without activity and ends after 45 minutes without it (`session_idle_minutes`), or by "Завершить" / "Начать сессию". Stretches split at breaks over 15 minutes (`pause_minutes`); it shows the switches between projects and the longest focus.
- **Day** (День): 06:00 to 06:00, as sessions with their stretches and the breaks between them, plus lanes for power, me, app, project, activity and agents. Drag over a range for "Правка отрезка": set its project or activity, leave it out, add time or a note; every correction can be undone. "No project" opens into what a rule could match, and picking a project writes the rule.
- **Week and Month** (Неделя, Месяц): projects by kind of activity, with my time and the agents' beside them and every day below; click a day to open it.
- **Report** (Отчёт): a project, a period (week, month or your own dates; ‹ › step to the previous week or month), rounding to 1, 5 or 15 minutes (15 by default), a switch "Показать время агентов (для справки)" (off), and a Markdown preview. "Сохранить CSV" / "Сохранить Markdown" writes the file to a folder you pick (`~/Downloads` by default) and **closes the period** for that project.
- **Closed periods:** listed under the report with how far today's rules would move them. "Переоткрыть" lifts the lock.
- **Chip:** a status-bar block. It shows the session's time and project, "отошёл" after a pause, "сессии нет" when none runs, and "нет данных" when the collector is silent (День then offers "Перезапустить"). A card opens on click. It also passes Herdr's signal (the folder you work in) to the collector; the agents the collector follows itself through `hc events`.

## What it counts

- **My time** is time with input in the last 5 minutes (`idle_minutes`), or a call, and not while the screen is locked. Which project it belongs to comes from the foreground window: its folder, title, URL, Figma file or app, matched by the rules in `projects.json`. Time after the last project carries on for up to 15 minutes (`carry_minutes`).
- **Agents' time** is the time Herdr agents work, from `hc events`, per project. It is only ever a side figure: it is never added to my time, and in a file it is its own line, outside the total.
- **A day** starts at 06:00 (`day_start`), so a night session stays with the day it began.
- **A call wins:** while Telemost holds the microphone, the time counts as the call's project and activity even if you type in another app. Fix such a stretch with a correction.

### How Telemost is detected

A call counts while the Telemost executable holds the microphone: the collector reads Windows' `CapabilityAccessManager` consent store for desktop (non-packaged) apps and treats an app as in a call while its `LastUsedTimeStop` is 0. So a call with your microphone muted in Telemost still counts. Packaged (MSIX) apps aren't in that store and aren't seen.

Проверено на: (заполняется после живой проверки)

## Install

Needs Windows, Go 1.23+ and Herdr's `hc`. Read [RECIPE.md](RECIPE.md) and the source first: the collector runs in the background and starts with Windows.

From this folder:

```sh
go -C helper run ./deploy
```

It builds `herdr-time.exe`, copies the mod into Herdr, writes the autostart entry and starts the collector. Running it again later stops the collector, replaces the program and starts the new one, while Herdr runs. `./sync.sh` (`.\sync.ps1`) copies only the mod's files and leaves `bin/` alone. Then:

```sh
herdr-app check ~/.config/herdr-app/mods/time-tracker
```

The `herdr-time` command line prints to the console it was started from, but PowerShell and cmd don't wait for it (it is built as a windowless program), so its output can land after the prompt. Pipe it for clean output, `herdr-time status | Out-String`, or use Git Bash. Besides `status`, `session [start|end]` starts or ends a session and `restart` starts the collector again, detached.

The mod opens as a 920×580 popup (`mod.json` has `place: none`):

```sh
herdr-app do mod.open --name time-tracker
```

Where to put the chip and the panel in a layout of your own is in [RECIPE.md](RECIPE.md).

## Where the data is

`%LOCALAPPDATA%\herdr-time\`: the raw samples (`raw\`), corrections, closed periods, `sessions.json`, `projects.json` (your projects and rules; a starter is written on first run) and an optional `settings.json` (`idle_minutes`, `carry_minutes`, `session_idle_minutes` (45), `pause_minutes` (15), `port`, `poll_seconds`, `day_start`). Nothing leaves the computer: the collector answers only on `127.0.0.1:47613`.

## Limits

- Windows only, and only Herdr's agents.
- IDEA and Firefox are told apart by the window title for now, so a project rule for their folder or URL is a title match.
- A report's end is cut at the present moment: time that hasn't happened yet stays open.
- A closed period keeps its snapshot after a rule change, so a day's totals can differ from the sum of the exports until the period is reopened.
- Minutes already exported for one project can't be exported for another: the export answers 409 `closed` with the first project's period until that one is reopened.
- The preview is the collector's dry run of the export, so it matches the file.

## Development

```sh
node --test "time-tracker/tests/*.test.cjs"
go -C time-tracker/helper test ./... && go -C time-tracker/helper vet ./...
```
