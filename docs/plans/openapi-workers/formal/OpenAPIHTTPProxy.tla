--------------------------- MODULE OpenAPIHTTPProxy ---------------------------
EXTENDS Naturals, Sequences, FiniteSets

\* P03 raw-request authorization state machine. The strict raw URI parser is
\* abstracted as one indivisible step: request.rawSegments are the exact escaped
\* segments after validation, before percent-decoding and before re-encoding.

CONSTANTS
  \* @type: Str;
  ComponentID,
  \* @type: Str;
  OtherComponent,
  \* @type: Bool;
  OverridePresent,
  \* @type: SetOf(Str);
  OverrideEnabledTools,
  \* @type: SetOf(Str);
  ProfileAllowedTools,
  \* @type: SetOf(Str);
  InstanceAllowedTools

Requests == {
  [component |-> ComponentID, method |-> "GET", rawSegments |-> <<"items">>, rawQuery |-> ""],
  [component |-> ComponentID, method |-> "GET", rawSegments |-> <<"items">>, rawQuery |-> "b=2&a=1&a="],
  [component |-> ComponentID, method |-> "GET", rawSegments |-> <<"items", "special">>, rawQuery |-> ""],
  [component |-> ComponentID, method |-> "GET", rawSegments |-> <<"items", "a%2Fb">>, rawQuery |-> ""],
  [component |-> ComponentID, method |-> "GET", rawSegments |-> <<"lists", "a%2Fb">>, rawQuery |-> "tag=x%2Fy"],
  [component |-> ComponentID, method |-> "POST", rawSegments |-> <<"lists", "a%2Fb">>, rawQuery |-> ""],
  [component |-> ComponentID, method |-> "DELETE", rawSegments |-> <<"items">>, rawQuery |-> ""],
  [component |-> OtherComponent, method |-> "GET", rawSegments |-> <<"items">>, rawQuery |-> ""]
}

Operations == {
  [component |-> ComponentID, method |-> "GET", template |-> <<"items">>, tool |-> "list_items"],
  [component |-> ComponentID, method |-> "GET", template |-> <<"items", "special">>, tool |-> "get_special_item"],
  [component |-> ComponentID, method |-> "GET", template |-> <<"items", "{id}">>, tool |-> "get_item"],
  [component |-> ComponentID, method |-> "GET", template |-> <<"items", "{identifier}">>, tool |-> "get_item_alias"],
  [component |-> ComponentID, method |-> "GET", template |-> <<"lists", "{id}">>, tool |-> "get_list_item"],
  [component |-> ComponentID, method |-> "POST", template |-> <<"lists", "{id}">>, tool |-> "update_list_item"],
  [component |-> OtherComponent, method |-> "GET", template |-> <<"items">>, tool |-> "other_list_items"]
}

ParameterPlaceholders == {"{id}", "{identifier}"}

VARIABLES
  phase,
  request,
  candidateOperations,
  selectedOperation,
  forwardedRawSegments,
  forwardedRawQuery,
  upstreamCalled,
  platformOutcome,
  executions

vars ==
  <<phase, request, candidateOperations, selectedOperation,
     forwardedRawSegments, forwardedRawQuery, upstreamCalled,
     platformOutcome, executions>>

IsParameter(segment) ==
  segment \in ParameterPlaceholders

TemplateMatches(incomingRequest, operation) ==
  /\ incomingRequest.component = operation.component
  /\ Len(incomingRequest.rawSegments) = Len(operation.template)
  /\ \A position \in 1..Len(operation.template) :
       \/ incomingRequest.rawSegments[position] = operation.template[position]
       \/ IsParameter(operation.template[position])

MatchingOperations(incomingRequest) ==
  {operation \in Operations :
     /\ TemplateMatches(incomingRequest, operation)
     /\ incomingRequest.method = operation.method}

