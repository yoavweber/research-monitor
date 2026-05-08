// Package pdf is the domain package for PDF artifact storage and retrieval.
//
// It defines the domain types, ports, and error sentinels that govern how
// PDF bytes are addressed, fetched from upstream sources, and persisted to
// the artifact store. The package contains no infrastructure: concrete
// fetchers, object-store clients, and repositories live under
// internal/infrastructure and are wired in at bootstrap.
//
// Dependency rule: this package may import only the Go standard library
// and other domain subpackages under internal/domain/shared. It must not
// import application, infrastructure, interface, or third-party packages.
package pdf
