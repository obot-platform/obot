# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for Obot. An ADR is the concise, durable record of a meaningful architectural decision reflected in the implementation.

ADRs answer the question: "Why do we do it this way?" They are not complete implementation documentation and should not repeat the full design discussion. Significant designs are discussed before implementation as [Obot Design Proposals](https://github.com/obot-platform/obot-design-proposals).

## When to write an ADR

Include an ADR when an implementation introduces or changes a decision that is likely to constrain future work, such as a significant component boundary, interface, data model, persistence strategy, security model, or operational approach.

Small, local, and easily reversible implementation choices generally do not need an ADR.

## Creating an ADR

1. Copy [`template.md`](template.md) to `NNNN-short-name.md` using the next available four-digit number and a concise kebab-case name.
2. Describe the decision reflected in the implementation.
3. If the implementation differs materially from an accepted ODP, complete a follow-up ODP before proceeding and link both ODPs from the ADR.
4. Link every GitHub issue related to the decision and the related ODP when applicable.
5. If the ADR supersedes an earlier decision, link both records: set `Supersedes` in the new ADR, then mark the old ADR `Superseded` and set its `Superseded by` field.
6. Include the ADR in the implementation pull request so it merges with the code whose architecture it records.

Keep ADRs short. Link to relevant source or documentation for detail that does not need to be repeated.

## Statuses

- **Accepted:** The decision currently governs the architecture.
- **Superseded:** Another ADR replaces this decision. Link both ADRs.
- **Deprecated:** The decision is being retired and should not be used for new work, but no ADR directly replaces it.

Use `Superseded` when a replacement ADR exists. Otherwise, use `Deprecated`.

Do not rewrite an accepted ADR to describe a materially different decision. Create a new ADR and update the old ADR in the same change so their `Supersedes` and `Superseded by` links are bidirectional. Small corrections and additional references may be added in place.
