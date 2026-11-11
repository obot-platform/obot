---- MODULE ExecutionBudgetLeases_TTrace_1794222786 ----
EXTENDS Sequences, TLCExt, Toolbox, Naturals, TLC, ExecutionBudgetLeases, ExecutionBudgetLeases_TEConstants

_expression ==
    LET ExecutionBudgetLeases_TEExpression == INSTANCE ExecutionBudgetLeases_TEExpression
    IN ExecutionBudgetLeases_TEExpression!expression
----

_trace ==
    LET ExecutionBudgetLeases_TETrace == INSTANCE ExecutionBudgetLeases_TETrace
    IN ExecutionBudgetLeases_TETrace!trace
----

_inv ==
    ~(
        TLCGet("level") = Len(_TETrace)
        /\
        phase = ((r1 :> "completed" @@ r2 :> "new"))
        /\
        generation = ((r1 :> 1 @@ r2 :> 0))
        /\
        reservedCount = (0)
        /\
        waitDeadline = ((r1 :> 2 @@ r2 :> 0))
        /\
        delivery = ((r1 :> "delivered" @@ r2 :> "none"))
        /\
        upstream = ((r1 :> "sent" @@ r2 :> "none"))
        /\
        upstreamStart = ((r1 :> 1 @@ r2 :> 0))
        /\
        origin = ((r1 :> obot1 @@ r2 :> "none"))
        /\
        now = (3)
        /\
        deadline = ((r1 :> 3 @@ r2 :> 0))
        /\
        finishedAt = ((r1 :> 3 @@ r2 :> 0))
    )
----

_init ==
    /\ phase = _TETrace[1].phase
    /\ origin = _TETrace[1].origin
    /\ finishedAt = _TETrace[1].finishedAt
    /\ upstream = _TETrace[1].upstream
    /\ now = _TETrace[1].now
    /\ upstreamStart = _TETrace[1].upstreamStart
    /\ waitDeadline = _TETrace[1].waitDeadline
    /\ delivery = _TETrace[1].delivery
    /\ deadline = _TETrace[1].deadline
    /\ generation = _TETrace[1].generation
    /\ reservedCount = _TETrace[1].reservedCount
----

_next ==
    /\ \E i,j \in DOMAIN _TETrace:
        /\ \/ /\ j = i + 1
              /\ i = TLCGet("level")
        /\ phase  = _TETrace[i].phase
        /\ phase' = _TETrace[j].phase
        /\ origin  = _TETrace[i].origin
        /\ origin' = _TETrace[j].origin
        /\ finishedAt  = _TETrace[i].finishedAt
        /\ finishedAt' = _TETrace[j].finishedAt
        /\ upstream  = _TETrace[i].upstream
        /\ upstream' = _TETrace[j].upstream
        /\ now  = _TETrace[i].now
        /\ now' = _TETrace[j].now
        /\ upstreamStart  = _TETrace[i].upstreamStart
        /\ upstreamStart' = _TETrace[j].upstreamStart
        /\ waitDeadline  = _TETrace[i].waitDeadline
        /\ waitDeadline' = _TETrace[j].waitDeadline
        /\ delivery  = _TETrace[i].delivery
        /\ delivery' = _TETrace[j].delivery
        /\ deadline  = _TETrace[i].deadline
        /\ deadline' = _TETrace[j].deadline
        /\ generation  = _TETrace[i].generation
        /\ generation' = _TETrace[j].generation
        /\ reservedCount  = _TETrace[i].reservedCount
        /\ reservedCount' = _TETrace[j].reservedCount

\* Uncomment the ASSUME below to write the states of the error trace
\* to the given file in Json format. Note that you can pass any tuple
\* to `JsonSerialize`. For example, a sub-sequence of _TETrace.
    \* ASSUME
    \*     LET J == INSTANCE Json
    \*         IN J!JsonSerialize("ExecutionBudgetLeases_TTrace_1794222786.json", _TETrace)

=============================================================================

 Note that you can extract this module `ExecutionBudgetLeases_TEExpression`
  to a dedicated file to reuse `expression` (the module in the 
  dedicated `ExecutionBudgetLeases_TEExpression.tla` file takes precedence 
  over the module `ExecutionBudgetLeases_TEExpression` below).

---- MODULE ExecutionBudgetLeases_TEExpression ----
EXTENDS Sequences, TLCExt, Toolbox, Naturals, TLC, ExecutionBudgetLeases, ExecutionBudgetLeases_TEConstants

