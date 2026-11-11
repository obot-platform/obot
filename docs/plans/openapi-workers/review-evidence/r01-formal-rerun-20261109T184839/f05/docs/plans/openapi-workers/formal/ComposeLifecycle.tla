--------------------------- MODULE ComposeLifecycle ---------------------------
EXTENDS Naturals

CONSTANTS MaxActiveCalls, BreakRouteGate, BreakRollGuard

VARIABLES sharedNetwork,
          database,
          obot,
          worker,
          serviceAuth,
          activeCalls,
          rollout,
          rolloutTarget,
          infrastructureOutage

ServiceState == {"absent", "starting", "ready", "draining", "failed"}
DatabaseState == {"absent", "starting", "ready", "failed"}
AuthState == {"absent", "staged", "trusted"}
RolloutState == {"none", "obot", "worker"}

Vars == <<sharedNetwork, database, obot, worker, serviceAuth,
          activeCalls, rollout, rolloutTarget, infrastructureOutage>>

Init ==
  /\ sharedNetwork = FALSE
  /\ database = "absent"
  /\ obot = [i \in {1, 2} |-> "absent"]
  /\ worker = [i \in {1, 2} |-> "absent"]
  /\ serviceAuth = "absent"
  /\ activeCalls = 0
  /\ rollout = "none"
  /\ rolloutTarget = 1
  /\ infrastructureOutage = FALSE

ServiceAuthUsable == serviceAuth = "trusted"
AnyObotReady == obot[1] = "ready" \/ obot[2] = "ready"
AnyWorkerReady == worker[1] = "ready" \/ worker[2] = "ready"

ObotCapacityReady == database = "ready" /\ AnyObotReady
WorkerCapacityReady == sharedNetwork /\ ServiceAuthUsable /\ AnyWorkerReady
TrafficCapacityReady == ObotCapacityReady /\ WorkerCapacityReady
PlannedRollActive == rollout # "none"

CreateSharedNetwork ==
  /\ ~sharedNetwork
  /\ sharedNetwork' = TRUE
  /\ UNCHANGED <<database, obot, worker, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

DestroySharedNetwork ==
  /\ sharedNetwork
  /\ sharedNetwork' = FALSE
  /\ activeCalls' = 0
  /\ infrastructureOutage' = IF PlannedRollActive THEN TRUE ELSE infrastructureOutage
  /\ UNCHANGED <<database, obot, worker, serviceAuth,
                  rollout, rolloutTarget>>

PrepareDatabase ==
  /\ database \in {"absent", "failed"}
  /\ database' = "starting"
  /\ UNCHANGED <<sharedNetwork, obot, worker, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

DatabaseBecomesReady ==
  /\ database = "starting"
  /\ database' = "ready"
  /\ UNCHANGED <<sharedNetwork, obot, worker, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

DatabaseFails ==
  /\ database = "ready"
  /\ database' = "failed"
  /\ activeCalls' = 0
  /\ infrastructureOutage' = IF PlannedRollActive THEN TRUE ELSE infrastructureOutage
  /\ UNCHANGED <<sharedNetwork, obot, worker, serviceAuth,
                  rollout, rolloutTarget>>

ConfigureServiceAuth ==
  /\ serviceAuth = "absent"
  /\ serviceAuth' = "staged"
  /\ UNCHANGED <<sharedNetwork, database, obot, worker, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

TrustServiceAuth ==
  /\ serviceAuth = "staged"
  /\ serviceAuth' = "trusted"
  /\ UNCHANGED <<sharedNetwork, database, obot, worker, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

WithdrawServiceAuth ==
  /\ serviceAuth = "trusted"
  /\ serviceAuth' = "absent"
  /\ activeCalls' = 0
  /\ infrastructureOutage' = IF PlannedRollActive THEN TRUE ELSE infrastructureOutage
  /\ UNCHANGED <<sharedNetwork, database, obot, worker,
                  rollout, rolloutTarget>>

