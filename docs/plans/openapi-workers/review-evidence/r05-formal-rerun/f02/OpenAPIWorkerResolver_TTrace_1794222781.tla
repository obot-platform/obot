---- MODULE OpenAPIWorkerResolver_TTrace_1794222781 ----
EXTENDS Sequences, TLCExt, Toolbox, Naturals, TLC, OpenAPIWorkerResolver

_expression ==
    LET OpenAPIWorkerResolver_TEExpression == INSTANCE OpenAPIWorkerResolver_TEExpression
    IN OpenAPIWorkerResolver_TEExpression!expression
----

_trace ==
    LET OpenAPIWorkerResolver_TETrace == INSTANCE OpenAPIWorkerResolver_TETrace
    IN OpenAPIWorkerResolver_TETrace!trace
----

_inv ==
    ~(
        TLCGet("level") = Len(_TETrace)
        /\
        phase = ("dispatched")
        /\
        caller = ("u2")
        /\
        component = ("component-1")
        /\
        credential = ([owner |-> "u1", token |-> "synthetic-u1-token", valid |-> TRUE])
        /\
        credentialSource = ("i1")
        /\
        operation = ("tool-1")
        /\
        requestedInstance = ("i2")
    )
----

_init ==
    /\ operation = _TETrace[1].operation
    /\ requestedInstance = _TETrace[1].requestedInstance
    /\ caller = _TETrace[1].caller
    /\ credential = _TETrace[1].credential
    /\ component = _TETrace[1].component
    /\ phase = _TETrace[1].phase
    /\ credentialSource = _TETrace[1].credentialSource
----

_next ==
    /\ \E i,j \in DOMAIN _TETrace:
        /\ \/ /\ j = i + 1
              /\ i = TLCGet("level")
        /\ operation  = _TETrace[i].operation
        /\ operation' = _TETrace[j].operation
        /\ requestedInstance  = _TETrace[i].requestedInstance
        /\ requestedInstance' = _TETrace[j].requestedInstance
        /\ caller  = _TETrace[i].caller
        /\ caller' = _TETrace[j].caller
        /\ credential  = _TETrace[i].credential
        /\ credential' = _TETrace[j].credential
        /\ component  = _TETrace[i].component
        /\ component' = _TETrace[j].component
        /\ phase  = _TETrace[i].phase
        /\ phase' = _TETrace[j].phase
        /\ credentialSource  = _TETrace[i].credentialSource
        /\ credentialSource' = _TETrace[j].credentialSource

\* Uncomment the ASSUME below to write the states of the error trace
\* to the given file in Json format. Note that you can pass any tuple
\* to `JsonSerialize`. For example, a sub-sequence of _TETrace.
    \* ASSUME
    \*     LET J == INSTANCE Json
    \*         IN J!JsonSerialize("OpenAPIWorkerResolver_TTrace_1794222781.json", _TETrace)

=============================================================================

 Note that you can extract this module `OpenAPIWorkerResolver_TEExpression`
  to a dedicated file to reuse `expression` (the module in the 
  dedicated `OpenAPIWorkerResolver_TEExpression.tla` file takes precedence 
  over the module `OpenAPIWorkerResolver_TEExpression` below).

---- MODULE OpenAPIWorkerResolver_TEExpression ----
EXTENDS Sequences, TLCExt, Toolbox, Naturals, TLC, OpenAPIWorkerResolver

expression == 
    [
        \* To hide variables of the `OpenAPIWorkerResolver` spec from the error trace,
        \* remove the variables below.  The trace will be written in the order
        \* of the fields of this record.
        operation |-> operation
        ,requestedInstance |-> requestedInstance
        ,caller |-> caller
        ,credential |-> credential
        ,component |-> component
        ,phase |-> phase
        ,credentialSource |-> credentialSource
        
        \* Put additional constant-, state-, and action-level expressions here:
        \* ,_stateNumber |-> _TEPosition
        \* ,_operationUnchanged |-> operation = operation'
        
        \* Format the `operation` variable as Json value.
        \* ,_operationJson |->
        \*     LET J == INSTANCE Json
        \*     IN J!ToJson(operation)
        
        \* Lastly, you may build expressions over arbitrary sets of states by
        \* leveraging the _TETrace operator.  For example, this is how to
        \* count the number of times a spec variable changed up to the current
        \* state in the trace.
        \* ,_operationModCount |->
        \*     LET F[s \in DOMAIN _TETrace] ==
        \*         IF s = 1 THEN 0
        \*         ELSE IF _TETrace[s].operation # _TETrace[s-1].operation
        \*             THEN 1 + F[s-1] ELSE F[s-1]
        \*     IN F[_TEPosition - 1]
    ]

=============================================================================



Parsing and semantic processing can take forever if the trace below is long.
 In this case, it is advised to uncomment the module below to deserialize the
 trace from a generated binary file.

\*
\*---- MODULE OpenAPIWorkerResolver_TETrace ----
\*EXTENDS IOUtils, TLC, OpenAPIWorkerResolver
\*
\*trace == IODeserialize("OpenAPIWorkerResolver_TTrace_1794222781.bin", TRUE)
\*
\*=============================================================================
\*

---- MODULE OpenAPIWorkerResolver_TETrace ----
EXTENDS TLC, OpenAPIWorkerResolver

trace == 
    <<
    ([phase |-> "request_received",caller |-> "none",component |-> "none",credential |-> [owner |-> "none", token |-> "none", valid |-> FALSE],credentialSource |-> "none",operation |-> "none",requestedInstance |-> "none"]),
    ([phase |-> "identity_resolved",caller |-> "u2",component |-> "none",credential |-> [owner |-> "none", token |-> "none", valid |-> FALSE],credentialSource |-> "none",operation |-> "none",requestedInstance |-> "none"]),
    ([phase |-> "instance_selected",caller |-> "u2",component |-> "none",credential |-> [owner |-> "none", token |-> "none", valid |-> FALSE],credentialSource |-> "none",operation |-> "none",requestedInstance |-> "i2"]),
    ([phase |-> "snapshot_resolution",caller |-> "u2",component |-> "none",credential |-> [owner |-> "none", token |-> "none", valid |-> FALSE],credentialSource |-> "none",operation |-> "none",requestedInstance |-> "i2"]),
    ([phase |-> "operation_authorization",caller |-> "u2",component |-> "component-1",credential |-> [owner |-> "none", token |-> "none", valid |-> FALSE],credentialSource |-> "none",operation |-> "tool-1",requestedInstance |-> "i2"]),
    ([phase |-> "credential_resolution",caller |-> "u2",component |-> "component-1",credential |-> [owner |-> "none", token |-> "none", valid |-> FALSE],credentialSource |-> "none",operation |-> "tool-1",requestedInstance |-> "i2"]),
    ([phase |-> "credential_ready",caller |-> "u2",component |-> "component-1",credential |-> [owner |-> "u1", token |-> "synthetic-u1-token", valid |-> TRUE],credentialSource |-> "i1",operation |-> "tool-1",requestedInstance |-> "i2"]),
    ([phase |-> "dispatched",caller |-> "u2",component |-> "component-1",credential |-> [owner |-> "u1", token |-> "synthetic-u1-token", valid |-> TRUE],credentialSource |-> "i1",operation |-> "tool-1",requestedInstance |-> "i2"])
    >>
----


=============================================================================

---- CONFIG OpenAPIWorkerResolver_TTrace_1794222781 ----
CONSTANTS
    FallbackEnabled = TRUE

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
\* Generated on Mon Nov 09 19:13:02 CST 2026