expression == 
    [
        \* To hide variables of the `ExecutionBudgetLeases` spec from the error trace,
        \* remove the variables below.  The trace will be written in the order
        \* of the fields of this record.
        phase |-> phase
        ,origin |-> origin
        ,finishedAt |-> finishedAt
        ,upstream |-> upstream
        ,now |-> now
        ,upstreamStart |-> upstreamStart
        ,waitDeadline |-> waitDeadline
        ,delivery |-> delivery
        ,deadline |-> deadline
        ,generation |-> generation
        ,reservedCount |-> reservedCount
        
        \* Put additional constant-, state-, and action-level expressions here:
        \* ,_stateNumber |-> _TEPosition
        \* ,_phaseUnchanged |-> phase = phase'
        
        \* Format the `phase` variable as Json value.
        \* ,_phaseJson |->
        \*     LET J == INSTANCE Json
        \*     IN J!ToJson(phase)
        
        \* Lastly, you may build expressions over arbitrary sets of states by
        \* leveraging the _TETrace operator.  For example, this is how to
        \* count the number of times a spec variable changed up to the current
        \* state in the trace.
        \* ,_phaseModCount |->
        \*     LET F[s \in DOMAIN _TETrace] ==
        \*         IF s = 1 THEN 0
        \*         ELSE IF _TETrace[s].phase # _TETrace[s-1].phase
        \*             THEN 1 + F[s-1] ELSE F[s-1]
        \*     IN F[_TEPosition - 1]
    ]

=============================================================================



Parsing and semantic processing can take forever if the trace below is long.
 In this case, it is advised to uncomment the module below to deserialize the
 trace from a generated binary file.

\*
\*---- MODULE ExecutionBudgetLeases_TETrace ----
\*EXTENDS IOUtils, TLC, ExecutionBudgetLeases, ExecutionBudgetLeases_TEConstants
\*
\*trace == IODeserialize("ExecutionBudgetLeases_TTrace_1794222786.bin", TRUE)
\*
\*=============================================================================
\*

---- MODULE ExecutionBudgetLeases_TETrace ----
EXTENDS TLC, ExecutionBudgetLeases, ExecutionBudgetLeases_TEConstants

