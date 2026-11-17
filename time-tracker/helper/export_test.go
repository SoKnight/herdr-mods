package main

import (
	"strings"
	"testing"
)

func previewMonth() monthReport {
	return monthReport{Project: "u", Month: "2026-11", Today: "2026-11-16", Running: true,
		Rows: []monthRow{
			{Label: "1 ноя", Text: "1 ноя", From: "2026-11-01", To: "2026-11-01", Me: 90, Fact: 90, Agents: 120},
			{Label: "2 – 8 ноя", Text: "2–8 ноя", From: "2026-11-02", To: "2026-11-08", Workdays: 4, Plan: 1560, Me: 870, Fact: 870, Agents: 1130, Notes: []string{"экран заказов: фильтры и пагинация", "созвон по ТЗ платёжного модуля"}},
			{Label: "9 – 15 ноя", Text: "9–15 ноя", From: "2026-11-09", To: "2026-11-15", Workdays: 5, Plan: 1950, Me: 1310, Fact: 1310, Agents: 1730, Notes: []string{"OrderService: ретраи и идемпотентность", "документация по деплою"}},
			{Label: "16 – 22 ноя", Text: "16 ноя", From: "2026-11-16", To: "2026-11-16", Workdays: 1, Plan: 390, Me: 181, Fact: 181, Agents: 343, Current: true, Notes: []string{"платёжный модуль: вебхуки"}},
		},
		Total: monthRow{Workdays: 10, Plan: 3900, Me: 2451, Fact: 2451, Agents: 3323},
		Goal:  &universeGoal, Plan: 7800,
		Pay: monthPay(&Pay{Kind: "fixed", Amount: 100_000}, 2451, 7800, []Bonus{{Title: "Релиз платёжного модуля в срок", Amount: 15_000, Day: 12}}),
	}
}

func TestExportTextMatchesThePreview(t *testing.T) {
	assertEq(t, exportText("Universe", previewMonth(), exportOpts{Notes: true, Pay: true}), `Universe — ноябрь 2026
Отработано на 16.11: 40ч 51м

1 ноя · 1ч 30м
2–8 ноя · 14ч 30м
  — экран заказов: фильтры и пагинация
  — созвон по ТЗ платёжного модуля
9–15 ноя · 21ч 50м
  — OrderService: ретраи и идемпотентность
  — документация по деплою
16 ноя · 3ч 1м
  — платёжный модуль: вебхуки

Оплата: 100 000 ₽ + премия 15 000 ₽ = 115 000 ₽
`)
}

func TestExportTextGoalAgentsAndPastMonth(t *testing.T) {
	m := previewMonth()
	m.Running = false
	got := strings.Split(exportText("Universe", m, exportOpts{Goal: true, Agents: true}), "\n")
	assertEq(t, strings.Join(got[1:5], "\n"), "Отработано: 40ч 51м\nЦель: 65ч · разница −24ч 9м\n\n1 ноя · 1ч 30м · агенты 2ч · цель — · +1ч 30м")
	assertEq(t, got[5], "2–8 ноя · 14ч 30м · агенты 18ч 50м · цель 26ч · −11ч 30м")
	m.Pay = monthPay(&Pay{Kind: "hourly", Amount: 2000}, 2451, 7800, []Bonus{{Amount: 15_000}, {Amount: 5_000}})
	assertEq(t, payLine(m.Pay), "Оплата: 81 700 ₽ (40ч 51м × 2 000 ₽/ч) + премии 20 000 ₽ = 101 700 ₽")
	assertEq(t, payLine(monthPay(&Pay{Kind: "fixed", Amount: 100_000}, 0, 0, nil)), "Оплата: 100 000 ₽")
}

func TestExportCSV(t *testing.T) {
	m := previewMonth()
	m.Rows[3].Notes = []string{`=HYPERLINK("x")`, "две\nстроки"}
	assertEq(t, exportCSV(m, exportOpts{Notes: true, Pay: true, Agents: true, Goal: true}), "\ufeff"+
		"С;По;Раб. дн.;Цель, мин;Моё, мин;Разница, мин;Агенты, мин;Заметки\r\n"+
		"01.11.2026;01.11.2026;0;0;90;90;120;\r\n"+
		"02.11.2026;08.11.2026;4;1560;870;-690;1130;\"экран заказов: фильтры и пагинация; созвон по ТЗ платёжного модуля\"\r\n"+
		"09.11.2026;15.11.2026;5;1950;1310;-640;1730;\"OrderService: ретраи и идемпотентность; документация по деплою\"\r\n"+
		"16.11.2026;16.11.2026;1;390;181;-209;343;\"'=HYPERLINK(\"\"x\"\"); две\r\nстроки\"\r\n"+ // a line break inside a field comes out as CRLF too
		"Итого;;10;3900;2451;-1449;3323;\r\n"+
		"\r\n"+
		"Фикс за ноябрь;100000\r\n"+
		"Премия: Релиз платёжного модуля в срок (12 ноя);15000\r\n"+
		"Итого к оплате;115000\r\n")
	assertEq(t, strings.Split(exportCSV(m, exportOpts{}), "\r\n")[0], "\ufeffС;По;Моё, мин")
}
