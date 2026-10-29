// Client for Obot's provenance API: the cross-surface session graph, Obot Policy, approvals,
// the kill switch and the Supervisor. Shapes mirror pkg/provenance JSON. Actions are content-free:
// the API never returns tool arguments, and nothing here should render them.
import { m } from '$lib/i18n';
import { buildQueryString } from '$lib/url';
import { doDelete, doGet, doPatch, doPost, doPut, type Fetcher } from './http';

type Opts = { fetch?: Fetcher; signal?: AbortSignal; dontLogErrors?: boolean };

const B = '/provenance';

export type Surface = 'llm' | 'mcp' | 'device' | 'egress' | 'copilot';
export const SURFACES: Surface[] = ['llm', 'mcp', 'device', 'egress', 'copilot'];

export type Effect =
	| 'allow'
	| 'log'
	| 'redact'
	| 'downshift'
	| 'quarantine'
	| 'judge'
	| 'require_approval'
	| 'deny';
export const EFFECTS: Effect[] = [
	'allow',
	'log',
	'redact',
	'downshift',
	'quarantine',
	'judge',
	'require_approval',
	'deny'
];

export type PolicyMode = 'enforce' | 'shadow';

export type EventKind =
	| 'session.start'
	| 'session.end'
	| 'llm.turn'
	| 'tool.call'
	| 'tool.result'
	| 'file.read'
	| 'file.write'
	| 'shell.exec'
	| 'net.egress'
	| 'delegation'
	| 'approval'
	| 'policy.decision'
	| 'kill';

export type ApprovalStatus = 'pending' | 'approved' | 'denied' | 'expired';
export const APPROVAL_STATUSES: ApprovalStatus[] = ['pending', 'approved', 'denied', 'expired'];

export type KillScope = 'org' | 'user' | 'agent' | 'session';
export const KILL_SCOPES: KillScope[] = ['org', 'user', 'agent', 'session'];

export const CAPABILITY_LABELS = [
	'reads-private',
	'reads-untrusted',
	'writes-external',
	'writes-internal',
	'exec',
	'destructive',
	'credential-bearing'
];

export const CONTENT_LABELS = [
	'pii',
	'secret',
	'customer-data',
	'source-code',
	'financial',
	'health',
	'confidential',
	'untrusted',
	'honeytoken'
];

export interface Actor {
	agent?: string;
	agentVersion?: string;
	deviceID?: string;
	environmentID?: string;
	apiKeyID?: string;
	clientSessionID?: string;
	delegation?: string[];
}

export interface Sink {
	external: boolean;
	destination?: string;
	writes: boolean;
	exec: boolean;
}

export interface Source {
	private: boolean;
	untrusted: boolean;
}

/** The normalized action an event decided. Arguments and text are never serialized. */
export interface Action {
	surface: Surface;
	kind: EventKind;
	tool?: string;
	server?: string;
	serverID?: string;
	model?: string;
	provider?: string;
	host?: string;
	method?: string;
	path?: string;
	command?: string;
	file?: string;
	capabilities?: string[];
	sink: Sink;
	source: Source;
}

export interface LineageHit {
	originEventID: string;
	originSummary?: string;
	label: string;
	count: number;
	exact: boolean;
}

export interface PolicyMatch {
	policyID: string;
	policyName: string;
	effect: Effect;
	shadow: boolean;
}

export interface Decision {
	allow: boolean;
	effect: Effect;
	policyID?: string;
	policyName?: string;
	reason: string;
	shadow?: boolean;
	approvalID?: string;
	matched?: PolicyMatch[];
	lineage?: LineageHit[];
	labels?: string[];
	risk: number;
	killed?: boolean;
	honeytoken?: boolean;
	/** Set when the Supervisor decided the action itself (a judge effect it resolved). */
	judged?: boolean;
}

export interface SurfaceRef {
	mcpAuditLogID?: number;
	llmAuditLogID?: string;
	enforcementDecisionID?: number;
	egressAuditLogID?: number;
	localToolCallID?: number;
}

export interface ProvenanceEvent {
	id: string;
	sessionID: string;
	surface: Surface;
	kind: EventKind;
	at: string;
	userID: string;
	actor: Actor;
	parentID?: string;
	action?: Action;
	labels?: string[];
	lineage?: LineageHit[];
	decision?: Decision;
	ref?: SurfaceRef;
	summary?: string;
	bytes?: number;
}

