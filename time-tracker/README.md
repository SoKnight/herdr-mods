# Time Tracker

Where your working day went, per project, for [Herdr Desktop](https://herdr.dev/) on Windows. A small collector watches your keyboard and mouse activity, the foreground window, the lock screen, calls and your Herdr agents, and a panel shows the day as a strip you can correct by selecting time. A week, a month and a month report with pay, as text to copy or a CSV file for a client, follow from the same numbers.

<!-- preview.png: added once the panel is checked on a live day -->

- **Session** (Сессия): the current working session. It starts after 45 minutes without activity and ends after 45 minutes without it (`session_idle_minutes`), or by "Завершить" / "Начать сессию". Stretches split at breaks over 15 minutes (`pause_minutes`); it shows the switches between projects and the longest focus.
- **Day** (День): 06:00 to 06:00, as one strip (the computer on, my time coloured by project, a line at now) with a thin agents strip under it, then the sessions with their stretches and the breaks between them. Drag over a range up to now for its menu: assign a project (or "Без проекта"), set the kind of work, leave it out ("Исключить — не работа"), add a note, or make a rule from the window with the most time in it (the rule item opens the rule editor). "+ Новый проект…" at the end of the project list makes a project from here. A saved correction says "Правка #n сохранена." with "Отменить", which undoes that last one. "+ вручную" adds time by hand to the day that is open: from–to (the end stops at now), a project, a kind of work and a note; it replaces recorded time it overlaps and says how much. The history icon opens История правок.
- **Разбор** (a link under the day, "разобрать «Другое» →"): the day's windows without a rule, each with a suggested project or "не работа". Принять, Исключить ("не работа") or another project; "Новые правила" lists the rules the decisions make, with "применить к прошлым дням" (from the 1st of this month, or that day if earlier). "Готово" saves them. A decision without its rule checked changes only that day. A correction that falls in a closed month refuses the whole call; rules alone are allowed.
- **История правок** (the icon on День): corrections, rule, goal and pay changes, days off and month closes by day or month, each with "Отменить". Undoing a correction deletes it; undoing a rule change puts the rule back as it was (a rule added is removed), unless that rule changed again since; undoing a goal or pay change puts the versions back, unless they changed since; undoing a day off removes it; "открыть заново" on a closed month reopens it.
- **Projects** (Проекты): the projects in a sidebar, each with its own colour (one of six) and an archive; with Досуг, Не работа and Другое below them. One ordered list of rules holds everything the collector matches: a folder, an app (with an optional title), a title, a Figma file or a Minecraft server. The first rule that matches a window wins, whatever its kind. The rule editor shows the matches of the last 7 days and, when an earlier rule keeps some of them, which one ("Поднять выше" moves the rule above it). A change applies "с сегодня" or "и к прошлым дням с 1-го числа"; days before that keep what they were. Не работа and Досуг take time out of mine. "+" makes a project and offers hints from Другое.
- **Goals** (цели, and pay in the same dialog «Цель и оплата»): per project, "не меньше" / "не больше" N ч в день, неделю or месяц, set with the pencil on a project in Проекты or "+ цель" on a new one. A day goal picks the weekdays, may follow the production calendar of Russia (holidays out, moved working days in) and may count time on unplanned days ("время в выходные идёт в зачёт"); week and month goals spread over every calendar day. "Действует с" is the 1st of this month, today or the 1st of next month: a new version replaces the ones from that day on and earlier days keep theirs ("история" lists them). "Убрать с …" ends it from that day; both buttons name the day. The dialog sends only the side you changed, goal or pay, and each change is an entry in История правок with "Отменить". Only my active time counts toward a goal.
- **Week** (Неделя): seven day strips with the agents' and my time, the day goal's difference (цель дня) and the totals; click a day to open it. The right panel shows the week's projects against their goals, the agents, switches and longest focus, and Досуг.
- **Month** (Месяц): a calendar of days with my time, a stripe of 8 hours, a tick for the day goal and a pale forecast on the days to come; a holiday is marked "праздник". Drag over days (shift-click for one) for "Отметить дни": отгул, отпуск or больничный, and which projects' plan drops on them. A day off is undone in История правок (`23–27 ноя · отгул`). The right panel has the month's projects, the agents and the forecast.
- **Report** (Отчёт): a project and a month, as a table by week (Monday to Sunday, cut at the month) or by day, up to today: "Раб. дн.", "Цель", "Моё", "Разница" and "Агенты" (a switch hides them), a totals row, and a footnote on how working days and weekends count; with no goal, "Цель" and "Разница" are left out. A goal strip above it (`изменить`), and on the right the month's kinds of work and **pay**: a fixed sum a month or an hourly rate, set per month in the goal dialog and kept from there on, with the rate by the month's plan (`≈ 769 ₽/ч`), the bonuses you add by hand ("+ добавить": a title, a sum, a day of the month; `×` removes one) and the total. "Скопировать текстом" opens «Отчёт текстом» (rows, and notes, pay, agents, goal and difference as options; the preview is what gets copied); "CSV" saves the same month to `Downloads\{project}_{YYYY-MM}.csv` for Russian Excel.
- **Month close:** once a month is over, "Закрыть месяц" on Отчёт (a checklist first: Другое not triaged, bonuses missing for projects with pay, days off, corrections; "отчёт отправлен заказчику" is remembered) closes it for every project and freezes its time, goals, calendar, days off and pay. Rules and goal or pay edits leave it alone, so its Неделя, Месяц, День and Отчёт don't move; corrections, Разбор, days off (adding and removing) and bonuses (adding and removing) there are refused. "открыть заново" on Отчёт or in История правок reopens it, and undoing that puts the closed month back, unless it was closed again since or changed while open (a correction, day off or bonus in it, a goal or pay change reaching it: close it again instead). A running month can't be closed.
- **Настройки:** the session and pause limits, the day start, "видео со звуком на переднем плане — активность", the autostart, the two reminders and "Не записывать заголовки окон" (patterns with "+ добавить"). Saving writes `settings.json` and applies at once; keys the screen doesn't edit stay as they were. `herdr-time settings` prints them, `settings set '<json>'` changes some (a bad value answers 400 and leaves the file alone). Autostart adds or removes the `herdr-time` value under `HKCU\...\Run`, and a later `go run ./deploy` keeps it off when `autostart` is false.
- **Agent waiting:** a robot pill on the chip ("агент ждёт тебя", `ждёт Nм`) and light segments on the agents strip when a Herdr terminal's agent is blocked or done and waits for me; "за сегодня агенты ждали тебя Nм" on the chip's card; `ждали Nм` on the summary line of Сессия and День and on a project's goal line (the day's waits; stretch rows show only commits). It comes from Herdr's terminal states in `hc events` (a snapshot, then each terminal's status changes as they happen), which Herdr gets from Claude Code, so nothing is installed into `~/.claude`.
- **Commits:** my commits (by the repo's `user.email`; a repo without one lists every author's commits) in the repos at, or one level under, the folder rules, as notes. A stretch shows its commits from the last 48 hours, read in the background once a minute, the export adds them as bullets with `--commits` ("коммиты" in «Отчёт текстом»), and Проекты shows `N за 7 дней` on a project. They are read live from git, even for a closed month, and are never a number in a report. `herdr-time commits --days N` lists them.
- **Reminders:** a toast when the collector has been silent for 10 minutes, and, once a day per Herdr start, when last month has my time and isn't closed.
- **Chip:** a status-bar block. It shows today's time and the project in focus, "отошёл" with how long after a pause in a session, a play icon instead of the clock when no session runs, and "нет данных" when the collector is silent (День then offers "Перезапустить"). Hovering opens a card with today's total, the session's start (or "сессии нет"), the agents and the projects (the goal projects against their goals, with the month's forecast); a click opens the panel. It also passes Herdr's signal (the folder you work in) to the collector; the agents the collector follows itself through `hc events`.