trace == 
    <<
    ([phase |-> (r1 :> "new" @@ r2 :> "new"),generation |-> (r1 :> 0 @@ r2 :> 0),reservedCount |-> 0,waitDeadline |-> (r1 :> 0 @@ r2 :> 0),delivery |-> (r1 :> "none" @@ r2 :> "none"),upstream |-> (r1 :> "none" @@ r2 :> "none"),upstreamStart |-> (r1 :> 0 @@ r2 :> 0),origin |-> (r1 :> "none" @@ r2 :> "none"),now |-> 0,deadline |-> (r1 :> 0 @@ r2 :> 0),finishedAt |-> (r1 :> 0 @@ r2 :> 0)]),
    ([phase |-> (r1 :> "waiting" @@ r2 :> "new"),generation |-> (r1 :> 0 @@ r2 :> 0),reservedCount |-> 0,waitDeadline |-> (r1 :> 2 @@ r2 :> 0),delivery |-> (r1 :> "none" @@ r2 :> "none"),upstream |-> (r1 :> "none" @@ r2 :> "none"),upstreamStart |-> (r1 :> 0 @@ r2 :> 0),origin |-> (r1 :> obot1 @@ r2 :> "none"),now |-> 0,deadline |-> (r1 :> 0 @@ r2 :> 0),finishedAt |-> (r1 :> 0 @@ r2 :> 0)]),
    ([phase |-> (r1 :> "active" @@ r2 :> "new"),generation |-> (r1 :> 1 @@ r2 :> 0),reservedCount |-> 1,waitDeadline |-> (r1 :> 2 @@ r2 :> 0),delivery |-> (r1 :> "none" @@ r2 :> "none"),upstream |-> (r1 :> "none" @@ r2 :> "none"),upstreamStart |-> (r1 :> 0 @@ r2 :> 0),origin |-> (r1 :> obot1 @@ r2 :> "none"),now |-> 0,deadline |-> (r1 :> 3 @@ r2 :> 0),finishedAt |-> (r1 :> 0 @@ r2 :> 0)]),
    ([phase |-> (r1 :> "active" @@ r2 :> "new"),generation |-> (r1 :> 1 @@ r2 :> 0),reservedCount |-> 1,waitDeadline |-> (r1 :> 2 @@ r2 :> 0),delivery |-> (r1 :> "none" @@ r2 :> "none"),upstream |-> (r1 :> "none" @@ r2 :> "none"),upstreamStart |-> (r1 :> 0 @@ r2 :> 0),origin |-> (r1 :> obot1 @@ r2 :> "none"),now |-> 1,deadline |-> (r1 :> 3 @@ r2 :> 0),finishedAt |-> (r1 :> 0 @@ r2 :> 0)]),
    ([phase |-> (r1 :> "active" @@ r2 :> "new"),generation |-> (r1 :> 1 @@ r2 :> 0),reservedCount |-> 1,waitDeadline |-> (r1 :> 2 @@ r2 :> 0),delivery |-> (r1 :> "delivered" @@ r2 :> "none"),upstream |-> (r1 :> "none" @@ r2 :> "none"),upstreamStart |-> (r1 :> 0 @@ r2 :> 0),origin |-> (r1 :> obot1 @@ r2 :> "none"),now |-> 1,deadline |-> (r1 :> 3 @@ r2 :> 0),finishedAt |-> (r1 :> 0 @@ r2 :> 0)]),
    ([phase |-> (r1 :> "active" @@ r2 :> "new"),generation |-> (r1 :> 1 @@ r2 :> 0),reservedCount |-> 1,waitDeadline |-> (r1 :> 2 @@ r2 :> 0),delivery |-> (r1 :> "delivered" @@ r2 :> "none"),upstream |-> (r1 :> "sent" @@ r2 :> "none"),upstreamStart |-> (r1 :> 1 @@ r2 :> 0),origin |-> (r1 :> obot1 @@ r2 :> "none"),now |-> 1,deadline |-> (r1 :> 3 @@ r2 :> 0),finishedAt |-> (r1 :> 0 @@ r2 :> 0)]),
    ([phase |-> (r1 :> "active" @@ r2 :> "new"),generation |-> (r1 :> 1 @@ r2 :> 0),reservedCount |-> 1,waitDeadline |-> (r1 :> 2 @@ r2 :> 0),delivery |-> (r1 :> "delivered" @@ r2 :> "none"),upstream |-> (r1 :> "sent" @@ r2 :> "none"),upstreamStart |-> (r1 :> 1 @@ r2 :> 0),origin |-> (r1 :> obot1 @@ r2 :> "none"),now |-> 2,deadline |-> (r1 :> 3 @@ r2 :> 0),finishedAt |-> (r1 :> 0 @@ r2 :> 0)]),
    ([phase |-> (r1 :> "expiredPending" @@ r2 :> "new"),generation |-> (r1 :> 1 @@ r2 :> 0),reservedCount |-> 1,waitDeadline |-> (r1 :> 2 @@ r2 :> 0),delivery |-> (r1 :> "delivered" @@ r2 :> "none"),upstream |-> (r1 :> "sent" @@ r2 :> "none"),upstreamStart |-> (r1 :> 1 @@ r2 :> 0),origin |-> (r1 :> obot1 @@ r2 :> "none"),now |-> 3,deadline |-> (r1 :> 3 @@ r2 :> 0),finishedAt |-> (r1 :> 0 @@ r2 :> 0)]),
    ([phase |-> (r1 :> "completed" @@ r2 :> "new"),generation |-> (r1 :> 1 @@ r2 :> 0),reservedCount |-> 0,waitDeadline |-> (r1 :> 2 @@ r2 :> 0),delivery |-> (r1 :> "delivered" @@ r2 :> "none"),upstream |-> (r1 :> "sent" @@ r2 :> "none"),upstreamStart |-> (r1 :> 1 @@ r2 :> 0),origin |-> (r1 :> obot1 @@ r2 :> "none"),now |-> 3,deadline |-> (r1 :> 3 @@ r2 :> 0),finishedAt |-> (r1 :> 3 @@ r2 :> 0)])
    >>
----


=============================================================================

---- MODULE ExecutionBudgetLeases_TEConstants ----
EXTENDS ExecutionBudgetLeases

CONSTANTS r1, r2, obot1, obot2

=============================================================================

---- CONFIG ExecutionBudgetLeases_TTrace_1794222786 ----
CONSTANTS
    Requests = { r1 , r2 }
    Replicas = { obot1 , obot2 }
    MaxActive = 1
    MaxWaiting = 1
    LeaseTTL = 3
    WaitBudget = 2
    ReclaimGrace = 1
    FencingMargin = 1
    Horizon = 8
    BrokenEarlyRelease = TRUE
    BrokenStaleStart = FALSE
    r1 = r1
    obot2 = obot2
    obot1 = obot1
    r2 = r2

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
\* Generated on Mon Nov 09 19:13:07 CST 2026