export interface LabelOrigin {
	eventID: string;
	summary?: string;
	at: string;
}

export interface Warrant {
	id: string;
	sessionID: string;
	task: string;
	scopes: string[];
	delegation?: string[];
	issuedAt: string;
	expiresAt: string;
	drifted?: boolean;
	driftSummary?: string;
}

export interface SessionState {
	labels?: string[];
	origins?: Record<string, LabelOrigin[]>;
	risk: number;
	events: number;
	toolCalls: number;
	denied: number;
	shadowed: number;
	approvals: number;
	externalOut: number;
	surfaces?: Surface[];
	warrant?: Warrant;
	killed?: boolean;
	quarantined?: boolean;
	downshifted?: boolean;
	lastSummary?: string;
}

export interface Session {
	id: string;
	userID?: string;
	email?: string;
	agent?: string;
	actor: Actor;
	title?: string;
	startedAt: string;
	lastActivityAt: string;
	endedAt?: string;
	state: SessionState;
}

export interface Policy {
	id?: string;
	name: string;
	description?: string;
	condition: string;
	effect: Effect;
	mode: PolicyMode;
	priority: number;
	surfaces?: Surface[];
	enabled: boolean;
	guidance?: string;
	approvers?: string[];
	/** Go time.Duration: nanoseconds. See {@link approvalTTLSeconds}. */
	approvalTTL?: number;
	builtIn?: boolean;
	createdAt?: string;
	updatedAt?: string;
	source?: string;
}

export interface Narrative {
	summary: string;
	timeline?: string[];
	concerns?: string[];
	model?: string;
}

export interface Triage {
	recommendation: 'approve' | 'deny' | 'escalate' | string;
	rationale: string;
	confidence: number;
	model?: string;
	at: string;
}

export interface Approval {
	id: string;
	sessionID: string;
	eventID: string;
	userID?: string;
	policyID: string;
	policyName: string;
	action: Action;
	reason: string;
	approvers?: string[];
	createdAt: string;
	expiresAt: string;
	status: ApprovalStatus;
	resolvedBy?: string;
	resolvedAt?: string;
	note?: string;
	triage?: Triage;
	risk: number;
	labels?: string[];
}

export interface Kill {
	id: string;
	scope: KillScope;
	target?: string;
	reason?: string;
	createdBy?: string;
	createdAt: string;
	expiresAt?: string;
}

export interface Honeytoken {
	id: string;
	kind: string;
	name: string;
	hint?: string;
	createdAt: string;
	tripped: number;
	lastTrip?: string;
	createdBy?: string;
}

export interface ToolLabel {
	server: string;
	tool: string;
	labels: string[];
	source: string;
	rationale?: string;
	updatedAt: string;
}

export interface ProvenanceSettings {
	approvedDestinations?: string[];
	trustedHosts?: string[];
	supervisorEnabled: boolean;
	supervisorModel?: string;
	supervisorBudgetSeconds?: number;
	defaultApprovalTTLSeconds?: number;
	retentionDays?: number;
}

export interface Coverage {
	sessions: number;
	surfaces: Partial<Record<Surface, number>>;
	blind: number;
}

export interface Overview {
	coverage: Coverage;
	decisions: { allowed: number; denied: number; shadowed: number; approvals_pending: number };
	byEffect: Partial<Record<Effect, number>>;
	bySurface: Partial<Record<Surface, number>>;
	riskiest: Session[];
	policies: { enforce: number; shadow: number };
	supervisor: { enabled: boolean; model?: string };
}

export interface SimulationResult {
	policy: Policy;
	since: string;
	until: string;
	examined: number;
	matched: number;
	byEffect: Partial<Record<Effect, number>>;
	changed: number;
	sessions: number;
	users: number;
	samples: ProvenanceEvent[] | null;
	errors: number;
	falsePositiveHint: number;
}

export interface PolicyTest {
	name: string;
	expected: boolean;
	actual: boolean;
	error?: string;
	summary: string;
}

export interface PolicyDraft {
	policy: Policy;
	explanation: string;
	tests?: PolicyTest[];
}

export interface RedTeamScenario {
	scenario: string;
	surface: Surface;
	observed: boolean;
	detected: boolean;
	blocked: boolean;
	remediated: boolean;
	notes?: string;
}

export interface RedTeamResult {
	ranAt?: string;
	scenarios: RedTeamScenario[];
}

