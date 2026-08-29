# conptyreconcile промежуточный статус

Дата фиксации: 2026-08-29

Это промежуточная фиксация работы, а не результат обязательной проверки и не
готовый пробник для Windows.

## Нормативная версия

- OpenConsole source: `e9b4e2e18fb1b9cee6839969d42cd0f95d228926`
- source tag: `v1.12.10982.0`
- pinned host version: `1.12.220408003-release1.12`
- pinned host SHA-256: `14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`

## Что зафиксировано

В `tools/conptyreconcile/` собран новый standalone-порт с отдельными
частями buffer/row/cell/reflow, cursor и attributes, UTF-16/UTF-8, VT parser,
frame/live capture, resize/interleaving, replayable logs и Windows-only
ConDrv/ConPTY launcher. В план добавлено требование случайных задержек и
отдельного `go test -race` прогона.

Старый удалённый код, старые field dumps и старые результаты в эту фиксацию
не возвращались.

## Проверенное на момент фиксации

- `go build -o /tmp/conptyreconcile-linux-check ./tools/conptyreconcile` —
  успешно.
- `GOOS=windows GOARCH=amd64 go build -o /tmp/conptyreconcile-host-path.exe
  ./tools/conptyreconcile` — успешно.
- `git diff --check` — успешно.
- `go vet` и сборка корневого репозитория не получили изменений в f4 runtime;
  новый пакет остаётся отдельным standalone-инструментом.

## Что намеренно не объявлено выполненным

Тройной source-fidelity audit ещё имеет `transition/control-flow=FAIL`.
Открыты прямые сверки ветвлений и состояний в parser/stream/viewport-scroll/
frame/launcher, поэтому обязательные mock-тесты, ровно 300 seed-прогонов,
fuzzing, `go test -race` и pinned-host прогон не засчитывались и не должны
считаться зелёными по этой фиксации. Windows-host stage в Linux-среде не
запускался.

Следующий шаг после этой фиксации: закрыть каждый source gap прямой сверкой
с pinned tree, выполнить три независимые проверки и только затем запускать
обязательный test/fuzz/race gate.
