---- MODULE OpenAPIHTTPProxyBroken_TTrace_1794222783 ----
EXTENDS Sequences, TLCExt, Toolbox, Naturals, TLC, OpenAPIHTTPProxyBroken

_expression ==
    LET OpenAPIHTTPProxyBroken_TEExpression == INSTANCE OpenAPIHTTPProxyBroken_TEExpression
    IN OpenAPIHTTPProxyBroken_TEExpression!expression
----

_trace ==
    LET OpenAPIHTTPProxyBroken_TETrace == INSTANCE OpenAPIHTTPProxyBroken_TETrace
    IN OpenAPIHTTPProxyBroken_TETrace!trace
----

_inv ==
    ~(
        TLCGet("level") = Len(_TETrace)
        /\
        phase = ("Executed")
        /\
        request = ([rawSegments |-> <<"items">>, rawQuery |-> "", component |-> "other_component", method |-> "GET"])
        /\
        selectedOperation = ([component |-> "other_component", method |-> "GET", template |-> <<"items">>, tool |-> "other_list_items"])
        /\
        upstreamCalled = (TRUE)
        /\
        executions = ({[rawSegments |-> <<"items">>, rawQuery |-> "", component |-> "other_component", method |-> "GET", operation |-> [component |-> "other_component", method |-> "GET", template |-> <<"items">>, tool |-> "other_list_items"]]})
        /\
        forwardedRawQuery = ("")
        /\
        platformOutcome = ("business_response")
        /\
        candidateOperations = ({[component |-> "other_component", method |-> "GET", template |-> <<"items">>, tool |-> "other_list_items"]})
        /\
        forwardedRawSegments = (<<"items">>)
    )
----

_init ==
    /\ selectedOperation = _TETrace[1].selectedOperation
    /\ forwardedRawQuery = _TETrace[1].forwardedRawQuery
    /\ request = _TETrace[1].request
    /\ forwardedRawSegments = _TETrace[1].forwardedRawSegments
    /\ phase = _TETrace[1].phase
    /\ platformOutcome = _TETrace[1].platformOutcome
    /\ executions = _TETrace[1].executions
    /\ candidateOperations = _TETrace[1].candidateOperations
    /\ upstreamCalled = _TETrace[1].upstreamCalled
----

_next ==
    /\ \E i,j \in DOMAIN _TETrace:
        /\ \/ /\ j = i + 1
              /\ i = TLCGet("level")
        /\ selectedOperation  = _TETrace[i].selectedOperation
        /\ selectedOperation' = _TETrace[j].selectedOperation
        /\ forwardedRawQuery  = _TETrace[i].forwardedRawQuery
        /\ forwardedRawQuery' = _TETrace[j].forwardedRawQuery
        /\ request  = _TETrace[i].request
        /\ request' = _TETrace[j].request
        /\ forwardedRawSegments  = _TETrace[i].forwardedRawSegments
        /\ forwardedRawSegments' = _TETrace[j].forwardedRawSegments
        /\ phase  = _TETrace[i].phase
        /\ phase' = _TETrace[j].phase
        /\ platformOutcome  = _TETrace[i].platformOutcome
        /\ platformOutcome' = _TETrace[j].platformOutcome
        /\ executions  = _TETrace[i].executions
        /\ executions' = _TETrace[j].executions
        /\ candidateOperations  = _TETrace[i].candidateOperations
        /\ candidateOperations' = _TETrace[j].candidateOperations
        /\ upstreamCalled  = _TETrace[i].upstreamCalled
        /\ upstreamCalled' = _TETrace[j].upstreamCalled

\* Uncomment the ASSUME below to write the states of the error trace
\* to the given file in Json format. Note that you can pass any tuple
\* to `JsonSerialize`. For example, a sub-sequence of _TETrace.
    \* ASSUME
    \*     LET J == INSTANCE Json
    \*         IN J!JsonSerialize("OpenAPIHTTPProxyBroken_TTrace_1794222783.json", _TETrace)

=============================================================================

 Note that you can extract this module `OpenAPIHTTPProxyBroken_TEExpression`
  to a dedicated file to reuse `expression` (the module in the 
  dedicated `OpenAPIHTTPProxyBroken_TEExpression.tla` file takes precedence 
  over the module `OpenAPIHTTPProxyBroken_TEExpression` below).

---- MODULE OpenAPIHTTPProxyBroken_TEExpression ----
EXTENDS Sequences, TLCExt, Toolbox, Naturals, TLC, OpenAPIHTTPProxyBroken