StartObot(i) ==
  /\ database = "ready"
  /\ obot[i] \in {"absent", "failed"}
  /\ obot' = [obot EXCEPT ![i] = "starting"]
  /\ UNCHANGED <<sharedNetwork, database, worker, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

ObotBecomesReady(i) ==
  /\ obot[i] = "starting"
  /\ obot' = [obot EXCEPT ![i] = "ready"]
  /\ UNCHANGED <<sharedNetwork, database, worker, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

OtherObotReady(i) == obot[IF i = 1 THEN 2 ELSE 1] = "ready"

DrainObot(i) ==
  /\ obot[i] = "ready"
  /\ \/ OtherObotReady(i)
     \/ BreakRollGuard /\ PlannedRollActive
  /\ obot' = [obot EXCEPT ![i] = "draining"]
  /\ UNCHANGED <<sharedNetwork, database, worker, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

ReplaceObot(i) ==
  /\ obot[i] = "draining"
  /\ obot' = [obot EXCEPT ![i] = "absent"]
  /\ UNCHANGED <<sharedNetwork, database, worker, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

ObotFails(i) ==
  /\ obot[i] # "absent"
  /\ obot' = [obot EXCEPT ![i] = "failed"]
  /\ activeCalls' = 0
  /\ infrastructureOutage' = IF PlannedRollActive THEN TRUE ELSE infrastructureOutage
  /\ UNCHANGED <<sharedNetwork, database, worker, serviceAuth,
                  rollout, rolloutTarget>>

StartWorker(i) ==
  /\ sharedNetwork
  /\ ServiceAuthUsable
  /\ worker[i] \in {"absent", "failed"}
  /\ worker' = [worker EXCEPT ![i] = "starting"]
  /\ UNCHANGED <<sharedNetwork, database, obot, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

WorkerBecomesReady(i) ==
  /\ worker[i] = "starting"
  /\ worker' = [worker EXCEPT ![i] = "ready"]
  /\ UNCHANGED <<sharedNetwork, database, obot, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

OtherWorkerReady(i) == worker[IF i = 1 THEN 2 ELSE 1] = "ready"

DrainWorker(i) ==
  /\ worker[i] = "ready"
  /\ \/ OtherWorkerReady(i)
     \/ BreakRollGuard /\ PlannedRollActive
  /\ worker' = [worker EXCEPT ![i] = "draining"]
  /\ UNCHANGED <<sharedNetwork, database, obot, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

ReplaceWorker(i) ==
  /\ worker[i] = "draining"
  /\ worker' = [worker EXCEPT ![i] = "absent"]
  /\ UNCHANGED <<sharedNetwork, database, obot, serviceAuth, activeCalls,
                  rollout, rolloutTarget, infrastructureOutage>>

WorkerFails(i) ==
  /\ worker[i] # "absent"
  /\ worker' = [worker EXCEPT ![i] = "failed"]
  /\ activeCalls' = 0
  /\ infrastructureOutage' = IF PlannedRollActive THEN TRUE ELSE infrastructureOutage
  /\ UNCHANGED <<sharedNetwork, database, obot, serviceAuth,
                  rollout, rolloutTarget>>

BeginObotRoll(i) ==
  /\ rollout = "none"
  /\ TrafficCapacityReady
  /\ obot[i] = "ready"
  /\ rollout' = "obot"
  /\ rolloutTarget' = i
  /\ UNCHANGED <<sharedNetwork, database, obot, worker, serviceAuth,
                  activeCalls, infrastructureOutage>>

BeginWorkerRoll(i) ==
  /\ rollout = "none"
  /\ TrafficCapacityReady
  /\ worker[i] = "ready"
  /\ rollout' = "worker"
  /\ rolloutTarget' = i
  /\ UNCHANGED <<sharedNetwork, database, obot, worker, serviceAuth,
                  activeCalls, infrastructureOutage>>

FinishRoll ==
  /\ PlannedRollActive
  /\ TrafficCapacityReady
  /\ (rollout = "obot" => obot[rolloutTarget] = "ready")
  /\ (rollout = "worker" => worker[rolloutTarget] = "ready")
  /\ rollout' = "none"
  /\ UNCHANGED <<sharedNetwork, database, obot, worker, serviceAuth,
                  activeCalls, rolloutTarget, infrastructureOutage>>

