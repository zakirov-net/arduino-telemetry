# Project rules (Cline workspace)

- Перед изменением Go-кода проверь, что `go build ./...` проходит.
- Перед изменением фронта запусти `cd web && npm run build`.
- Не коммить `build/`, `data/`, `.env`, `poller.env`.
- `internal/` — общий код; если меняешь `internal/storage` или `internal/models`, пересобери **все** Go-бинарники.
- Serial-порт: не менять baud 9600 на другое значение без обновления `poller.env` и скетча.