expression == 
    [
        \* To hide variables of the `OpenAPIHTTPProxyBroken` spec from the error trace,
        \* remove the variables below.  The trace will be written in the order
        \* of the fields of this record.
        selectedOperation |-> selectedOperation
        ,forwardedRawQuery |-> forwardedRawQuery
        ,request |-> request
        ,forwardedRawSegments |-> forwardedRawSegments
        ,phase |-> phase
        ,platformOutcome |-> platformOutcome
        ,executions |-> executions
        ,candidateOperations |-> candidateOperations
        ,upstreamCalled |-> upstreamCalled
        
        \* Put additional constant-, state-, and action-level expressions here:
        \* ,_stateNumber |-> _TEPosition
        \* ,_selectedOperationUnchanged |-> selectedOperation = selectedOperation'
        
        \* Format the `selectedOperation` variable as Json value.
        \* ,_selectedOperationJson |->
        \*     LET J == INSTANCE Json
        \*     IN J!ToJson(selectedOperation)
        
        \* Lastly, you may build expressions over arbitrary sets of states by
        \* leveraging the _TETrace operator.  For example, this is how to
        \* count the number of times a spec variable changed up to the current
        \* state in the trace.
        \* ,_selectedOperationModCount |->
        \*     LET F[s \in DOMAIN _TETrace] ==
        \*         IF s = 1 THEN 0
        \*         ELSE IF _TETrace[s].selectedOperation # _TETrace[s-1].selectedOperation
        \*             THEN 1 + F[s-1] ELSE F[s-1]
        \*     IN F[_TEPosition - 1]
    ]

=============================================================================



Parsing and semantic processing can take forever if the trace below is long.
 In this case, it is advised to uncomment the module below to deserialize the
 trace from a generated binary file.

\*
\*---- MODULE OpenAPIHTTPProxyBroken_TETrace ----
\*EXTENDS IOUtils, TLC, OpenAPIHTTPProxyBroken
\*
\*trace == IODeserialize("OpenAPIHTTPProxyBroken_TTrace_1794222783.bin", TRUE)
\*
\*=============================================================================
\*

---- MODULE OpenAPIHTTPProxyBroken_TETrace ----
EXTENDS TLC, OpenAPIHTTPProxyBroken

trace == 
    <<
    ([phase |-> "Idle",request |-> [rawSegments |-> <<"items">>, rawQuery |-> "", component |-> "search_component", method |-> "GET"],selectedOperation |-> <<>>,upstreamCalled |-> FALSE,executions |-> {},forwardedRawQuery |-> "",platformOutcome |-> "",candidateOperations |-> {},forwardedRawSegments |-> <<>>]),
    ([phase |-> "Received",request |-> [rawSegments |-> <<"items">>, rawQuery |-> "", component |-> "other_component", method |-> "GET"],selectedOperation |-> <<>>,upstreamCalled |-> FALSE,executions |-> {},forwardedRawQuery |-> "",platformOutcome |-> "",candidateOperations |-> {},forwardedRawSegments |-> <<>>]),
    ([phase |-> "RouteMatched",request |-> [rawSegments |-> <<"items">>, rawQuery |-> "", component |-> "other_component", method |-> "GET"],selectedOperation |-> [component |-> "other_component", method |-> "GET", template |-> <<"items">>, tool |-> "other_list_items"],upstreamCalled |-> FALSE,executions |-> {},forwardedRawQuery |-> "",platformOutcome |-> "",candidateOperations |-> {[component |-> "other_component", method |-> "GET", template |-> <<"items">>, tool |-> "other_list_items"]},forwardedRawSegments |-> <<>>]),
    ([phase |-> "Executed",request |-> [rawSegments |-> <<"items">>, rawQuery |-> "", component |-> "other_component", method |-> "GET"],selectedOperation |-> [component |-> "other_component", method |-> "GET", template |-> <<"items">>, tool |-> "other_list_items"],upstreamCalled |-> TRUE,executions |-> {[rawSegments |-> <<"items">>, rawQuery |-> "", component |-> "other_component", method |-> "GET", operation |-> [component |-> "other_component", method |-> "GET", template |-> <<"items">>, tool |-> "other_list_items"]]},forwardedRawQuery |-> "",platformOutcome |-> "business_response",candidateOperations |-> {[component |-> "other_component", method |-> "GET", template |-> <<"items">>, tool |-> "other_list_items"]},forwardedRawSegments |-> <<"items">>])
    >>
----


=============================================================================

---- CONFIG OpenAPIHTTPProxyBroken_TTrace_1794222783 ----
CONSTANTS
    ComponentID = "search_component"
    OtherComponent = "other_component"
    OverridePresent = TRUE
    OverrideEnabledTools = { "list_items" , "get_list_item" , "update_list_item" }
    ProfileAllowedTools = { "list_items" , "get_special_item" , "get_list_item" , "update_list_item" }
    InstanceAllowedTools = { "list_items" , "get_list_item" }

INVARIANT
    _inv

CHECK_DEADLOCK
    \* CHECK_DEADLOCK off because of PROPERTY or INVARIANT above.
    FALSE

INIT
    _init

NEXT
    _next

CONSTANT
    _TETrace <- _trace

ALIAS
    _expression
=============================================================================
\* Generated on Mon Nov 09 19:13:03 CST 2026