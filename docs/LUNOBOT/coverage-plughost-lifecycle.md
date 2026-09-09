# Лунобот: lifecycle coverage `internal/plughost`

Статус: добавлены детерминированные тесты для `application.go` и конструкторов/identity `transport_rpc.go`; локальные Go build/test не выполнялись по правилам `LUNOBOT.md`.

Основание задачи: после предыдущего extui-круга Codecov main был 60.88%; `application.go` оставался на 0.0% (7 строк), а `transport_rpc.go` — на 19.67% (61 строка).

Покрыты nil-host ветка `currentApp`, конструкторы RPC plugin/ring, имя RPC-плагина, fallback identity по пути, явная identity из манифеста и безопасное закрытие не запущенного plugin.