export interface ListResponse<T> {
	items: T[] | null;
	total: number;
	limit?: number;
	offset?: number;
}

export interface SessionFilters {
	userId?: string;
	agent?: string;
	surface?: Surface | '';
	minRisk?: number;
	label?: string;
	open?: boolean;
	killed?: boolean;
	query?: string;
	startTime?: string;
	endTime?: string;
	limit?: number;
	offset?: number;
	sortBy?: 'last_activity_at' | 'started_at' | 'risk';
	sortOrder?: 'asc' | 'desc';
}

export interface EventFilters {
	userId?: string;
	surface?: Surface;
	kind?: EventKind;
	denied?: boolean;
	startTime?: string;
	endTime?: string;
	limit?: number;
	offset?: number;
}

export interface ApprovalFilters {
	status?: ApprovalStatus;
	userId?: string;
	sessionId?: string;
	limit?: number;
	offset?: number;
}

function query(filters: Record<string, string | number | boolean | string[] | undefined | null>) {
	const qs = buildQueryString(filters);
	return qs ? `?${qs}` : '';
}

function enc(id: string) {
	return encodeURIComponent(id);
}

// Sessions

export async function listSessions(
	filters: SessionFilters = {},
	opts?: Opts
): Promise<ListResponse<Session>> {
	const resp = (await doGet(
		`${B}/sessions${query({ ...filters })}`,
		opts
	)) as ListResponse<Session>;
	return { ...resp, items: resp.items ?? [] };
}

export async function getSession(id: string, opts?: Opts): Promise<Session> {
	return (await doGet(`${B}/sessions/${enc(id)}`, opts)) as Session;
}

export async function listSessionEvents(
	id: string,
	params: { limit?: number; offset?: number; ascending?: boolean } = {},
	opts?: Opts
): Promise<ListResponse<ProvenanceEvent>> {
	const resp = (await doGet(
		`${B}/sessions/${enc(id)}/events${query({ ascending: true, ...params })}`,
		opts
	)) as ListResponse<ProvenanceEvent>;
	return { ...resp, items: resp.items ?? [] };
}

export async function getSessionNarrative(id: string, opts?: Opts): Promise<Narrative> {
	return (await doGet(`${B}/sessions/${enc(id)}/narrative`, opts)) as Narrative;
}

export async function issueWarrant(
	id: string,
	body: { task: string; scopes: string[]; ttlSeconds?: number; delegation?: string[] },
	opts?: Opts
): Promise<Warrant> {
	return (await doPost(`${B}/sessions/${enc(id)}/warrant`, body, opts)) as Warrant;
}

export async function endSession(id: string, opts?: Opts): Promise<void> {
	await doPost(`${B}/sessions/${enc(id)}/end`, {}, opts);
}

export async function listEvents(
	filters: EventFilters = {},
	opts?: Opts
): Promise<ListResponse<ProvenanceEvent>> {
	const resp = (await doGet(
		`${B}/events${query({ ...filters })}`,
		opts
	)) as ListResponse<ProvenanceEvent>;
	return { ...resp, items: resp.items ?? [] };
}

// Policies

export async function listPolicies(opts?: Opts): Promise<Policy[]> {
	const resp = (await doGet(`${B}/policies`, opts)) as { items: Policy[] | null };
	return resp.items ?? [];
}

export async function getPolicy(id: string, opts?: Opts): Promise<Policy> {
	return (await doGet(`${B}/policies/${enc(id)}`, opts)) as Policy;
}

export async function createPolicy(policy: Policy, opts?: Opts): Promise<Policy> {
	return (await doPost(`${B}/policies`, policy, opts)) as Policy;
}

export async function updatePolicy(id: string, policy: Policy, opts?: Opts): Promise<Policy> {
	return (await doPut(`${B}/policies/${enc(id)}`, policy, opts)) as Policy;
}

export async function deletePolicy(id: string, opts?: Opts): Promise<void> {
	await doDelete(`${B}/policies/${enc(id)}`, opts);
}

export async function validatePolicyCondition(
	condition: string,
	opts?: Opts
): Promise<{ valid: boolean; error?: string }> {
	return (await doPost(`${B}/policies/validate`, { condition }, opts)) as {
		valid: boolean;
		error?: string;
	};
}

