// Package di is a composition-root registry for Edadai Go services.
//
// Intended provider layers, from the outside in:
//
//	CONFIG
//	INFRASTRUCTURE
//	CLIENT
//	REPOSITORY
//	SERVICE
//	HANDLER
//	WORKER
//	RUNTIME
//
// Rules:
//
//   - The composition root can depend on everything.
//   - Infrastructure never depends on modules.
//   - Modules never depend on the DI registry.
//   - Transport never accesses repositories directly.
//   - Modules own business logic.
//   - Runtime owns execution and process lifecycle.
//   - Infrastructure owns external resources.
//   - Cross-module dependencies use narrow contracts.
//   - Modules must not form circular dependencies.
//   - Messaging infrastructure knows nothing about business domains.
//   - DI constructs objects; it does not become a service locator.
//   - Resource lifecycle is managed centrally via Provider.Cleanup.
package di