## What it counts

- **My time** is time with input in the last 5 minutes (`idle_minutes`), or a call, and not while the screen is locked. Which project it belongs to comes from the foreground window: its folder, title, Figma file, Minecraft server or app, matched by the one list of rules in `projects.json` (first match wins; a signal's project name counts only when no rule matches). Time after the last project carries on for up to 15 minutes (`carry_minutes`). Не работа and Досуг are not my time and are not in sessions. They don't take the carry, and they don't break it either: a window without a project after them still goes to the last real project before them while it is within 15 minutes of that project's end.
- **Agents' time** is the time Herdr agents work, from `hc events`, per project. It is only ever a side figure: it is never added to my time, and in a file it is its own line, outside the total.
- **Agent waiting** is the time a Herdr terminal's agent is `blocked` or `done`, as a side figure like the agents' time. One wait counts at most `session_idle_minutes` (a terminal left `done` for days is not waiting), and only while the PC is on and unlocked. It is not part of a month record, and agents outside Herdr's terminals aren't seen.
- **Video with sound:** with `video_activity` on, the foreground app that holds an audible session on the default playback device counts as activity, so watching a video isn't idle. Audio in the background doesn't count.
- **Hidden titles:** a pattern in `hidden_titles` (case-insensitive, `*` is a wildcard) that matches the exe name or the title keeps the time and drops the title text. It applies to new data only. With no title, title rules (and app rules with a title) can't match that window, so its time may fall to Другое unless a folder or app rule takes it.
- **Changing a setting recounts the open month:** `video_activity`, `day_start`, `session_idle_minutes` and `pause_minutes` apply to the stored samples, so the past days of the month that isn't closed (and its pay) are counted again by the new values; a closed month stays as it was frozen.
- **A day** starts at 06:00 (`day_start`), so a night session stays with the day it began.
- **Goal figures:** plan to date is the goal over the days up to and including today (a day goal: planned days; week: minutes × days / 7; month: minutes × days / days in the month); balance is the fact minus it, forecast is fact × the period's plan / plan to date, and `дальше X в раб. день` (a "не меньше" goal only) is what is left over the planned days left, rounded up. A day off drops the plan of the projects ticked in it.
- **Pay** is in whole rubles, each amount rounded half up once: a fixed sum is paid as it is and its rate is sum × 60 / the month's plan minutes (no plan, no rate); hourly is my minutes × rate / 60; agents are never paid; the total is that plus the bonuses.
- **Minutes are rounded per project and day** (half up) on every screen, and only then added up: День, Сессия, Неделя, Месяц and the chip agree to the minute.
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

The `herdr-time` command line prints to the console it was started from, but PowerShell and cmd don't wait for it (it is built as a windowless program), so its output can land after the prompt. Pipe it for clean output, `herdr-time status | Out-String`, or use Git Bash. Besides `status`, `session [start|end]` starts or ends a session and `restart` starts the collector again, detached. `rules` lists the rules with their 7-day times, `rule add|set|rm|preview` changes them, `project add|set` makes or edits a project and `hints` suggests rules for Другое. `goal set <project> <json>` sets a goal, `pay set <project> <json>` a pay (`{"from":"2026-11","kind":"fixed","amount":100000}`; kind `fixed`, `hourly` or `none`), `bonus add <json> | rm <id>` bonuses, `dayoff add <json> | rm <id>` marks or unmarks days off, `plan --from --to` (days, `YYYY-MM-DD`) prints the days' plan and fact and `goals [--day]` the goals' state. `inbox [--day]` lists a day's Другое windows and `inbox apply` decides them, `history --from --to` lists corrections and changes and `history undo <id>` undoes one. `month report <YYYY-MM> --project <id> [--rows week|day]` prints a project's month, `month state <YYYY-MM>` whether it is closed, `month close <YYYY-MM> [--sent]` closes it and `month reopen <YYYY-MM>` opens it again. `export --project <id> --month <YYYY-MM> [--format text|csv] [--rows week|day] [--notes] [--pay] [--agents] [--goal] [--out <file>]` prints the month as text (or CSV) and writes it to `--out`; it closes nothing. `periods` lists closed periods and `reopen <id>` reopens one.

The mod opens as a 920×580 popup (`mod.json` has `place: none`):

```sh
herdr-app do mod.open --name time-tracker
```

Where to put the chip and the panel in a layout of your own is in [RECIPE.md](RECIPE.md).

## Where the data is

`%LOCALAPPDATA%\herdr-time\`: the raw samples (`raw\`), corrections, `history.json` (rule, goal, pay and month changes, for undo), `periods.json` (month records, each with the month's time, plan days and pay; and v1's closed periods, which still apply and show in `periods`), `bonuses.json`, `sessions.json`, `calendar.json` (the production calendar: `holidays` and `workdays` moved from weekends per year; copied once from the shipped one and editable; a year the file lacks falls back to the shipped one, a year neither has counts Mon–Fri), `daysoff.json`, `projects.json` (`projects`, with their goals and pay, and `rules`; a starter is written on first run; a file from before rules is converted once and kept as `projects.v1.json`) and an optional `settings.json` (`idle_minutes`, `carry_minutes`, `session_idle_minutes` (45), `pause_minutes` (15), `port`, `poll_seconds`, `day_start`, `video_activity`, `autostart`, `silent_warning`, `close_reminder` (all on), `hidden_titles` (none), `minecraft_logs`: default `%APPDATA%\.minecraft\logs\latest.log`, the newest of the listed files is read). Nothing leaves the computer: the collector answers only on `127.0.0.1:47613`.

## Limits

- Windows only, and only Herdr's agents. There is no sync with a Mac or another device.
- Sound is read from the default playback device only; a video playing on another device isn't seen.
- Commits come from `git log` over branches, tags and remotes (not stash or notes) with a 5 s limit per repo and 10 s for all repos of one report or export, so a slow or missing git leaves its commits out. Stretches show only the last 48 hours' commits, up to a minute late.
- IDEA and Firefox are told apart by the window title for now, so a rule for their folder is a title match.
- Minecraft's server comes from the game log; another launcher's game folder goes into `minecraft_logs`.
- A report's end is cut at the present moment: time that hasn't happened yet stays open.
- A closed month keeps its snapshot after a rule, goal or pay change, so its numbers can differ from what the rules give today until it is reopened (День shows the difference).
- Closing and reopening are for whole months and every project; the collector answers 409 `closed` for a refused write.
- Bonuses are typed in by hand: the tracker can't tell a release in time from a late one.
- The text, the CSV and the table come from one month report, so they agree to the minute and the ruble. There is no PDF and no Markdown file.

## Development

```sh
node --test "time-tracker/tests/*.test.cjs"
go -C time-tracker/helper test ./... && go -C time-tracker/helper vet ./...
```