export async function simulatePolicy(
	body: { policy: Policy; sinceHours?: number; surfaces?: Surface[] },
	opts?: Opts
): Promise<SimulationResult> {
	const res = (await doPost(`${B}/policies/simulate`, body, opts)) as SimulationResult;
	return { ...res, samples: res.samples ?? [], byEffect: res.byEffect ?? {} };
}

export async function draftPolicy(text: string, opts?: Opts): Promise<PolicyDraft> {
	return (await doPost(`${B}/policies/draft`, { text }, opts)) as PolicyDraft;
}

// Approvals

export async function listApprovals(
	filters: ApprovalFilters = {},
	opts?: Opts
): Promise<ListResponse<Approval>> {
	const resp = (await doGet(
		`${B}/approvals${query({ ...filters })}`,
		opts
	)) as ListResponse<Approval>;
	return { ...resp, items: resp.items ?? [] };
}

export async function getApproval(id: string, opts?: Opts): Promise<Approval> {
	return (await doGet(`${B}/approvals/${enc(id)}`, opts)) as Approval;
}

export async function approveApproval(id: string, note = '', opts?: Opts): Promise<Approval> {
	return (await doPost(`${B}/approvals/${enc(id)}/approve`, { note }, opts)) as Approval;
}

export async function denyApproval(id: string, note = '', opts?: Opts): Promise<Approval> {
	return (await doPost(`${B}/approvals/${enc(id)}/deny`, { note }, opts)) as Approval;
}

// Kill switch

export async function listKills(opts?: Opts): Promise<Kill[]> {
	const resp = (await doGet(`${B}/kills`, opts)) as { items: Kill[] | null };
	return resp.items ?? [];
}

export async function createKill(
	body: { scope: KillScope; target?: string; reason: string; ttlSeconds?: number },
	opts?: Opts
): Promise<Kill> {
	return (await doPost(`${B}/kills`, body, opts)) as Kill;
}

export async function deleteKill(id: string, opts?: Opts): Promise<void> {
	await doDelete(`${B}/kills/${enc(id)}`, opts);
}

// Honeytokens, tool labels and settings

export async function listHoneytokens(opts?: Opts): Promise<Honeytoken[]> {
	const resp = (await doGet(`${B}/honeytokens`, opts)) as { items: Honeytoken[] | null };
	return resp.items ?? [];
}

export async function createHoneytoken(
	body: { kind: string; name: string; value?: string },
	opts?: Opts
): Promise<{ honeytoken: Honeytoken; value: string }> {
	return (await doPost(`${B}/honeytokens`, body, opts)) as {
		honeytoken: Honeytoken;
		value: string;
	};
}

export async function deleteHoneytoken(id: string, opts?: Opts): Promise<void> {
	await doDelete(`${B}/honeytokens/${enc(id)}`, opts);
}

export async function listToolLabels(opts?: Opts): Promise<ToolLabel[]> {
	const resp = (await doGet(`${B}/tool-labels`, opts)) as { items: ToolLabel[] | null };
	return resp.items ?? [];
}

export async function putToolLabel(
	body: { server: string; tool: string; labels: string[] },
	opts?: Opts
): Promise<ToolLabel> {
	return (await doPut(`${B}/tool-labels`, body, opts)) as ToolLabel;
}

export async function getProvenanceSettings(opts?: Opts): Promise<ProvenanceSettings> {
	return (await doGet(`${B}/settings`, opts)) as ProvenanceSettings;
}

export async function updateProvenanceSettings(
	settings: ProvenanceSettings,
	opts?: Opts
): Promise<ProvenanceSettings> {
	return (await doPut(`${B}/settings`, settings, opts)) as ProvenanceSettings;
}

// Overview and red team

export async function getOverview(hours = 24, opts?: Opts): Promise<Overview> {
	const o = (await doGet(`${B}/overview${query({ hours })}`, opts)) as Partial<Overview>;
	return {
		coverage: {
			sessions: o.coverage?.sessions ?? 0,
			blind: o.coverage?.blind ?? 0,
			surfaces: o.coverage?.surfaces ?? {}
		},
		decisions: {
			allowed: o.decisions?.allowed ?? 0,
			denied: o.decisions?.denied ?? 0,
			shadowed: o.decisions?.shadowed ?? 0,
			approvals_pending: o.decisions?.approvals_pending ?? 0
		},
		byEffect: o.byEffect ?? {},
		bySurface: o.bySurface ?? {},
		riskiest: o.riskiest ?? [],
		policies: { enforce: o.policies?.enforce ?? 0, shadow: o.policies?.shadow ?? 0 },
		supervisor: { enabled: o.supervisor?.enabled ?? false, model: o.supervisor?.model }
	};
}

