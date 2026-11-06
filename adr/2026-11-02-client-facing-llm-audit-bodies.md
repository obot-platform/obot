# 2026-11-02: Client-facing LLM audit bodies

- **Status:** Accepted
- **Date:** 2026-11-02
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None. This refines audit capture within the existing embedded gateway architecture.

## Context

Obot reconstructed streamed audit responses with its own dialect-specific SSE parsers. Bifrost already provides response accumulation, but its provider-side logging hooks run before held guardrail output is replayed. Those snapshots can differ from client-facing output.

## Decision

Retain the original incoming request JSON. Feed client-bound core stream chunks, after guardrail replay, into Bifrost's framework accumulator, isolated from provider logging. Serialize the snapshot in the client's API dialect, removing engine fields. Capture non-streaming JSON after proxy transformations. Persistence receives completed bodies rather than raw SSE streams.

## Rationale

Using Bifrost's assembler removes duplicate protocol reconstruction while observing after guardrail replay excludes withheld output. The core handler observes chunks after successful encoding using request context callbacks. The engine owns the accumulator and stops it on shutdown.

The MCP tester uses the normal LLM gateway when model providers are configured. When no provider is configured and the external model service is enabled, the tester calls that service directly so installations can test MCP servers without configuring a model provider. This path intentionally bypasses the local Bifrost engine; its audit capture uses the complete response supplied by the terminal Responses event. Moving this path through Bifrost is not planned as part of this cutover.

## Consequences

Audit capture remains bounded at 5 MiB; oversized bodies are omitted rather than persisted as truncated JSON. Interrupted streams may have partial bodies. Historical encrypted fields remain readable. Bifrost logging stays disabled, and guardrail evaluation buffers remain independent of audit capture.

## References

- [Core gateway ADR](2026-11-02-core-llm-gateway.md)
