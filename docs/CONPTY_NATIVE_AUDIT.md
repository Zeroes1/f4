# Native gate audit

Статус на 2026-08-31: гейт не готов; выполнен только фундамент standalone
инструмента. Заявление о готовности запрещено до native-прогона на пиннутом
хосте и закрытия всех разделов требований.

## Что подтверждено командой

После синхронизации `git fetch && git checkout main && git pull` дерево было
на `1d3db2e4` плюс рабочие изменения этого шага. Команда

```text
go test ./...
```

в `tools/conptyreconcile` прошла после удаления старого mock/grid-пути. Кэш
использован системный (`go env GOCACHE`), без task-local замены.

Новые unit-тесты проверяют, что поток режется только по явному `LF`, что
`CRLF` сохраняется как терминатор, что одинаковые строки не сливаются, и что
перевыкладка целой строки по ширине не изменяет её байты. Это не native
доказательство поведения хоста.

Реальный static-прогон после исправления точного ProductVersion:

```text
go run . -probe-static -report ../../artifacts/pinned-conpty-probe-static.json
native OpenConsole probe complete: ../../artifacts/pinned-conpty-probe-static.json
```

Проверка артефакта командой дала `raw_output=1324`, `.raw=1324`,
`equal=True`, SHA-256 `e03681a92f5c386d6ec8083c58e588dc02e7eb8c15c1d646877dac2813818780`.
В отчёте зафиксированы оба маркера ровно по одному и в правильном порядке.
Это закрывает только транспортную запись static-прогона; логическая история
и reflow ещё не сверены.

Dynamic-прогон командой

```text
go run . -probe -report ../../artifacts/pinned-conpty-probe.json
```

показал в сыром потоке повторную отрисовку begin-маркера при resize (в
первой сессии было 2 вхождения) и перестановку raw end-маркера в сессии
`121x40`. Это repaint-байты ConPTY; строгая проверка ровно одного маркера и
порядка должна выполняться после восстановления logical history, а не на
сыром потоке. Raw-слой сохраняет оба факта как предупреждения.

После перевода raw reorder в предупреждения dynamic-прогон завершился:
`80x25` — 1351 байт, 4 resize, 1 repaint warning; `1x1` — 1361 байт, 4
resize, 1 repaint warning; `121x40` — 1381 байт, 4 resize, без warning. Для
всех трёх сессий проверка `len(base64decode(raw_output)) == len(.raw)` и
побайтовое равенство дала `True`. Это подтверждает только native transport,
resize и запись артефактов; logical history/reflow пока не доказаны.

Эксперимент с control-stream parser остановлен и удалён: попытка обработать
`CSI`-перемещение как продолжение записи потребовала бы выводить границу из
экранной позиции. Это прямо запрещённая эвристика. Поэтому A/B остаются
открытыми до появления источника логических событий, где граница задана самим
потоком `LF`, а не восстановлена из repaint.

Команда `go run . -gate` сейчас намеренно завершается ошибкой после native
static/dynamic стадий с сообщением `native gate incomplete ...`: это защитный
гейт от ложной зелени, пока не реализованы проверки A–D.

## Handoff через текущий f4 pipeline

Добавлен opt-in диагностический тест `cmd/f4/TestNativeConPTYReplay`. Он
подаёт сохранённый native raw через `PanelsFrame.consumeLocalOutput` и
`AnsiParser` в настоящий `TerminalView`, после чего сверяет marker count и
полный payload. Это пока replay, а не live native session, поэтому D1 ещё не
закрыт и тест не включён в обычный прогон.

Команда на Windows:

```text
F4_NATIVE_CONPTY_REPLAY=<absolute path to pinned-conpty-probe-static-v6.json> go test ./cmd/f4 -run '^TestNativeConPTYReplay$' -count=1 -v
```

Прогон получил `raw=2852`, `f4_log=1293`, `begin=1`, `end=1`, но завершился
первым строгим несоответствием payload: `actual=1236`, `expected=1326`,
`first_diff=1`. В фактическом логе присутствуют две лишние начальные пустые
строки, а строка cursor/rewrite также имеет изменённое содержимое. Это
первое прямое измерение потери/перестановки на пути f4; оно оставлено
красным и не маскируется удалением control-последовательностей.

## Текущая реализация

