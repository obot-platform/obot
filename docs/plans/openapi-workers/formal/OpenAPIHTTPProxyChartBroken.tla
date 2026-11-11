------------------------- MODULE OpenAPIHTTPProxyChartBroken -------------------------
EXTENDS OpenAPIHTTPProxyChart

BypassWhitelist ==
  /\ phase = "RouteMatched"
  /\ UNCHANGED request
  /\ UNCHANGED candidateOperations
  /\ UNCHANGED selectedOperation
  /\ ~IsWhitelisted(selectedOperation)
  /\ phase' = "Executed"
  /\ forwardedRawPath' = request.rawPath
  /\ forwardedRawQuery' = request.rawQuery
  /\ upstreamCalled' = TRUE
  /\ platformOutcome' = "business_response"

FallThroughAmbiguousRoute ==
  /\ phase = "Rejected"
  /\ platformOutcome = "platform_404"
  /\ ~HasExactlyOne(candidateOperations)
  /\ \E chosen \in candidateOperations :
       /\ selectedOperation' = chosen
       /\ phase' = "Executed"
       /\ candidateOperations' = candidateOperations
       /\ forwardedRawPath' = request.rawPath
       /\ forwardedRawQuery' = request.rawQuery
       /\ upstreamCalled' = TRUE
       /\ platformOutcome' = "business_response"
  /\ UNCHANGED request

BrokenNext == Next \/ BypassWhitelist \/ FallThroughAmbiguousRoute
BrokenSpec == Init /\ [][BrokenNext]_vars

=============================================================================
