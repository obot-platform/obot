------------------------------- MODULE OpenAPIWorkerResolver -------------------------------
(***************************************************************************)
(* Design model for the P02 OpenAPI Worker per-invocation resolver. It     *)
(* checks the high-cost confidentiality property that a business request   *)
(* can be dispatched only with the calling user's credential from that     *)
(* user's exact instance context.                                          *)
(***************************************************************************)

CONSTANT FallbackEnabled

Users == {"u1", "u2"}
Instances == {"i1", "i2"}
Component == "component-1"
Tools == {"tool-1"}

InstanceOwner ==
  [instance \in Instances |->
     IF instance = "i1" THEN "u1" ELSE "u2"]

ProfileAllowed ==
  [user \in Users |->
     [instance \in Instances |->
        IF user = "u1" THEN instance = "i1" ELSE instance = "i2"]]

ToolAllowed ==
  [user \in Users |->
     [instance \in Instances |->
        [tool \in Tools |-> TRUE]]]

NoCredential == [owner |-> "none", token |-> "none", valid |-> FALSE]

(* Each credential is bound to the exact instance context. u2's required    *)
(* credential is absent so the model must fail closed rather than borrow a  *)
(* valid credential from u1's instance.                                     *)
Credentials ==
  [instance \in Instances |->
     IF instance = "i1"
       THEN [owner |-> "u1", token |-> "synthetic-u1-token", valid |-> TRUE]
       ELSE [owner |-> "u2", token |-> "none", valid |-> FALSE]]

VARIABLES
  phase,
  caller,
  requestedInstance,
  component,
  operation,
  credentialSource,
  credential

vars == <<phase, caller, requestedInstance, component, operation,
          credentialSource, credential>>

Init ==
  /\ phase = "request_received"
  /\ caller = "none"
  /\ requestedInstance = "none"
  /\ component = "none"
  /\ operation = "none"
  /\ credentialSource = "none"
  /\ credential = NoCredential

AuthenticateCaller ==
  /\ phase = "request_received"
  /\ \E user \in Users :
       /\ caller' = user
       /\ phase' = "identity_resolved"
  /\ UNCHANGED <<requestedInstance, component, operation,
                 credentialSource, credential>>

SelectInstance ==
  /\ phase = "identity_resolved"
  /\ \E instance \in Instances :
       /\ requestedInstance' = instance
       /\ phase' = "instance_selected"
  /\ UNCHANGED <<caller, component, operation,
                 credentialSource, credential>>

AuthorizeInstanceAndProfiles ==
  /\ phase = "instance_selected"
  /\ IF InstanceOwner[requestedInstance] = caller
       /\ ProfileAllowed[caller][requestedInstance]
     THEN phase' = "snapshot_resolution"
     ELSE phase' = "instance_rejected"
  /\ UNCHANGED <<caller, requestedInstance, component, operation,
                 credentialSource, credential>>

ResolveSnapshotAndOperation ==
  /\ phase = "snapshot_resolution"
  /\ \E tool \in Tools :
       /\ component' = Component
       /\ operation' = tool
       /\ phase' = "operation_authorization"
  /\ UNCHANGED <<caller, requestedInstance,
                 credentialSource, credential>>

AuthorizeOperation ==
  /\ phase = "operation_authorization"
  /\ IF ToolAllowed[caller][requestedInstance][operation]
     THEN phase' = "credential_resolution"
     ELSE phase' = "tool_rejected"
  /\ UNCHANGED <<caller, requestedInstance, component, operation,
                 credentialSource, credential>>

ResolveExactCredential ==
  /\ phase = "credential_resolution"
  /\ LET expected == Credentials[requestedInstance] IN
       IF expected.valid /\ expected.owner = caller
         THEN /\ credential' = expected
              /\ credentialSource' = requestedInstance
              /\ phase' = "credential_ready"
         ELSE /\ credential' = NoCredential
              /\ credentialSource' = requestedInstance
              /\ phase' = "business_authorization_required"
  /\ UNCHANGED <<caller, requestedInstance, component, operation>>

(* Broken variant used only by Broken.cfg: when an exact credential is not  *)
(* available, the resolver searches another instance's valid credential.    *)
ResolveCredentialWithFallback ==
  /\ phase = "credential_resolution"
  /\ \E source \in Instances :
       /\ Credentials[source].valid
       /\ credential' = Credentials[source]
       /\ credentialSource' = source
       /\ phase' = "credential_ready"
  /\ UNCHANGED <<caller, requestedInstance, component, operation>>

ResolveCredential ==
  IF FallbackEnabled
    THEN ResolveCredentialWithFallback
    ELSE ResolveExactCredential

DispatchToWorker ==
  /\ phase = "credential_ready"
  /\ phase' = "dispatched"
  /\ UNCHANGED <<caller, requestedInstance, component, operation,
                 credentialSource, credential>>

TerminalStates ==
  {"dispatched", "business_authorization_required",
   "instance_rejected", "tool_rejected"}

Done ==
  phase \in TerminalStates /\ UNCHANGED vars

Next ==
  AuthenticateCaller
  \/ SelectInstance
  \/ AuthorizeInstanceAndProfiles
  \/ ResolveSnapshotAndOperation
  \/ AuthorizeOperation
  \/ ResolveCredential
  \/ DispatchToWorker
  \/ Done

Spec == Init /\ [][Next]_vars

Phases ==
  {"request_received", "identity_resolved", "instance_selected",
   "snapshot_resolution", "operation_authorization", "credential_resolution",
   "credential_ready", "dispatched", "business_authorization_required",
   "instance_rejected", "tool_rejected"}

TypeOK ==
  /\ phase \in Phases
  /\ caller \in Users \union {"none"}
  /\ requestedInstance \in Instances \union {"none"}
  /\ component \in {Component} \union {"none"}
  /\ operation \in Tools \union {"none"}
  /\ credentialSource \in Instances \union {"none"}
  /\ credential = NoCredential
     \/ \E instance \in Instances :
          credential = Credentials[instance]

NoDeadlock == ENABLED Next

(* A dispatched request must use the valid credential stored under the      *)
(* requested instance's exact context, and that credential must belong to   *)
(* the authenticated caller and instance owner.                            *)
ExactCallerCredentialOnly ==
  phase /= "dispatched"
  \/ /\ credential.valid
     /\ credential.owner = caller
     /\ credential = Credentials[requestedInstance]
     /\ credentialSource = requestedInstance
     /\ InstanceOwner[requestedInstance] = caller

=============================================================================