export async function runRedTeam(opts?: Opts): Promise<RedTeamResult> {
	// The server's scorecard names each scenario `name` and stamps `runAt`.
	type ServerScenario = Partial<RedTeamScenario> & { name?: string };
	const res = (await doPost(`${B}/redteam/run`, {}, opts)) as {
		ranAt?: string;
		runAt?: string;
		scenarios?: ServerScenario[];
		items?: ServerScenario[];
	};
	const scenarios = (res.scenarios ?? res.items ?? []).map((s) => ({
		scenario: s.scenario ?? s.name ?? '',
		surface: s.surface ?? 'llm',
		observed: s.observed ?? false,
		detected: s.detected ?? false,
		blocked: s.blocked ?? false,
		remediated: s.remediated ?? false,
		notes: s.notes
	}));
	return { ranAt: res.ranAt ?? res.runAt, scenarios };
}

// The Supervisor: presence, proactive insights and questions

export interface SupervisorStatus {
	enabled: boolean;
	configured: boolean;
	model?: string;
	deciderModel?: string;
	watching: { sessions: number; openSessions: number; surfaces: Surface[] };
	lastDecisionAt: string | null;
	lastJudgementAt: string | null;
	today: {
		decided: number;
		judged: number;
		held: number;
		denied: number;
		allowed: number;
		shadowed: number;
		triaged: number;
	};
	pendingApprovals: number;
}

export type InsightKind =
	| 'shadow-policy'
	| 'open-risky-session'
	| 'honeytoken-tripped'
	| 'approvals-aging'
	| 'coverage-gap'
	| 'unlabeled-tools';

export type InsightSeverity = 'info' | 'warning' | 'critical';

/** A one-click follow-up the Supervisor proposes: an existing provenance API call. */
export interface InsightAction {
	label: string;
	method: string;
	path: string;
	body?: unknown;
}

export interface Insight {
	id: string;
	kind: InsightKind | string;
	severity: InsightSeverity;
	title: string;
	detail: string;
	sessionID?: string;
	policyID?: string;
	action?: InsightAction;
}

export interface SupervisorInsights {
	generatedAt: string;
	briefing: string;
	briefingModel?: string;
	items: Insight[];
}

export interface SupervisorAnswer {
	answer: string;
	model?: string;
	context?: { sessions: number; events: number };
}

export async function getSupervisorStatus(opts?: Opts): Promise<SupervisorStatus> {
	const s = (await doGet(`${B}/supervisor/status`, opts)) as Partial<SupervisorStatus>;
	return {
		enabled: s.enabled ?? false,
		configured: s.configured ?? false,
		model: s.model,
		deciderModel: s.deciderModel,
		watching: {
			sessions: s.watching?.sessions ?? 0,
			openSessions: s.watching?.openSessions ?? 0,
			surfaces: s.watching?.surfaces ?? []
		},
		lastDecisionAt: s.lastDecisionAt ?? null,
		lastJudgementAt: s.lastJudgementAt ?? null,
		today: {
			decided: s.today?.decided ?? 0,
			judged: s.today?.judged ?? 0,
			held: s.today?.held ?? 0,
			denied: s.today?.denied ?? 0,
			allowed: s.today?.allowed ?? 0,
			shadowed: s.today?.shadowed ?? 0,
			triaged: s.today?.triaged ?? 0
		},
		pendingApprovals: s.pendingApprovals ?? 0
	};
}

/** Pending approvals for the nav badge: from the Supervisor status, else from the approvals list. */
export async function getPendingApprovalCount(opts?: Opts): Promise<number> {
	try {
		return (await getSupervisorStatus(opts)).pendingApprovals;
	} catch {
		const resp = await listApprovals({ status: 'pending', limit: 1 }, opts);
		return resp.total ?? resp.items?.length ?? 0;
	}
}

export async function getSupervisorInsights(
	refresh = false,
	opts?: Opts
): Promise<SupervisorInsights> {
	const res = (await doGet(
		`${B}/supervisor/insights${refresh ? '?refresh=true' : ''}`,
		opts
	)) as Partial<SupervisorInsights>;
	return {
		generatedAt: res.generatedAt ?? '',
		briefing: res.briefing ?? '',
		briefingModel: res.briefingModel,
		items: res.items ?? []
	};
}

