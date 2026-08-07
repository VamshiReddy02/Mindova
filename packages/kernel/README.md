# Mindova Go Kernel

> **The shared foundation for every Go service in the Mindova platform.**

The **Mindova Go Kernel** is a shared library that contains the common code every Go service needs. Instead of rewriting the same functionality in every service, developers can reuse the kernel and focus on building business features.

The kernel provides reusable packages for:

* Configuration management
* Structured logging
* HTTP server utilities
* Middleware
* Health checks
* Metrics and tracing
* WebAssembly (WASM) plugin runtime

The kernel is **not a framework** and does not contain any business logic. It simply provides the building blocks that make every Go service consistent, reliable, and easier to maintain.

By using the Mindova Go Kernel, all services follow the same standards, reduce duplicate code, and share a common engineering foundation.