AdmitOpenAPICall ==
  /\ activeCalls < MaxActiveCalls
  /\ \/ ~BreakRouteGate /\ TrafficCapacityReady
     \/ BreakRouteGate
  /\ activeCalls' = activeCalls + 1
  /\ UNCHANGED <<sharedNetwork, database, obot, worker, serviceAuth,
                  rollout, rolloutTarget, infrastructureOutage>>

CompleteOpenAPICall ==
  /\ activeCalls > 0
  /\ activeCalls' = activeCalls - 1
  /\ UNCHANGED <<sharedNetwork, database, obot, worker, serviceAuth,
                  rollout, rolloutTarget, infrastructureOutage>>

ClearOutageAfterRecovery ==
  /\ infrastructureOutage
  /\ TrafficCapacityReady
  /\ activeCalls = 0
  /\ infrastructureOutage' = FALSE
  /\ UNCHANGED <<sharedNetwork, database, obot, worker, serviceAuth,
                  activeCalls, rollout, rolloutTarget>>

Next ==
  \/ CreateSharedNetwork
  \/ DestroySharedNetwork
  \/ PrepareDatabase
  \/ DatabaseBecomesReady
  \/ DatabaseFails
  \/ ConfigureServiceAuth
  \/ TrustServiceAuth
  \/ WithdrawServiceAuth
  \/ \E i \in {1, 2}: StartObot(i)
  \/ \E i \in {1, 2}: ObotBecomesReady(i)
  \/ \E i \in {1, 2}: DrainObot(i)
  \/ \E i \in {1, 2}: ReplaceObot(i)
  \/ \E i \in {1, 2}: ObotFails(i)
  \/ \E i \in {1, 2}: StartWorker(i)
  \/ \E i \in {1, 2}: WorkerBecomesReady(i)
  \/ \E i \in {1, 2}: DrainWorker(i)
  \/ \E i \in {1, 2}: ReplaceWorker(i)
  \/ \E i \in {1, 2}: WorkerFails(i)
  \/ \E i \in {1, 2}: BeginObotRoll(i)
  \/ \E i \in {1, 2}: BeginWorkerRoll(i)
  \/ FinishRoll
  \/ AdmitOpenAPICall
  \/ CompleteOpenAPICall
  \/ ClearOutageAfterRecovery

Spec == Init /\ [][Next]_Vars

TypeOK ==
  /\ sharedNetwork \in BOOLEAN
  /\ database \in DatabaseState
  /\ \A i \in {1, 2}: obot[i] \in ServiceState
  /\ \A i \in {1, 2}: worker[i] \in ServiceState
  /\ serviceAuth \in AuthState
  /\ activeCalls \in 0..MaxActiveCalls
  /\ rollout \in RolloutState
  /\ rolloutTarget \in {1, 2}
  /\ infrastructureOutage \in BOOLEAN

NoDeadlock == TRUE

OpenAPITrafficGated ==
  activeCalls = 0 \/ TrafficCapacityReady

PlannedRollRetainsCapacity ==
  rollout = "none" \/ infrastructureOutage \/ TrafficCapacityReady

ChartAllServicesAbsent ==
  obot[1] = "absent" /\ obot[2] = "absent" /\
  worker[1] = "absent" /\ worker[2] = "absent"