export async function askSupervisor(
	question: string,
	sessionID?: string,
	opts?: Opts
): Promise<SupervisorAnswer> {
	return (await doPost(
		`${B}/supervisor/ask`,
		sessionID ? { question, sessionID } : { question },
		opts
	)) as SupervisorAnswer;
}

/**
 * Performs an insight's one-click action. Only provenance API calls are allowed: the action
 * comes from the server, but the UI should never be steered into calling arbitrary endpoints.
 */
export async function performInsightAction(action: InsightAction, opts?: Opts): Promise<unknown> {
	const path = action.path.startsWith('/api/') ? action.path.slice(4) : action.path;
	if (!path.startsWith(`${B}/`) || path.includes('..')) {
		throw new Error(`unsupported action path: ${action.path}`);
	}
	const body = (action.body ?? {}) as object;
	switch (action.method.toUpperCase()) {
		case 'GET':
			return doGet(path, opts);
		case 'POST':
			return doPost(path, body, opts);
		case 'PUT':
			return doPut(path, body, opts);
		case 'PATCH':
			return doPatch(path, body, opts);
		case 'DELETE':
			return doDelete(path, opts);
		default:
			throw new Error(`unsupported action method: ${action.method}`);
	}
}

// Presentation helpers

export type RiskLevel = 'ok' | 'warning' | 'error';

/** Buckets a 0-100 risk score: under 30 is fine, under 60 warrants attention, the rest is high. */
export function riskLevel(risk: number): RiskLevel {
	if (risk < 30) return 'ok';
	if (risk < 60) return 'warning';
	return 'error';
}

export function riskClass(risk: number): string {
	switch (riskLevel(risk)) {
		case 'ok':
			return 'badge-success';
		case 'warning':
			return 'badge-warning';
		default:
			return 'badge-error';
	}
}

export function surfaceLabel(surface: string): string {
	switch (surface) {
		case 'llm':
			return m.supervision_surface_llm();
		case 'mcp':
			return m.supervision_surface_mcp();
		case 'device':
			return m.supervision_surface_device();
		case 'egress':
			return m.supervision_surface_egress();
		case 'copilot':
			return m.supervision_surface_copilot();
		default:
			return surface;
	}
}

export function effectLabel(effect: string): string {
	switch (effect) {
		case 'allow':
			return m.supervision_effect_allow();
		case 'log':
			return m.supervision_effect_log();
		case 'redact':
			return m.supervision_effect_redact();
		case 'downshift':
			return m.supervision_effect_downshift();
		case 'quarantine':
			return m.supervision_effect_quarantine();
		case 'judge':
			return m.supervision_effect_judge();
		case 'require_approval':
			return m.supervision_effect_require_approval();
		case 'deny':
			return m.supervision_effect_deny();
		default:
			return effect;
	}
}

/** DaisyUI badge color for an effect, by how restrictive it is. */
export function effectBadgeClass(effect: string): string {
	switch (effect) {
		case 'deny':
			return 'badge-error';
		case 'require_approval':
		case 'judge':
		case 'quarantine':
			return 'badge-warning';
		case 'allow':
			return 'badge-success';
		default:
			return 'badge-primary';
	}
}

export function modeLabel(mode: string): string {
	return mode === 'enforce' ? m.supervision_mode_enforce() : m.supervision_mode_shadow();
}

export function killScopeLabel(scope: string): string {
	switch (scope) {
		case 'org':
			return m.supervision_kill_scope_org();
		case 'user':
			return m.supervision_kill_scope_user();
		case 'agent':
			return m.supervision_kill_scope_agent();
		case 'session':
			return m.supervision_kill_scope_session();
		default:
			return scope;
	}
}

export function approvalStatusLabel(status: string): string {
	switch (status) {
		case 'pending':
			return m.supervision_approvals_tab_pending();
		case 'approved':
			return m.supervision_approvals_tab_approved();
		case 'denied':
			return m.supervision_approvals_tab_denied();
		case 'expired':
			return m.supervision_approvals_tab_expired();
		default:
			return status;
	}
}

/** A policy's approval TTL in seconds; the API carries a Go duration in nanoseconds. */
export function approvalTTLSeconds(policy: Pick<Policy, 'approvalTTL'>): number | undefined {
	if (!policy.approvalTTL) return undefined;
	return Math.round(policy.approvalTTL / 1e9);
}

