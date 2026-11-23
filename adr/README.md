# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for Obot. An ADR is the concise, durable record of a meaningful architectural decision.

ADRs answer the question: "Why do we do it this way?" They are not complete implementation documentation and should not repeat the full design discussion.

ADRs are also how significant designs are proposed and reviewed before implementation. A design that needs agreement before work starts is opened as a pull request that adds a `Proposed` ADR, and merging that pull request is the signal that implementation may proceed.

## When to write an ADR

Write an ADR when a change introduces or changes a decision that is likely to constrain future work, such as a significant component boundary, interface, data model, persistence strategy, security model, or operational approach.

Small, local, and easily reversible implementation choices generally do not need an ADR.

## Choosing a review path

### Design-first ADRs

Use this path for significant or architecturally important changes: work that spans components or teams, changes a public API or persisted data model, affects the security model, or would be expensive to undo once built.

1. Open a pull request that adds only the ADR, with its status set to `Proposed`. Include enough context, alternatives, and consequences for reviewers to evaluate the design.
2. Discuss and revise the design in that pull request. Merge it once the reviewers agree on the design. The merged `Proposed` ADR is the signal that implementation may proceed.
3. In the implementation pull request, set the status to `Accepted` and update the ADR to reflect what shipped. Corrections and details learned during implementation can be made in place.
4. If the implementation must differ materially from the merged proposal, stop and revise the `Proposed` ADR in its own pull request for review before continuing. Do not document the change only in the implementation pull request.

If a design is abandoned after its proposal merges, set its status to `Withdrawn` and note why.

### Implementation-time ADRs

Use this path for smaller decisions that are worth recording but do not need agreement before work starts. The developer and their agent agree on the decision while implementing it, and the ADR is reviewed as part of the implementation pull request. It is added with status `Accepted` and merges with the code whose architecture it records.

When it is unclear which path applies, prefer the design-first path.

## Creating an ADR

1. Copy [`template.md`](template.md) to `YYYY-MM-DD-short-name.md` using the creation date and a concise kebab-case name. Keep the file name when the status changes later.
2. Describe the decision. A `Proposed` ADR describes the intended design; an `Accepted` ADR describes the decision reflected in the implementation.
3. Link every GitHub issue related to the decision.
4. If the ADR supersedes an earlier decision, link both records: set `Supersedes` in the new ADR, then mark the old ADR `Superseded` and set its `Superseded by` field when the new ADR becomes `Accepted`.

Keep ADRs short. Link to relevant source or documentation for detail that does not need to be repeated.

## Statuses

- **Proposed:** The design has been reviewed and approved for implementation but has not shipped.
- **Accepted:** The decision currently governs the architecture.
- **Superseded:** Another ADR replaces this decision. Link both ADRs.
- **Deprecated:** The decision is being retired and should not be used for new work, but no ADR directly replaces it.
- **Withdrawn:** A `Proposed` design was abandoned before it shipped.

Use `Superseded` when a replacement ADR exists. Otherwise, use `Deprecated`.

Do not rewrite an accepted ADR to describe a materially different decision. Create a new ADR and update the old ADR in the same change so their `Supersedes` and `Superseded by` links are bidirectional. Small corrections and additional references may be added in place.

## Obot Design Proposals

Significant designs were previously proposed in the separate [Obot Design Proposals](https://github.com/obot-platform/obot-design-proposals) (ODP) repository. That process is retired: do not open new ODPs. Older ADRs keep their `Related ODPs` sections as historical references.
