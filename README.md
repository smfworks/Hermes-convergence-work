# Hermes Convergence Work

Reference implementations for the four Hermes/OpenClaw convergence gaps identified in the Dr J diagnostic.

## Packages

- `healthv1/` — `health_event_v1` schema, validation, JSONL store, and emitter.
- `recovery/` — context-aware recovery engine. Proposes scored, shadow-mode recovery actions based on event severity, session type, user presence, in-flight mutations, and recent failure history.
- `memory/` — placeholder for the unified memory contract / Mnemosyne (gap #3).
- `contracts/` — placeholder for the tool contract registry (gap #4).

## Design principles

- No external dependencies beyond the Go standard library.
- Library-free, portable packages that can be copied into OpenClaw, Hermes, or the dashboard.
- Shadow-mode semantics: observability and recovery packages propose, they do not execute.

## Running tests

```bash
go test ./... -count=1
```

## License

MIT
