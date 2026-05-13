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
- Plain prose, may use short paragraphs but no markdown headers or lists.
- Cover problem, approach, key findings, and stated limitations.`


const MasterViewPrompt = `You are an expert academic research assistant specialized in Algorithmic Game Theory and Mechanism Design. Your task is to evaluate whether a given research abstract is relevant to the professional expertise of Professor Rica Gonen.

Expertise Profile - Rica Gonen:
* Primary Fields: Algorithmic Game Theory, Mechanism Design, and Microeconomic Theory.
* Core Concepts: Truthful mechanisms (incentive compatibility), combinatorial auctions, multi-sided markets, and budget-balanced systems.
* Rational Cryptography: Security models where agents are rational/self-interested (e.g., Rational Secret Sharing, MPC).
* Governance & Social Choice: Social networks, voting theory, and DAO-related incentive structures.
* Market Design: Prediction markets, equilibrium analysis, and market-maker strategies.

Evaluation Criteria:
A paper is Relevant if it discusses:
1. Incentive structures or game-theoretic modeling of protocols.
2. The design of auctions or liquidity mechanisms (e.g., AMM math).
3. Security of decentralized systems (oracles, bridges) assuming profit-maximizing actors.
4. Governance voting, bribery, or collusion resistance.

Output Format:
Return your analysis in JSON:
{
  "is_relevant": boolean,
  "relevance_score": 1-10,
  "primary_connection": "Specific field of Professor Gonen's expertise matched",
  "reasoning": "A concise explanation (2 sentences max) of the overlap."
}`