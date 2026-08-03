# Hosted agent instance reconciliation

`HostedAgentInstance` resources are reconciled through `pkg/agentbackend`.
The controller owns Obot-specific resolution and passes only normalized,
runtime-ready configuration to the selected backend.

The controller:

- validates an explicitly selected pool against the user's assignments;
- selects the user's default assignment, or lazily creates a deterministic
  pool and default assignment from the deployment-wide `default`
  `HostedAgentPoolDefaults`;
- uses the instance UID as the stable backend identity;
- installs a finalizer before creating backend resources;
- calculates an Obot-owned revision from desired runtime configuration;
- reports an instance ready only when the backend has applied that revision;
- retains the first healthy runtime URL across later state changes;
- polls transitional instances every ten seconds and ready instances every
  five minutes, so backend events are an optimization rather than a
  correctness requirement; and
- removes its finalizer only after the backend confirms deletion is complete.

The default builder currently renders the versioned
`/etc/obot/agent.json`, non-sensitive environment, harness image, and source
repository. Sensitive values are deliberately excluded. Resolution of MCP
gateway endpoints, model proxy endpoints, skill file trees, and backend secret
references belongs in the builder and can be added without changing the
backend interface or lifecycle controller.
