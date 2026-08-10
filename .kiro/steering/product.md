# product.md

Personal web tool that aggregates DeFi news and academic sources, summarises new content with an LLM, and presents a chronological research feed. Goal: help identify patterns, gaps, and potential thesis topics.

## Ingestion (v1)

- RSS/Atom feeds — concrete implementation.
- Generic API fetcher — port defined, no concrete impl yet.
- Email — not in scope.

## Pipeline

fetch → dedupe → triage (news | governance | paper) → extract body (HTML / PDF) → LLM summarise + thesis-angle flag → persist.

## UI surface (consumed by frontend, deferred)

- Chronological feed with filters by content type, source, topic, thesis-angle.
- Save/bookmark articles.

## Auth

Email + password login (`POST /auth/login`); 24h JWT in `Authorization: Bearer`; seeded via `cmd/seed users <email> <password>`. No refresh tokens, no logout endpoint — re-login on expiry. No roles; any seeded user has equal access.

## Deferred

- Email ingestion, periodic scheduling, API fetcher impls, frontend, observability backend, Postgres.
