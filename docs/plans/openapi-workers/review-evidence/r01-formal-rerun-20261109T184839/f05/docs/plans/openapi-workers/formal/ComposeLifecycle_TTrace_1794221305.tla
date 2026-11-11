---- MODULE ComposeLifecycle_TTrace_1794221305 ----
EXTENDS Sequences, TLCExt, ComposeLifecycle, Toolbox, Naturals, TLC

_expression ==
    LET ComposeLifecycle_TEExpression == INSTANCE ComposeLifecycle_TEExpression
    IN ComposeLifecycle_TEExpression!expression
----

_trace ==
    LET ComposeLifecycle_TETrace == INSTANCE ComposeLifecycle_TETrace
    IN ComposeLifecycle_TETrace!trace
----

_inv ==
    ~(
        TLCGet("level") = Len(_TETrace)
        /\
        rolloutTarget = (1)
        /\
        database = ("absent")
        /\
        rollout = ("none")
        /\
        obot = (<<"absent", "absent">>)
        /\
        sharedNetwork = (FALSE)
        /\
        serviceAuth = ("absent")
        /\
        worker = (<<"absent", "absent">>)
        /\
        activeCalls = (1)
        /\
        infrastructureOutage = (FALSE)
    )
----

_init ==
    /\ obot = _TETrace[1].obot
    /\ database = _TETrace[1].database
    /\ rollout = _TETrace[1].rollout
    /\ rolloutTarget = _TETrace[1].rolloutTarget
    /\ sharedNetwork = _TETrace[1].sharedNetwork
    /\ activeCalls = _TETrace[1].activeCalls
    /\ worker = _TETrace[1].worker
    /\ serviceAuth = _TETrace[1].serviceAuth
    /\ infrastructureOutage = _TETrace[1].infrastructureOutage
----

_next ==
    /\ \E i,j \in DOMAIN _TETrace:
        /\ \/ /\ j = i + 1
              /\ i = TLCGet("level")
        /\ obot  = _TETrace[i].obot
        /\ obot' = _TETrace[j].obot
        /\ database  = _TETrace[i].database
        /\ database' = _TETrace[j].database
        /\ rollout  = _TETrace[i].rollout
        /\ rollout' = _TETrace[j].rollout
        /\ rolloutTarget  = _TETrace[i].rolloutTarget
        /\ rolloutTarget' = _TETrace[j].rolloutTarget
        /\ sharedNetwork  = _TETrace[i].sharedNetwork
        /\ sharedNetwork' = _TETrace[j].sharedNetwork
        /\ activeCalls  = _TETrace[i].activeCalls
        /\ activeCalls' = _TETrace[j].activeCalls
        /\ worker  = _TETrace[i].worker
        /\ worker' = _TETrace[j].worker
        /\ serviceAuth  = _TETrace[i].serviceAuth
        /\ serviceAuth' = _TETrace[j].serviceAuth
        /\ infrastructureOutage  = _TETrace[i].infrastructureOutage
        /\ infrastructureOutage' = _TETrace[j].infrastructureOutage

\* Uncomment the ASSUME below to write the states of the error trace
\* to the given file in Json format. Note that you can pass any tuple
\* to `JsonSerialize`. For example, a sub-sequence of _TETrace.
    \* ASSUME
    \*     LET J == INSTANCE Json
    \*         IN J!JsonSerialize("ComposeLifecycle_TTrace_1794221305.json", _TETrace)

=============================================================================

 Note that you can extract this module `ComposeLifecycle_TEExpression`
  to a dedicated file to reuse `expression` (the module in the 
  dedicated `ComposeLifecycle_TEExpression.tla` file takes precedence 
  over the module `ComposeLifecycle_TEExpression` below).

---- MODULE ComposeLifecycle_TEExpression ----
EXTENDS Sequences, TLCExt, ComposeLifecycle, Toolbox, Naturals, TLC

expression == 
    [
        \* To hide variables of the `ComposeLifecycle` spec from the error trace,
        \* remove the variables below.  The trace will be written in the order
        \* of the fields of this record.
        obot |-> obot
        ,database |-> database
        ,rollout |-> rollout
        ,rolloutTarget |-> rolloutTarget
        ,sharedNetwork |-> sharedNetwork
        ,activeCalls |-> activeCalls
        ,worker |-> worker
        ,serviceAuth |-> serviceAuth
        ,infrastructureOutage |-> infrastructureOutage
        
        \* Put additional constant-, state-, and action-level expressions here:
        \* ,_stateNumber |-> _TEPosition
        \* ,_obotUnchanged |-> obot = obot'
        
        \* Format the `obot` variable as Json value.
        \* ,_obotJson |->
        \*     LET J == INSTANCE Json
        \*     IN J!ToJson(obot)
        
        \* Lastly, you may build expressions over arbitrary sets of states by
        \* leveraging the _TETrace operator.  For example, this is how to
        \* count the number of times a spec variable changed up to the current
        \* state in the trace.
        \* ,_obotModCount |->
        \*     LET F[s \in DOMAIN _TETrace] ==
        \*         IF s = 1 THEN 0
        \*         ELSE IF _TETrace[s].obot # _TETrace[s-1].obot
        \*             THEN 1 + F[s-1] ELSE F[s-1]
        \*     IN F[_TEPosition - 1]
    ]

=============================================================================



Parsing and semantic processing can take forever if the trace below is long.
 In this case, it is advised to uncomment the module below to deserialize the
 trace from a generated binary file.

\*
\*---- MODULE ComposeLifecycle_TETrace ----
\*EXTENDS IOUtils, ComposeLifecycle, TLC
\*
\*trace == IODeserialize("ComposeLifecycle_TTrace_1794221305.bin", TRUE)
\*
\*=============================================================================
\*

---- MODULE ComposeLifecycle_TETrace ----
EXTENDS ComposeLifecycle, TLC

trace == 
    <<
    ([rolloutTarget |-> 1,database |-> "absent",rollout |-> "none",obot |-> <<"absent", "absent">>,sharedNetwork |-> FALSE,serviceAuth |-> "absent",worker |-> <<"absent", "absent">>,activeCalls |-> 0,infrastructureOutage |-> FALSE]),
    ([rolloutTarget |-> 1,database |-> "absent",rollout |-> "none",obot |-> <<"absent", "absent">>,sharedNetwork |-> FALSE,serviceAuth |-> "absent",worker |-> <<"absent", "absent">>,activeCalls |-> 1,infrastructureOutage |-> FALSE])
    >>
----


=============================================================================

---- CONFIG ComposeLifecycle_TTrace_1794221305 ----
CONSTANTS
    MaxActiveCalls = 1
    BreakRouteGate = TRUE
    BreakRollGuard = TRUE

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
\* Generated on Mon Nov 09 18:48:26 CST 2026