# Time Tracker

Where your working day went, per project, for [Herdr Desktop](https://herdr.dev/) on Windows. A small collector watches your keyboard and mouse activity, the foreground window, the lock screen, calls and your Herdr agents, and a panel shows the day as a strip you can correct by selecting time. A week, a month and a report with a Markdown or CSV file for a client follow from the same numbers.

<!-- preview.png: added once the panel is checked on a live day -->

- **Session** (Сессия): the current working session. It starts after 45 minutes without activity and ends after 45 minutes without it (`session_idle_minutes`), or by "Завершить" / "Начать сессию". Stretches split at breaks over 15 minutes (`pause_minutes`); it shows the switches between projects and the longest focus.
- **Day** (День): 06:00 to 06:00, as one strip (the computer on, my time coloured by project, a line at now) with a thin agents strip under it, then the sessions with their stretches and the breaks between them. Drag over a range up to now for its menu: assign a project (or "Без проекта"), set the kind of work, leave it out ("Исключить — не работа"), add a note, or make a rule from the window with the most time in it (the rule item opens the rule editor). "+ Новый проект…" at the end of the project list makes a project from here. A saved correction says "Правка #n сохранена." with "Отменить", which undoes that last one. "+ вручную" adds time by hand to the day that is open: from–to (the end stops at now), a project, a kind of work and a note; it replaces recorded time it overlaps and says how much. The history icon opens История правок.
- **Разбор** (a link under the day, "разобрать «Другое» →"): the day's windows without a rule, each with a suggested project or "не работа". Принять, Исключить ("не работа") or another project; "Новые правила" lists the rules the decisions make, with "применить к прошлым дням" (from the 1st of this month, or that day if earlier). "Готово" saves them. A decision without its rule checked changes only that day. A correction that falls in a closed period refuses the whole call; rules alone are allowed.
- **История правок** (the icon on День): corrections and rule changes by day or month, each with "Отменить". Undoing a correction deletes it; undoing a rule change puts the rule back as it was (a rule added is removed), unless that rule changed again since.
- **Projects** (Проекты): the projects in a sidebar, each with its own colour (one of six) and an archive; with Досуг, Не работа and Другое below them. One ordered list of rules holds everything the collector matches: a folder, an app (with an optional title), a title, a Figma file or a Minecraft server. The first rule that matches a window wins, whatever its kind. The rule editor shows the matches of the last 7 days and, when an earlier rule keeps some of them, which one ("Поднять выше" moves the rule above it). A change applies "с сегодня" or "и к прошлым дням с 1-го числа"; days before that keep what they were. Не работа and Досуг take time out of mine. "+" makes a project and offers hints from Другое.
- **Week and Month** (Неделя, Месяц): projects by kind of activity, with my time and the agents' beside them and every day below; click a day to open it.
- **Report** (Отчёт): a project, a period (week, month or your own dates; ‹ › step to the previous week or month), rounding to 1, 5 or 15 minutes (15 by default), a switch "Показать время агентов (для справки)" (off), and a Markdown preview. "Сохранить CSV" / "Сохранить Markdown" writes the file to a folder you pick (`~/Downloads` by default) and **closes the period** for that project.
- **Closed periods:** listed under the report with how far today's rules would move them. "Переоткрыть" lifts the lock.
- **Chip:** a status-bar block. It shows today's time and the project in focus, "отошёл" with how long after a pause in a session, a play icon instead of the clock when no session runs, and "нет данных" when the collector is silent (День then offers "Перезапустить"). Hovering opens a card with today's total, the session's start (or "сессии нет"), the agents and the projects; a click opens the panel. It also passes Herdr's signal (the folder you work in) to the collector; the agents the collector follows itself through `hc events`.

## What it counts

- **My time** is time with input in the last 5 minutes (`idle_minutes`), or a call, and not while the screen is locked. Which project it belongs to comes from the foreground window: its folder, title, Figma file, Minecraft server or app, matched by the one list of rules in `projects.json` (first match wins; a signal's project name counts only when no rule matches). Time after the last project carries on for up to 15 minutes (`carry_minutes`). Не работа and Досуг are not my time and are not in sessions. They don't take the carry, and they don't break it either: a window without a project after them still goes to the last real project before them while it is within 15 minutes of that project's end.
- **Agents' time** is the time Herdr agents work, from `hc events`, per project. It is only ever a side figure: it is never added to my time, and in a file it is its own line, outside the total.
- **A day** starts at 06:00 (`day_start`), so a night session stays with the day it began.
- **A call wins:** while Telemost holds the microphone, the time counts as the call's project and activity even if you type in another app, except in a window a Досуг rule matches. Fix such a stretch with a correction.

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

The `herdr-time` command line prints to the console it was started from, but PowerShell and cmd don't wait for it (it is built as a windowless program), so its output can land after the prompt. Pipe it for clean output, `herdr-time status | Out-String`, or use Git Bash. Besides `status`, `session [start|end]` starts or ends a session and `restart` starts the collector again, detached. `rules` lists the rules with their 7-day times, `rule add|set|rm|preview` changes them, `project add|set` makes or edits a project and `hints` suggests rules for Другое. `inbox [--day]` lists a day's Другое windows and `inbox apply` decides them, `history --from --to` lists corrections and rule changes and `history undo <id>` undoes one.

The mod opens as a 920×580 popup (`mod.json` has `place: none`):

```sh
herdr-app do mod.open --name time-tracker
```

Where to put the chip and the panel in a layout of your own is in [RECIPE.md](RECIPE.md).

## Where the data is

`%LOCALAPPDATA%\herdr-time\`: the raw samples (`raw\`), corrections, `history.json` (rule changes, for undo), closed periods, `sessions.json`, `projects.json` (`projects` and `rules`; a starter is written on first run; a file from before rules is converted once and kept as `projects.v1.json`) and an optional `settings.json` (`idle_minutes`, `carry_minutes`, `session_idle_minutes` (45), `pause_minutes` (15), `port`, `poll_seconds`, `day_start`, `minecraft_logs`: default `%APPDATA%\.minecraft\logs\latest.log`, the newest of the listed files is read). Nothing leaves the computer: the collector answers only on `127.0.0.1:47613`.

## Limits

- Windows only, and only Herdr's agents.
- IDEA and Firefox are told apart by the window title for now, so a rule for their folder is a title match.
- Minecraft's server comes from the game log; another launcher's game folder goes into `minecraft_logs`.
- A report's end is cut at the present moment: time that hasn't happened yet stays open.
- A closed period keeps its snapshot after a rule change, so a day's totals can differ from the sum of the exports until the period is reopened.
- Minutes already exported for one project can't be exported for another: the export answers 409 `closed` with the first project's period until that one is reopened.
- The preview is the collector's dry run of the export, so it matches the file.

## Development

```sh
node --test "time-tracker/tests/*.test.cjs"
go -C time-tracker/helper test ./... && go -C time-tracker/helper vet ./...
```
