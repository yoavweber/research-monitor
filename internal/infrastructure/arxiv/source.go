package arxiv

// SourceArxiv identifies arXiv as the upstream source on every paper.Entry
// produced by this package. Persistence joins on (source, source_id), so the
// constant must stay stable across releases.
const SourceArxiv = "arxiv"
