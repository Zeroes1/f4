# Coverage: internal/vtvibe provider

Кастомная задача по § 22 п. 3 инструкции Лунобота: повысить покрытие internal/vtvibe, начиная с provider.go (4,61% по Codecov main на момент взятия; пакет — 54,49%).

В этот PR перенесены тесты HTTP-провайдера на актуальную структуру проекта после переноса пакета из vtvibe в internal/vtvibe: успешные Chat/Models с проверкой запроса и авторизации, строковый и составной JSON-контент, ошибки API и HTTP, отсутствие ключа, retry/cancellation, разбор statusError, backoff и вспомогательные функции.

Локальные Go-сборки и тесты не запускались согласно LUNOBOT.md; результат проверяется GitHub Actions.
