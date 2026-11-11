------------------------- MODULE OpenAPIHTTPProxyBroken -------------------------
EXTENDS OpenAPIHTTPProxy

\* Deliberate failure variant: a route-matched operation can skip the
\* ToolOverride/Profile/instance whitelist and still be marked as executed.
BypassWhitelist ==
  /\ phase = "RouteMatched"
  /\ UNCHANGED request
  /\ ~IsWhitelisted(selectedOperation)
  /\ phase' = "Executed"
  /\ candidateOperations' = candidateOperations
  /\ selectedOperation' = selectedOperation
  /\ forwardedRawSegments' = request.rawSegments
  /\ forwardedRawQuery' = request.rawQuery
  /\ upstreamCalled' = TRUE
  /\ platformOutcome' = "business_response"
  /\ executions' = executions \union {ExecutionRecord}

BrokenNext == Next \/ BypassWhitelist
BrokenSpec == Init /\ [][BrokenNext]_vars

=============================================================================
