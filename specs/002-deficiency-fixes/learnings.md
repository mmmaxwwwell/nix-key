# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## T009-fix: Struct tag validation

- `go-playground/validator/v10` treats `required` as a restricted tag — cannot be overridden via `RegisterValidation`. It short-circuits: if `required` fails on a zero-value int, subsequent tags (`min`, `max`) are never checked. Workaround: map `required` → `min` for numeric fields in error formatting.
- After `go get`, must run `go mod vendor` before tests will work (vendored project).

