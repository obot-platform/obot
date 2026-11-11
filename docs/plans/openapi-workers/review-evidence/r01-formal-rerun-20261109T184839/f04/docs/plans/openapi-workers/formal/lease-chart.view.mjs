export const title = "OpenAPI execution-budget lease lifecycle";

export const state = (vars) => {
  switch (vars.phase) {
    case "waiting":
      return "Bounded queue ticket waiting";
    case "activeNotSent":
      return "Lease held, upstream not sent";
    case "activeSent":
      return "Lease held, upstream already sent";
    case "expiredPending":
      return "Expired, quota still held";
    case "completed":
      return "Confirmed result released";
    case "expired":
      return "Expired quota reclaimed";
    default:
      return "No request yet";
  }
};

export const group = (vars) => {
  const label = state(vars);
  if (label === "Confirmed result released") return "Confirmed release";
  if (label === "Expired quota reclaimed") return "Recovery";
  if (label === "Expired, quota still held") return "Conservative hold";
  if (label.startsWith("Lease held")) return "Execution";
  if (label === "Bounded queue ticket waiting") return "Admission";
  return "Idle";
};

export const colors = {
  Idle: "blue",
  Admission: "purple",
  Execution: "teal",
  "Conservative hold": "orange",
  Recovery: "yellow",
  "Confirmed release": "teal",
};

export const step = (action, from, to) => {
  if (action === "Tick") return "time advances and a due lease expires";
  if (action === "Enqueue") return "a replica takes a bounded queue ticket";
  if (action === "Admit") return "the shared quota admits one request";
  if (action === "WorkerStartsUpstream") {
    return from === to
      ? null
      : "a Worker starts the only upstream attempt before the fence";
  }
  if (action === "ReleaseConfirmed") {
    return from === to
      ? null
      : "an observed completion releases quota inside the lease";
  }
  if (action === "ReclaimExpired") {
    return "the reclaimer releases quota after the grace period";
  }
  return null;
};