SamePathOperations(incomingRequest) ==
  {operation \in Operations :
     /\ TemplateMatches(incomingRequest, operation)
     /\ incomingRequest.method # operation.method}

StaticCount(template) ==
  Cardinality({position \in 1..Len(template) :
                 ~IsParameter(template[position])})

BestStaticCount(incomingRequest) ==
  LET counts == {StaticCount(operation.template) :
                   operation \in MatchingOperations(incomingRequest)}
  IN CHOOSE highest \in counts :
       \A other \in counts : highest >= other

\* P03 conflict policy: a more-static candidate wins; ties at the best static
\* count are ambiguous and never fall through to another candidate.
ResolvedMatches(incomingRequest) ==
  LET matches == MatchingOperations(incomingRequest)
  IN IF matches = {} THEN {}
     ELSE {operation \in matches :
             StaticCount(operation.template) = BestStaticCount(incomingRequest)}

IsWhitelisted(operation) ==
  /\ operation \in Operations
  /\ (~OverridePresent \/ operation.tool \in OverrideEnabledTools)
  /\ operation.tool \in ProfileAllowedTools
  /\ operation.tool \in InstanceAllowedTools

AuthorizedOtherMethods(incomingRequest) ==
  {operation \in SamePathOperations(incomingRequest) : IsWhitelisted(operation)}

InitialRequest == CHOOSE initial \in Requests : TRUE

Init ==
  /\ phase = "Idle"
  /\ request = InitialRequest
  /\ candidateOperations = {}
  /\ selectedOperation = <<>>
  /\ forwardedRawSegments = <<>>
  /\ forwardedRawQuery = ""
  /\ upstreamCalled = FALSE
  /\ platformOutcome = ""
  /\ executions = {}

ClearForwardedRequest ==
  /\ forwardedRawSegments' = <<>>
  /\ forwardedRawQuery' = ""
  /\ upstreamCalled' = FALSE
  /\ platformOutcome' = ""

ReceiveRequest ==
  /\ phase = "Idle"
  /\ \E incoming \in Requests :
       /\ request' = incoming
       /\ phase' = "Received"
       /\ candidateOperations' = {}
       /\ selectedOperation' = <<>>
       /\ ClearForwardedRequest
       /\ executions' = executions

ResolveRoute ==
  /\ phase = "Received"
  /\ UNCHANGED request
  /\ LET resolved == ResolvedMatches(request)
     IN IF Cardinality(resolved) = 1 THEN
          /\ phase' = "RouteMatched"
          /\ candidateOperations' = resolved
          /\ selectedOperation' = CHOOSE operation \in resolved : TRUE
          /\ ClearForwardedRequest
          /\ executions' = executions
        ELSE
          /\ phase' = "Rejected"
          /\ candidateOperations' = resolved
          /\ selectedOperation' = <<>>
          /\ upstreamCalled' = FALSE
          /\ forwardedRawSegments' = <<>>
          /\ forwardedRawQuery' = ""
          /\ platformOutcome' =
               IF Cardinality(MatchingOperations(request)) = 0
                  /\ Cardinality(AuthorizedOtherMethods(request)) > 0
                 THEN "platform_405"
                 ELSE "platform_404"
          /\ executions' = executions

ResolveAuthorization ==
  /\ phase = "RouteMatched"
  /\ UNCHANGED request
  /\ IF IsWhitelisted(selectedOperation) THEN
       /\ phase' = "Authorized"
       /\ ClearForwardedRequest
     ELSE
       /\ phase' = "Rejected"
       /\ upstreamCalled' = FALSE
       /\ forwardedRawSegments' = <<>>
       /\ forwardedRawQuery' = ""
       /\ platformOutcome' = "platform_403"
  /\ candidateOperations' = candidateOperations
  /\ selectedOperation' = selectedOperation
  /\ executions' = executions

ExecutionRecord ==
  [operation |-> selectedOperation,
   component |-> request.component,
   method |-> request.method,
   rawSegments |-> request.rawSegments,
   rawQuery |-> request.rawQuery]

ExecuteAuthorized ==
  /\ phase = "Authorized"
  /\ UNCHANGED request
  /\ IsWhitelisted(selectedOperation)
  /\ phase' = "Executed"
  /\ candidateOperations' = candidateOperations
  /\ selectedOperation' = selectedOperation
  /\ forwardedRawSegments' = request.rawSegments
  /\ forwardedRawQuery' = request.rawQuery
  /\ upstreamCalled' = TRUE
  /\ platformOutcome' = "business_response"
  /\ executions' = executions \union {ExecutionRecord}

ResetRequest ==
  /\ phase \in {"Rejected", "Executed"}
  /\ phase' = "Idle"
  /\ request' = request
  /\ candidateOperations' = {}
  /\ selectedOperation' = <<>>
  /\ ClearForwardedRequest
  /\ executions' = executions

Next ==
  ReceiveRequest
  \/ ResolveRoute
  \/ ResolveAuthorization
  \/ ExecuteAuthorized
  \/ ResetRequest

Spec == Init /\ [][Next]_vars

ExecutionRequiresUniqueAuthorizedMatch ==
  phase # "Executed"
  \/ /\ upstreamCalled
     /\ Cardinality(candidateOperations) = 1
     /\ candidateOperations = {selectedOperation}
     /\ selectedOperation \in ResolvedMatches(request)
     /\ selectedOperation.component = request.component
     /\ selectedOperation.method = request.method
     /\ IsWhitelisted(selectedOperation)

RawRequestSemanticsPreservedOnExecution ==
  phase # "Executed"
  \/ /\ forwardedRawSegments = request.rawSegments
     /\ forwardedRawQuery = request.rawQuery
     /\ request.method = selectedOperation.method

RejectionNeverCallsUpstream ==
  phase # "Rejected"
  \/ /\ ~upstreamCalled
     /\ platformOutcome # "business_response"

OnlyAuthorizedExecutionsAreRetained ==
  \A execution \in executions :
    /\ execution.operation \in Operations
    /\ IsWhitelisted(execution.operation)
    /\ execution.operation.component = execution.component
    /\ execution.operation.method = execution.method
    /\ execution.rawSegments = execution.rawSegments

=============================================================================
