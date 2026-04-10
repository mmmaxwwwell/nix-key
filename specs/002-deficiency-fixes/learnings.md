# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## T009-fix: Struct tag validation

- `go-playground/validator/v10` treats `required` as a restricted tag — cannot be overridden via `RegisterValidation`. It short-circuits: if `required` fails on a zero-value int, subsequent tags (`min`, `max`) are never checked. Workaround: map `required` → `min` for numeric fields in error formatting.
- After `go get`, must run `go mod vendor` before tests will work (vendored project).

## T010-fix: ShutdownManager logging

- `slog.NewJSONHandler` writes structured JSON with `"level"` and `"msg"` keys — use `json.NewDecoder` with `decoder.More()` loop to parse multiple JSON objects from a single buffer (no newline splitting needed).
- daemon.go uses old `log` package for the agent backend but `log/slog` for ShutdownManager — these coexist fine since they're separate loggers.

