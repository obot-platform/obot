export const title = "OpenAPI Worker per-call identity and credential resolution";

const phaseLabels = {
  request_received: "Request received",
  identity_resolved: "Caller authenticated",
  instance_selected: "Instance selected",
  snapshot_resolution: "Published snapshot loaded",
  operation_authorization: "Operation checked",
  credential_resolution: "Credential context checked",
  credential_ready: "Caller credential ready",
  business_authorization_required: "Authorization required",
  instance_rejected: "Instance denied",
  tool_rejected: "Tool denied",
  dispatched: "Sent to worker",
};

export const state = (variables) => phaseLabels[variables.phase] ?? variables.phase;

export const group = (variables) => {
  if (variables.phase === "request_received" || variables.phase === "identity_resolved") {
    return "Request";
  }
  if (variables.phase === "instance_rejected" || variables.phase === "tool_rejected") {
    return "Denied";
  }
  if (variables.phase === "business_authorization_required") {
    return "Credential missing";
  }
  if (variables.phase === "dispatched") {
    return "Dispatched";
  }
  return "Authorizing";
};

export const colors = {
  Request: "blue",
  Authorizing: "yellow",
  "Credential missing": "orange",
  Denied: "purple",
  Dispatched: "teal",
};

export const step = (action, fromState, toState) => {
  if (action === "AuthenticateCaller") return "the platform identifies the caller";
  if (action === "SelectInstance") return "the request names an instance";
  if (action === "AuthorizeInstanceAndProfiles") {
    return toState.phase === "instance_rejected"
      ? "instance ownership or profile fails"
      : "instance ownership and profile pass";
  }
  if (action === "ResolveSnapshotAndOperation") {
    return "the published snapshot and original tool are loaded";
  }
  if (action === "AuthorizeOperation") {
    return toState.phase === "tool_rejected"
      ? "tool authorization fails"
      : "tool authorization passes";
  }
  if (action === "ResolveCredential") {
    return toState.phase === "credential_ready"
      ? "a valid credential is found for the exact instance"
      : "the exact instance credential is missing or invalid";
  }
  if (action === "DispatchToWorker") return "the request is sent to the worker";
  return null;
};