- модуль инструмента самостоятельный: `github.com/unxed/pinned-conpty-probe`;
- маркеры: `__PINNED_CONPTY_PROBE_BEGIN__` и
  `__PINNED_CONPTY_PROBE_END__`;
- OSC-title: `pinned-conpty-probe`;
- кэш: `%LOCALAPPDATA%\pinned-conpty\1.12.10983.0\`;
- проверка личности требует именно пиннутый `OpenConsole.exe` из
  `docs/PINNED_CONSOLE.md` и проверяет живой процесс;
- старый mock/grid-код и неиспользуемый сценарный runner удалены/исключены;
  текущий Windows fallback в основном проекте не менялся.

## Открыто

Не выполнены native A1–A4, B1–B3, C1–C4 и D1–D3: точная история и экран/
scrollback/cursor, динамический reflow, реальные команды, произвольные
границы чтения, lifecycle, быстрые resize, рекурсивный `dir`, fuzzing и 300
сидов. До их прогона нельзя считать гейт пройденным.

## Сохранённое расхождение артефактов

Для ранее созданного `native-openconsole-probe-static.json` проверка показала:
`base64decode(raw_output)` — 957 байт с 49 `CR`, файл `.raw` — 908 байт с нулём
`CR`; поэтому прежняя запись о совпадении была ошибочной. Хэш совпадал с
`raw_output`, но не с файлом на диске. Критерий закрытия остаётся строгим:
`sha256(file) == raw_sha256` и побайтовое равенство `file` и
`base64decode(raw_output)`, проверенные командой.

Прежние наблюдения о 46/211 и зависании статического `1x1` сохранены как
диагностические факты удалённого пути; они не являются доказательством
текущего native-гейта.

## Новый host-stream прогон

После чтения `docs/PINNED_HOST_FACTS.md` и исходника `e9b4e2e` добавлен
разбор, который режет поток только по host-emitted `CRLF`; resize-frame
отмечается собственным output-offset при вызове `ResizePseudoConsole`.
Ни экранная сетка, ни флаг wrap, ни хвостовые пробелы в этом разборе не
используются. Unit-тесты проходят с системным кэшем
`C:\Users\Windows\AppData\Local\go-build`.

Команда:

```text
go run . -probe-static -report ../../artifacts/pinned-conpty-probe-static-v6.json
native OpenConsole probe complete: ../../artifacts/pinned-conpty-probe-static-v6.json
```

Проверка дала `base64decode(raw_output) == .raw` побайтово (`2852 == 2852`),
`sha256=.raw` совпадает с `raw_sha256`. Static-сессия содержит 89 host-CRLF,
89 записанных logical-line объектов и не содержит resize-frame, что ожидаемо
для режима без resize. Динамический прогон тем же payload завершился для
80x25/1x1/121x40 (2890/1956/3279 байт); на 1x1 сохранено предупреждение о
двух begin-маркерах как о repaint.

Важное наблюдение: в этих бинарных прогонах не обнаружена последовательность
`ESC[8;H;Wt`; вместо неё видны CUP-перемещения (`ESC[...H`). После чтения
`src/host/PtySignalInputThread.cpp:133-146` это ожидаемо: PTY-resize сразу
вызывает `s_SuppressResizeRepaint`, чтобы не эхо-отправлять размер терминалу.
Границы repaint теперь должны отмечаться собственными output-offset в момент
`ResizePseudoConsole`, а содержимое таких кадров не участвует в истории.

Повторный seed-прогон `go run . -seeds -report ../../artifacts/pinned-conpty-seeds-v7.json`
выполнил все 300 сессий и записал 300 `.raw`; две семантические ошибки маркеров
остались в отчёте: seed 21 (width 1, begin) и seed 115 (width 121, end).
Это открытые случаи устойчивости и не доказательство прохождения D3.

Отдельный прогон незавершённой строки:

```text
go run . -partial -report ../../artifacts/pinned-conpty-partial-v1.json
```

завершился успешно. Результат содержит одну host-логическую строку и один
явный `CRLF`; четыре собственных resize-offset записаны (`0,0,8,116`), а
артефакт проверен побайтово и по SHA. Это подтверждает только транспортную
последовательность «половина строки → resize → хвост»; проверка того, что
история не принимает repaint как новую строку, ещё не реализована.
