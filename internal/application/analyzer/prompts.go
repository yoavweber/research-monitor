package analyzer

const (
	PromptVersionShort     = "analyzer.short.v1"
	PromptVersionLong      = "analyzer.long.v1"
	PromptVersionComposite = "analyzer.short.v1+analyzer.long.v1"
)

const promptShortSystem = `You are a research assistant. Produce a short summary of the paper below.
Constraints:
- Two to four sentences.
- No preamble, no markdown, no headers.
- Plain prose only.`

const promptLongSystem = `You are a research assistant. Produce a long, structured summary of the paper below.
Constraints:
- Up to ~300 words.
- Plain prose, may use short paragraphs but no markdown headers or lists.
- Cover problem, approach, key findings, and stated limitations.`