export function approvalTTLFromSeconds(seconds: number | undefined): number | undefined {
	if (!seconds || seconds <= 0) return undefined;
	return Math.round(seconds * 1e9);
}

/** A short, content-free name for a session in lists: its title, then its last summary, then its ID. */
export function sessionDisplayName(session: Session): string {
	return session.title || session.state?.lastSummary || session.id;
}

/** Content-free description of an action: "tools/call github.search_issues", "POST paste.example". */
export function describeAction(action: Action | undefined): string {
	if (!action) return '';
	switch (action.kind) {
		case 'tool.call':
		case 'tool.result':
			return [action.server, action.tool].filter(Boolean).join('.') || action.kind;
		case 'net.egress':
			return [action.method, action.host].filter(Boolean).join(' ') || action.kind;
		case 'shell.exec':
			return action.kind;
		case 'file.read':
		case 'file.write':
			return action.file ? `${action.kind} ${action.file}` : action.kind;
		case 'llm.turn':
			return [action.provider, action.model].filter(Boolean).join('/') || action.kind;
		default:
			return action.kind;
	}
}

/** What the Supervisor contributed to a decision it judged. */
export interface Judgement {
	judged: boolean;
	rationale?: string;
	model?: string;
	confidence?: number;
}

// The engine appends the Supervisor's verdict to the reason as
// "Supervisor (<model>, confidence <n>%): <rationale>".
const JUDGEMENT_RE = /Supervisor \(([^,()]+), confidence (\d+)%\):\s*([\s\S]*)$/;

export function judgementOf(decision: Decision | null | undefined): Judgement {
	if (!decision) return { judged: false };
	const match = decision.reason ? JUDGEMENT_RE.exec(decision.reason) : null;
	if (!match) return { judged: !!decision.judged };
	return {
		judged: true,
		model: match[1].trim(),
		confidence: Number(match[2]),
		rationale: match[3].trim()
	};
}

export type VoiceTone = 'judged' | 'held' | 'blocked' | 'shadow' | 'acted' | 'allowed';

export interface SupervisorLine {
	text: string;
	tone: VoiceTone;
	judgement: Judgement;
}

/** The subject of a decision in the Supervisor's sentences: the event's content-free summary. */
export function eventSubject(event: Pick<ProvenanceEvent, 'summary' | 'action' | 'kind'>): string {
	return event.summary || describeAction(event.action) || event.kind;
}

/** A decision told by the Supervisor in the first person: "I held tools/call acme.send_report…". */
export function supervisorLine(event: ProvenanceEvent): SupervisorLine | undefined {
	const d = event.decision;
	if (!d) return undefined;
	const action = eventSubject(event);
	const policy = d.policyName || m.supervision_voice_a_policy();
	const effect = effectLabel(d.effect).toLowerCase();
	const judgement = judgementOf(d);
	const line = (text: string, tone: VoiceTone) => ({ text, tone, judgement });

	if (d.killed) return line(m.supervision_voice_killed({ action }), 'blocked');
	if (d.honeytoken && !d.allow) return line(m.supervision_voice_honeytoken({ action }), 'blocked');
	if (judgement.judged && !d.shadow) {
		if (d.effect === 'require_approval' || d.approvalID) {
			return line(m.supervision_voice_judged_hold({ action }), 'judged');
		}
		if (!d.allow) return line(m.supervision_voice_judged_deny({ action }), 'judged');
		return line(m.supervision_voice_judged_allow({ action }), 'judged');
	}
	if (d.shadow && d.effect !== 'allow') {
		return line(m.supervision_voice_shadow({ action, policy, effect }), 'shadow');
	}
	if (d.effect === 'require_approval' || d.approvalID) {
		return line(m.supervision_voice_held({ action, policy }), 'held');
	}
	if (!d.allow) return line(m.supervision_voice_blocked({ action, policy }), 'blocked');
	if (d.effect !== 'allow')
		return line(m.supervision_voice_effect({ action, policy, effect }), 'acted');
	return line(m.supervision_voice_allowed({ action }), 'allowed');
}

/** Whether a decision is one the Supervisor should speak up about, not a routine allow. */
export function isIntervention(event: ProvenanceEvent): boolean {
	const line = supervisorLine(event);
	return !!line && line.tone !== 'allowed';
}
