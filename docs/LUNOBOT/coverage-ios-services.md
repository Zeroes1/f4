# Coverage: iOS service cancellation paths

Кастомная задача по § 22 п. 3 инструкции Лунобота: повысить покрытие plugins/ios/services.go (0% по Codecov main на момент взятия; main — 61,39%).

Добавлены тесты раннего завершения по отменённому context.Context для общего подключения, House Arrest и crash-report service. Тесты подтверждают, что отменённый запрос не вызывает попытку подключения к устройству.

Локальные Go-сборки и тесты не запускались согласно LUNOBOT.md; результат проверяется GitHub Actions.
