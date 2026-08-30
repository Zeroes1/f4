# Следующее задание для `conptyreconcile`

Продолжи задачу по `docs/CONPTYRECONCILE_PLAN.md` с GitHub `main` после
checkpoint-коммита `03bfbb8696e1b2cf139420a74aea55dcd55e9f37`.

## Нормативная граница

Единственный нормативный источник — Microsoft OpenConsole:

- commit `e9b4e2e18fb1b9cee6839969d42cd0f95d228926`;
- tag `v1.12.10982.0`;
- x64 host `OpenConsole.exe`, version resource
  `1.12.220408003-release1.12`;
- host SHA-256
  `14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`.

Сверь `origin/main` с этим checkpoint перед работой. Не возвращай удалённый
код, старые field dumps, captures или результаты. Не используй другие conhost,
Windows Terminal, inbox conhost, Wine, Linux pty или старые captures. f4 не
изменяй и ни с чем в f4 не интегрируй.

## Что уже зафиксировано

В `tools/conptyreconcile` зафиксирован source-audit checkpoint: buffer/row/cell
и reflow-путь, parser/VT state, UTF-8 boundary, cursor/scroll/tab-stop/reset
порядок, alternate buffer, renderer attribute state, delayed random scheduler,
replayable capture structures и fail-closed pinned-host boundary. В
`AUDIT_LEDGER.md` записаны source paths и обнаруженные внешние gaps.

Последняя локальная проверка была только compile-only:
`go test -run '^$' ./tools/conptyreconcile` и `git diff --check` прошли.
Полные тесты, fuzz, `-race`, mock gate, 300 seeds и pinned-host gate ещё не
запускались и не должны считаться пройденными.

## Обязательный следующий порядок

1. Выполни три независимые проверки до любых тестовых прогонов:
   - symbol/source-path и SHA manifest;
   - построчная проверка control-flow, branch order, defaults, failure paths и
     boundary conversions для каждого используемого MS symbol;
   - negative/provenance audit, исключающий эквивалентные реализации,
     догадки, старые host data и неподтверждённые семантики.
   Каждая проверка должна читать фактический pinned source. Нельзя превращать
   набор строковых якорей в фиктивный PASS. Пока хотя бы одна ветвь или порядок
   не сверены, `runTransitionAudit` обязан оставаться hard FAIL.

2. Закрой source-backed gaps в parser, buffer/row/cell, reflow, VT renderer,
   `renderer/base`, cursor/grid и Windows launcher только буквальной
   транспиляцией pinned source. Если кода MS в pinned tree нет, сначала
   проверь документированные данные, изолируй реконструкцию и внеси точную
   запись в audit ledger. Не подменяй внешний Windows API собственной
   эквивалентной логикой.

3. Отдельно доведи renderer lifecycle по pinned
   `src/renderer/vt/{paint.cpp,XtermEngine.cpp,state.cpp,invalidate.cpp,math.cpp}`
   и `src/renderer/base/renderer.cpp`: invalid map, dirty rectangles,
   viewport-to-buffer mapping, scroll delta, `ScrollFrame`, resize suppression,
   first/quick paint, new-bottom state, cursor deferral and exact byte order.
   Текущий frame helper всё ещё нельзя объявлять source-faithful: он не
   воспроизводит полный dirty/invalidation lifecycle. Hyperlink/process-id и
   Windows-only seams остаются блокирующими, если источник их не предоставляет.

4. После завершения аудита добавь проверяемые assertions для всех пунктов
   плана: CJK/wide/ambiguous, combining/variation/surrogate/grapheme, bidi,
   equal strings without merge/loss, random patterns, widths `N-1/N/N+1`,
   width 1/exact multiples, long genuine wraps, blanks, cursor moves, erase,
   tabs, VT, arbitrary stream splits including UTF-8/CSI/OSC, live/frame
   interleaving, partial frame, eviction, padding, incremental chunks,
   mirror/re-wrap/viewport/coordinates/scroll and replayable logs/dumps.

5. Добавь реальную Windows-команду (минимум recursive `dir` с маркерами
   завершения), synchronized rapid identical/narrowing/widening resizes while
   output is active, randomized delays at arbitrary boundaries, fuzz всех
   границ и отдельный `go test -race`. Никакой проверки по одному отсутствию
   panic или по самосогласованности двух самодельных моделей.

6. Только после полного mock PASS прогони ровно 300 независимых seeds,
   сохрани seed list и per-seed logs/dumps. Затем используй тот же список и те
   же assertions против проверенного pinned host. Executable обязан сам
   проверить version и SHA-256 host и завершиться с ошибкой при несовпадении.

7. Интеграционный blocker нельзя скрывать: stock pinned ConPTY предоставляет
   один неразмеченный output byte stream, поэтому split live/frame при
   interleaving нельзя восстанавливать timestamp/read boundary/marker/parser
   heuristic. Если source-backed channel не найден, оставь host gate hard
   blocked и явно запиши причину в отчёте; не объявляй цель выполненной.

## Условия завершения

Завершение разрешено только при отсутствии source gaps, неопределённостей и
непройденных seeds, при полном отчёте и verified hosted result. Тогда закоммить
результат в `main`, отправь на GitHub, приложи один архив с exe и нужной
версией OpenConsole, приложи отчёт и остановись до следующей команды.
