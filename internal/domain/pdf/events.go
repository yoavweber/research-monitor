package pdf

// Structured-log event names emitted by Store implementations.
// Exported here so dashboards, alerts, and tests share a single source of
// truth and a typo cannot silently break observability.
const (
	EventFetched  = "pdf.store.fetched"
	EventCacheHit = "pdf.store.cache_hit"
	EventFailed   = "pdf.store.failed"
)

// Failure categories used as the "category" field on EventFailed records.
// Each value corresponds to one of the error sentinels: CategoryInvalidKey
// pairs with ErrInvalidKey, CategoryFetch with ErrFetch, CategoryStore with
// ErrStore.
const (
	CategoryInvalidKey = "invalid_key"
	CategoryFetch      = "fetch"
	CategoryStore      = "store"
)
