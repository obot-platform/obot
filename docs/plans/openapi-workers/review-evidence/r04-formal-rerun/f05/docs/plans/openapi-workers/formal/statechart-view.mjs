export const title = "OpenAPI Compose deployment lifecycle";

export const state = (vars) => {
  const anyObotReady = vars.obot?.[1] === "ready" || vars.obot?.[2] === "ready";
  const anyWorkerReady =
    vars.worker?.[1] === "ready" || vars.worker?.[2] === "ready";
  const trafficCapacity =
    vars.sharedNetwork === true &&
    vars.database === "ready" &&
    vars.serviceAuth === "trusted" &&
    anyObotReady &&
    anyWorkerReady;

  if (vars.activeCalls > 0) {
    return trafficCapacity
      ? "OpenAPI traffic admitted"
      : "Unsafe traffic admitted";
  }
  if (vars.rollout !== "none") {
    return trafficCapacity
      ? "Rolling upgrade, capacity retained"
      : "Rolling upgrade, capacity lost";
  }
  if (vars.infrastructureOutage) {
    return "Recovering after extra failure";
  }
  if (!vars.sharedNetwork) {
    return "Provisioning shared network";
  }
  if (vars.database !== "ready") {
    return "Provisioning Obot dependencies";
  }
  if (vars.serviceAuth !== "trusted") {
    return "Provisioning service authentication";
  }
  if (!anyObotReady) {
    return "Provisioning Obot";
  }
  if (!anyWorkerReady) {
    return "Provisioning Workers";
  }
  return "Both stacks ready, traffic closed";
};

export const group = (vars) => {
  const label = state(vars);
  if (label === "Unsafe traffic admitted" || label === "Rolling upgrade, capacity lost") {
    return "Broken";
  }
  if (label === "OpenAPI traffic admitted") {
    return "Serving";
  }
  if (label === "Rolling upgrade, capacity retained") {
    return "Upgrade";
  }
  if (label === "Recovering after extra failure") {
    return "Recovery";
  }
  return "Provisioning";
};

export const colors = {
  Broken: "pink",
  Serving: "teal",
  Upgrade: "purple",
  Recovery: "orange",
  Provisioning: "blue",
};

export const step = (action, from, to) => {
  if (action === "CreateSharedNetwork") {
    return "operator creates the explicit shared network";
  }
  if (action === "DestroySharedNetwork") {
    return "the shared network disappears";
  }
  if (action === "PrepareDatabase") {
    return "PostgreSQL starts";
  }
  if (action === "DatabaseBecomesReady") {
    return "PostgreSQL passes readiness";
  }
  if (action === "DatabaseFails") {
    return "PostgreSQL fails";
  }
  if (action === "ConfigureServiceAuth") {
    return "service certificates are staged";
  }
  if (action === "TrustServiceAuth") {
    return "the service CA becomes trusted";
  }
  if (action === "WithdrawServiceAuth") {
    return "service authentication is withdrawn";
  }
  if (action.startsWith("StartObot")) {
    return "an Obot replica starts";
  }
  if (action.startsWith("ObotBecomesReady")) {
    return "the Obot replica passes readiness";
  }
  if (action.startsWith("DrainObot")) {
    return "the Obot replica drains while another serves";
  }
  if (action.startsWith("ReplaceObot")) {
    return "the drained Obot replica is replaced";
  }
  if (action.startsWith("ObotFails")) {
    return "an Obot replica fails";
  }
  if (action.startsWith("StartWorker")) {
    return "a Worker starts on the trusted shared network";
  }
  if (action.startsWith("WorkerBecomesReady")) {
    return "the Worker passes readiness";
  }
  if (action.startsWith("DrainWorker")) {
    return "the Worker drains while another serves";
  }
  if (action.startsWith("ReplaceWorker")) {
    return "the drained Worker is replaced";
  }
  if (action.startsWith("WorkerFails")) {
    return "a Worker fails";
  }
  if (action.startsWith("BeginObotRoll")) {
    return "a one-at-a-time Obot upgrade starts";
  }
  if (action.startsWith("BeginWorkerRoll")) {
    return "a one-at-a-time Worker upgrade starts";
  }
  if (action === "FinishRoll") {
    return "the upgraded replica is healthy again";
  }
  if (action === "AdmitOpenAPICall") {
    return from === to
      ? null
      : "an OpenAPI request is admitted after every gate holds";
  }
  if (action === "CompleteOpenAPICall") {
    return from === to ? null : "the OpenAPI request completes";
  }
  if (action === "ClearOutageAfterRecovery") {
    return "capacity returns after the extra failure";
  }
  return null;
};
