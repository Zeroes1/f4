# Coverage: `internal/app/bootstrap_ctrlhandler_windows.go`

Кастомная задача Lunobot-2 по §22 п.3: покрыть Windows-обработчик событий консоли.

- На `main` Codecov после PR #1104: 63.14%; файл: 0.00% (29 строк).
- Тесты проверяют обработку Ctrl+C и Ctrl+Break, а также игнорирование прочих событий.
- Тесты Windows-only и не запускаются локально; CI будет проверен перед merge.