ChartConstraint ==
  \/ ~sharedNetwork /\ database = "absent" /\ serviceAuth = "absent" /\
     ChartAllServicesAbsent /\ activeCalls = 0 /\ rollout = "none" /\
     infrastructureOutage = FALSE
  \/ sharedNetwork /\ database = "absent" /\ serviceAuth = "absent" /\
     ChartAllServicesAbsent /\ activeCalls = 0 /\ rollout = "none" /\
     infrastructureOutage = FALSE
  \/ sharedNetwork /\ database = "starting" /\ serviceAuth = "absent" /\
     ChartAllServicesAbsent /\ activeCalls = 0 /\ rollout = "none" /\
     infrastructureOutage = FALSE
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "absent" /\
     ChartAllServicesAbsent /\ activeCalls = 0 /\ rollout = "none" /\
     infrastructureOutage = FALSE
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "staged" /\
     ChartAllServicesAbsent /\ activeCalls = 0 /\ rollout = "none" /\
     infrastructureOutage = FALSE
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "trusted" /\
     worker[1] = "absent" /\ worker[2] = "absent" /\
     activeCalls = 0 /\ rollout = "none" /\ infrastructureOutage = FALSE /\
     \/ obot[1] = "absent" /\ obot[2] = "absent"
     \/ obot[1] = "starting" /\ obot[2] = "absent"
     \/ obot[1] = "ready" /\ obot[2] = "absent"
     \/ obot[1] = "ready" /\ obot[2] = "starting"
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "trusted" /\
     obot[1] = "ready" /\ obot[2] = "ready" /\ rollout = "none" /\
     infrastructureOutage = FALSE /\
     \/ worker[1] = "absent" /\ worker[2] = "absent" /\ activeCalls = 0
     \/ worker[1] = "starting" /\ worker[2] = "absent" /\ activeCalls = 0
     \/ worker[1] = "ready" /\ worker[2] = "absent" /\ activeCalls = 0
     \/ worker[1] = "ready" /\ worker[2] = "starting" /\ activeCalls = 0
     \/ worker[1] = "ready" /\ worker[2] = "ready" /\ activeCalls = 0
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "trusted" /\
     obot[1] = "ready" /\ obot[2] = "ready" /\
     worker[1] = "ready" /\ worker[2] = "ready" /\
     rollout = "none" /\ infrastructureOutage = FALSE /\
     activeCalls \in 0..MaxActiveCalls
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "trusted" /\
     obot[1] = "ready" /\ obot[2] = "ready" /\
     worker[2] = "ready" /\ rollout = "worker" /\
     rolloutTarget = 1 /\ infrastructureOutage = FALSE /\
     activeCalls \in 0..MaxActiveCalls /\
     worker[1] \in {"ready", "draining", "absent", "starting"}
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "trusted" /\
     obot[1] = "ready" /\ obot[2] = "ready" /\
     worker[1] = "ready" /\ worker[2] = "ready" /\
     rollout = "none" /\ infrastructureOutage = FALSE /\
     activeCalls \in 0..MaxActiveCalls
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "trusted" /\
     worker[1] = "ready" /\ worker[2] = "ready" /\
     rollout = "obot" /\ rolloutTarget = 1 /\
     infrastructureOutage = FALSE /\
     activeCalls \in 0..MaxActiveCalls /\
     obot[1] \in {"ready", "draining", "absent", "starting"}

ChartBrokenConstraint ==
  \/ ChartConstraint
  \/ ~sharedNetwork /\ database = "absent" /\ serviceAuth = "absent" /\
     ChartAllServicesAbsent /\ activeCalls = 1 /\ rollout = "none" /\
     infrastructureOutage = FALSE
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "trusted" /\
     obot[1] = "ready" /\ obot[2] = "absent" /\ rollout = "none" /\
     infrastructureOutage = FALSE /\ activeCalls \in 0..MaxActiveCalls /\
     \/ worker[1] = "starting" /\ worker[2] = "absent"
     \/ worker[1] = "ready" /\ worker[2] = "absent"
     \/ worker[1] = "ready" /\ worker[2] = "starting"
     \/ worker[1] = "ready" /\ worker[2] = "ready"
  \/ sharedNetwork /\ database = "ready" /\ serviceAuth = "trusted" /\
     worker[1] = "ready" /\ worker[2] = "ready" /\
     rollout = "obot" /\ rolloutTarget = 1 /\
     infrastructureOutage = FALSE /\ activeCalls \in 0..MaxActiveCalls /\
     obot[2] = "absent" /\
     obot[1] \in {"ready", "draining", "absent", "starting"}

=============================================================================
