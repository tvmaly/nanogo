# core/contracts

Tiny cross-cutting capability interfaces for nanogo.

This package is intentionally narrow. It defines only stable request/result
shapes and implicit Go interfaces that many packages may consume without
depending on concrete implementations.

Do not put implementations, provider code, product policy, voice adapters,
tutor logic, or large facade APIs here.

Implementations usually live in `core/agent`, `core/tools`, `modules/*`, or
`ext/*`. Composition lives in `cmd/nanogo`.
