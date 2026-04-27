package paper

// SourceArxiv is the wire-contract value used in paper.Entry.Source for
// arXiv-ingested papers. The string is part of the persisted composite key
// (source, source_id); changing it would split the catalogue.
const SourceArxiv = "arxiv"
