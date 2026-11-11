export const title = "P03 API proxy request authorization";

export const state = (values) => {
  if (values.phase === "Idle") return "Ready for next request";
  if (values.phase === "Received") return "Raw path and query parsed";
  if (values.phase === "RouteMatched") return "One operation uniquely matched";
  if (values.phase === "Authorized") return "Operation whitelist passed";
  if (values.phase === "Executed") return "Business request executed";
  if (values.platformOutcome === "platform_405") return "Rejected: method not allowed";
  if (values.platformOutcome === "platform_404") return "Rejected: no unique route";
  if (values.platformOutcome === "platform_403") return "Rejected: operation disabled";
  return "Rejected";
};

export const group = (values) => {
  if (values.phase === "Executed") return "business";
  if (values.phase === "Rejected") return "platform rejection";
  return "platform processing";
};

export const colors = {
  "platform processing": "blue",
  "platform rejection": "orange",
  business: "purple",
};

export const step = (action, from, to) => {
  if (action === "ReceiveRequest") return "a caller sends the proxy request";
  if (action === "ResolveRoute") {
    if (to.phase === "RouteMatched") return "one raw template wins [when static precedence makes it unique]";
    if (to.platformOutcome === "platform_405") return "the proxy reports method not allowed [when another authorized method exists]";
    return "the proxy rejects the request [when no route is unique]";
  }
  if (action === "ResolveAuthorization") {
    if (to.phase === "Authorized") return "overrides, profile, and instance all allow the operation";
    return "the proxy rejects the operation [when any whitelist layer disables it]";
  }
  if (action === "ExecuteAuthorized") return "the proxy executes with raw path and query preserved";
  if (action === "BypassWhitelist") return "the broken variant skips the whitelist";
  if (action === "FallThroughAmbiguousRoute") return "the broken variant executes one of the ambiguous routes";
  if (action === "ResetRequest") return "the proxy records the result and takes the next request";
  return null;
};
