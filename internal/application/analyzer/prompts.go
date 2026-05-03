package analyzer

const (
	PromptVersionShort     = "analyzer.short.v1"
	PromptVersionLong      = "analyzer.long.v1"
	PromptVersionThesis    = "analyzer.thesis.v1"
	PromptVersionComposite = "short.v1+long.v1+thesis.v1"
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

// TODO(thesis-profile): this prompt has no operator-supplied criterion for
// what "thesis-angle candidate" actually means, so the LLM is guessing. The
// stub currently sidesteps the gap by returning a hardcoded flag=true with a
// "default" rationale. Before the real provider ships, a follow-up spec must
// add an editable thesis-profile (e.g. data/thesis_profile.md, mirroring
// arxiv-search-defaults) and inject it into this system prompt — otherwise
// the persisted thesis_angle_flag is meaningless.
const promptThesisSystem = `You are a research assistant assessing whether the paper below is a candidate
for a DeFi research thesis angle.

Respond with a single JSON object and nothing else, with exactly these fields:
- "flag": boolean, true if the paper looks like a credible thesis-angle candidate.
- "rationale": string, one to three sentences justifying the flag.

Do not wrap the JSON in code fences. Do not emit any text outside the JSON object.`
