--------------------------- MODULE OpenAPIHTTPProxyChart ---------------------------
\* Chart-only projection of OpenAPIHTTPProxy. It keeps the same decision
\* sequence and whitelist data, but replaces sequence-based raw-segment
\* matching with finite route shapes already produced by the strict raw parser.

CONSTANTS
  \* @type: Str;
  ComponentID,
  \* @type: SetOf(Str);
  OverrideEnabledTools,
  \* @type: SetOf(Str);
  ProfileAllowedTools,
  \* @type: SetOf(Str);
  InstanceAllowedTools

Requests == {
  [component |-> ComponentID, method |-> "GET", routeShape |-> "items.static",
   rawPath |-> "/items", rawQuery |-> "b=2&a=1&a="],
  [component |-> ComponentID, method |-> "GET", routeShape |-> "items.special",
   rawPath |-> "/items/special", rawQuery |-> ""],
  [component |-> ComponentID, method |-> "GET", routeShape |-> "items.parameter",
   rawPath |-> "/items/a%2Fb", rawQuery |-> ""],
  [component |-> ComponentID, method |-> "GET", routeShape |-> "lists.parameter",
   rawPath |-> "/lists/a%2Fb", rawQuery |-> "tag=x%2Fy"]
}

Operations == {
  [component |-> ComponentID, method |-> "GET", routeShape |-> "items.static",
   tool |-> "list_items"],
  [component |-> ComponentID, method |-> "GET", routeShape |-> "items.special",
   tool |-> "get_special_item"],
  [component |-> ComponentID, method |-> "GET", routeShape |-> "items.parameter",
   tool |-> "get_item"],
  [component |-> ComponentID, method |-> "GET", routeShape |-> "items.parameter",
   tool |-> "get_item_alias"],
  [component |-> ComponentID, method |-> "GET", routeShape |-> "lists.parameter",
   tool |-> "get_list_item"]
}

IsWhitelisted(operation) ==
  /\ operation \in Operations
  /\ operation.tool \in OverrideEnabledTools
  /\ operation.tool \in ProfileAllowedTools
  /\ operation.tool \in InstanceAllowedTools

MatchingOperations(incomingRequest) ==
  {operation \in Operations :
     /\ operation.component = incomingRequest.component
     /\ operation.method = incomingRequest.method
     /\ operation.routeShape = incomingRequest.routeShape}

HasExactlyOne(set) ==
  \E element \in set : set = {element}

VARIABLES
  phase,
  request,
  candidateOperations,
  selectedOperation,
  forwardedRawPath,
  forwardedRawQuery,
  upstreamCalled,
  platformOutcome

vars ==
  <<phase, request, candidateOperations, selectedOperation,
     forwardedRawPath, forwardedRawQuery, upstreamCalled, platformOutcome>>

InitialRequest == CHOOSE initial \in Requests : TRUE

Init ==
  /\ phase = "Idle"
  /\ request = InitialRequest
  /\ candidateOperations = {}
  /\ selectedOperation = <<>>
  /\ forwardedRawPath = ""
  /\ forwardedRawQuery = ""
  /\ upstreamCalled = FALSE
  /\ platformOutcome = ""

ReceiveRequest ==
  /\ phase = "Idle"
  /\ \E incoming \in Requests :
       /\ request' = incoming
       /\ phase' = "Received"
       /\ candidateOperations' = {}
       /\ selectedOperation' = <<>>
       /\ forwardedRawPath' = ""
       /\ forwardedRawQuery' = ""
       /\ upstreamCalled' = FALSE
       /\ platformOutcome' = ""

ResolveRoute ==
  /\ phase = "Received"
  /\ UNCHANGED request
  /\ LET resolved == MatchingOperations(request)
     IN IF HasExactlyOne(resolved) THEN
          /\ phase' = "RouteMatched"
          /\ candidateOperations' = resolved
          /\ selectedOperation' = CHOOSE operation \in resolved : TRUE
          /\ forwardedRawPath' = ""
          /\ forwardedRawQuery' = ""
          /\ upstreamCalled' = FALSE
          /\ platformOutcome' = ""
        ELSE
          /\ phase' = "Rejected"
          /\ candidateOperations' = resolved
          /\ selectedOperation' = <<>>
          /\ forwardedRawPath' = ""
          /\ forwardedRawQuery' = ""
          /\ upstreamCalled' = FALSE
          /\ platformOutcome' = "platform_404"

ResolveAuthorization ==
  /\ phase = "RouteMatched"
  /\ UNCHANGED request
  /\ UNCHANGED candidateOperations
  /\ UNCHANGED selectedOperation
  /\ IF IsWhitelisted(selectedOperation) THEN
       /\ phase' = "Authorized"
       /\ forwardedRawPath' = ""
       /\ forwardedRawQuery' = ""
       /\ upstreamCalled' = FALSE
       /\ platformOutcome' = ""
     ELSE
       /\ phase' = "Rejected"
       /\ forwardedRawPath' = ""
       /\ forwardedRawQuery' = ""
       /\ upstreamCalled' = FALSE
       /\ platformOutcome' = "platform_403"

ExecuteAuthorized ==
  /\ phase = "Authorized"
  /\ UNCHANGED request
  /\ UNCHANGED candidateOperations
  /\ UNCHANGED selectedOperation
  /\ IsWhitelisted(selectedOperation)
  /\ phase' = "Executed"
  /\ forwardedRawPath' = request.rawPath
  /\ forwardedRawQuery' = request.rawQuery
  /\ upstreamCalled' = TRUE
  /\ platformOutcome' = "business_response"

ResetRequest ==
  /\ phase \in {"Rejected", "Executed"}
  /\ phase' = "Idle"
  /\ UNCHANGED request
  /\ candidateOperations' = {}
  /\ selectedOperation' = <<>>
  /\ forwardedRawPath' = ""
  /\ forwardedRawQuery' = ""
  /\ upstreamCalled' = FALSE
  /\ platformOutcome' = ""

Next ==
  ReceiveRequest
  \/ ResolveRoute
  \/ ResolveAuthorization
  \/ ExecuteAuthorized
  \/ ResetRequest

Spec == Init /\ [][Next]_vars

ExecutionRequiresUniqueRoute ==
  phase # "Executed"
  \/ /\ upstreamCalled
     /\ candidateOperations = {selectedOperation}
     /\ selectedOperation \in MatchingOperations(request)

ExecutionRequiresWhitelist ==
  phase # "Executed"
  \/ /\ upstreamCalled
     /\ IsWhitelisted(selectedOperation)

RawRequestSemanticsPreservedOnExecution ==
  phase # "Executed"
  \/ /\ forwardedRawPath = request.rawPath
     /\ forwardedRawQuery = request.rawQuery

RejectionNeverCallsUpstream ==
  phase # "Rejected"
  \/ /\ ~upstreamCalled
     /\ platformOutcome # "business_response"

=============================================================================
