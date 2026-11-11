---- MODULE SnapshotFencing_TTrace_1794221302 ----
EXTENDS Sequences, TLCExt, SnapshotFencing, Toolbox, Naturals, TLC

_expression ==
    LET SnapshotFencing_TEExpression == INSTANCE SnapshotFencing_TEExpression
    IN SnapshotFencing_TEExpression!expression
----

_trace ==
    LET SnapshotFencing_TETrace == INSTANCE SnapshotFencing_TETrace
    IN SnapshotFencing_TETrace!trace
----

_inv ==
    ~(
        TLCGet("level") = Len(_TETrace)
        /\
        request = ([r1 |-> [phase |-> "executing", digest |-> "digest-a", supplies |-> 1, entries |-> 2, upstreamAttempts |-> 1, reuseRejections |-> 0, digestMismatchRejections |-> 0]])
    )
----

_init ==
    /\ request = _TETrace[1].request
----

_next ==
    /\ \E i,j \in DOMAIN _TETrace:
        /\ \/ /\ j = i + 1
              /\ i = TLCGet("level")
        /\ request  = _TETrace[i].request
        /\ request' = _TETrace[j].request

\* Uncomment the ASSUME below to write the states of the error trace
\* to the given file in Json format. Note that you can pass any tuple
\* to `JsonSerialize`. For example, a sub-sequence of _TETrace.
    \* ASSUME
    \*     LET J == INSTANCE Json
    \*         IN J!JsonSerialize("SnapshotFencing_TTrace_1794221302.json", _TETrace)

=============================================================================

 Note that you can extract this module `SnapshotFencing_TEExpression`
  to a dedicated file to reuse `expression` (the module in the 
  dedicated `SnapshotFencing_TEExpression.tla` file takes precedence 
  over the module `SnapshotFencing_TEExpression` below).

---- MODULE SnapshotFencing_TEExpression ----
EXTENDS Sequences, TLCExt, SnapshotFencing, Toolbox, Naturals, TLC

expression == 
    [
        \* To hide variables of the `SnapshotFencing` spec from the error trace,
        \* remove the variables below.  The trace will be written in the order
        \* of the fields of this record.
        request |-> request
        
        \* Put additional constant-, state-, and action-level expressions here:
        \* ,_stateNumber |-> _TEPosition
        \* ,_requestUnchanged |-> request = request'
        
        \* Format the `request` variable as Json value.
        \* ,_requestJson |->
        \*     LET J == INSTANCE Json
        \*     IN J!ToJson(request)
        
        \* Lastly, you may build expressions over arbitrary sets of states by
        \* leveraging the _TETrace operator.  For example, this is how to
        \* count the number of times a spec variable changed up to the current
        \* state in the trace.
        \* ,_requestModCount |->
        \*     LET F[s \in DOMAIN _TETrace] ==
        \*         IF s = 1 THEN 0
        \*         ELSE IF _TETrace[s].request # _TETrace[s-1].request
        \*             THEN 1 + F[s-1] ELSE F[s-1]
        \*     IN F[_TEPosition - 1]
    ]

=============================================================================



Parsing and semantic processing can take forever if the trace below is long.
 In this case, it is advised to uncomment the module below to deserialize the
 trace from a generated binary file.

\*
\*---- MODULE SnapshotFencing_TETrace ----
\*EXTENDS IOUtils, SnapshotFencing, TLC
\*
\*trace == IODeserialize("SnapshotFencing_TTrace_1794221302.bin", TRUE)
\*
\*=============================================================================
\*

---- MODULE SnapshotFencing_TETrace ----
EXTENDS SnapshotFencing, TLC

trace == 
    <<
    ([request |-> [r1 |-> [phase |-> "new", digest |-> "not-bound", supplies |-> 0, entries |-> 0, upstreamAttempts |-> 0, reuseRejections |-> 0, digestMismatchRejections |-> 0]]]),
    ([request |-> [r1 |-> [phase |-> "cache-miss", digest |-> "digest-a", supplies |-> 0, entries |-> 0, upstreamAttempts |-> 0, reuseRejections |-> 0, digestMismatchRejections |-> 0]]]),
    ([request |-> [r1 |-> [phase |-> "snapshot-required", digest |-> "digest-a", supplies |-> 0, entries |-> 0, upstreamAttempts |-> 0, reuseRejections |-> 0, digestMismatchRejections |-> 0]]]),
    ([request |-> [r1 |-> [phase |-> "compiling", digest |-> "digest-a", supplies |-> 1, entries |-> 0, upstreamAttempts |-> 0, reuseRejections |-> 0, digestMismatchRejections |-> 0]]]),
    ([request |-> [r1 |-> [phase |-> "ready", digest |-> "digest-a", supplies |-> 1, entries |-> 0, upstreamAttempts |-> 0, reuseRejections |-> 0, digestMismatchRejections |-> 0]]]),
    ([request |-> [r1 |-> [phase |-> "executing", digest |-> "digest-a", supplies |-> 1, entries |-> 1, upstreamAttempts |-> 0, reuseRejections |-> 0, digestMismatchRejections |-> 0]]]),
    ([request |-> [r1 |-> [phase |-> "executing", digest |-> "digest-a", supplies |-> 1, entries |-> 1, upstreamAttempts |-> 1, reuseRejections |-> 0, digestMismatchRejections |-> 0]]]),
    ([request |-> [r1 |-> [phase |-> "business-done", digest |-> "digest-a", supplies |-> 1, entries |-> 1, upstreamAttempts |-> 1, reuseRejections |-> 0, digestMismatchRejections |-> 0]]]),
    ([request |-> [r1 |-> [phase |-> "executing", digest |-> "digest-a", supplies |-> 1, entries |-> 2, upstreamAttempts |-> 1, reuseRejections |-> 0, digestMismatchRejections |-> 0]]])
    >>
----


=============================================================================

---- CONFIG SnapshotFencing_TTrace_1794221302 ----
CONSTANTS
    EnforceFence = FALSE
    MaxRejections = 1

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
\* Generated on Mon Nov 09 18:48:22 CST 2026