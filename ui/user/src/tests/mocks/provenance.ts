// Fixtures for the provenance (Supervision) API, shared by the supervision route specs.
import type {
	Approval,
	Kill,
	Narrative,
	Overview,
	Policy,
	PolicyDraft,
	ProvenanceEvent,
	RedTeamResult,
	Session,
	SimulationResult
} from '$lib/services/provenance';

const now = Date.now();
const minutesAgo = (n: number) => new Date(now - n * 60_000).toISOString();
const minutesFromNow = (n: number) => new Date(now + n * 60_000).toISOString();

export const riskySession: Session = {
	id: 'ses-risky',
	userID: 'u-alice',
	email: 'alice@example.com',
	agent: 'claude_code',
	actor: { agent: 'claude_code', agentVersion: '1.2.3', clientSessionID: 'cc-1' },
	title: 'Triage GitHub issue #4411',
	startedAt: minutesAgo(40),
	lastActivityAt: minutesAgo(2),
	state: {
		labels: ['untrusted', 'pii', 'confidential'],
		origins: {
			untrusted: [{ eventID: 'ev-2', summary: 'github.search_issues', at: minutesAgo(35) }],
			pii: [{ eventID: 'ev-4', summary: 'salesforce.query', at: minutesAgo(20) }],
			confidential: [{ eventID: 'ev-4', summary: 'salesforce.query', at: minutesAgo(20) }]
		},
		risk: 72,
		events: 7,
		toolCalls: 3,
		denied: 1,
		shadowed: 1,
		approvals: 1,
		externalOut: 1,
		surfaces: ['llm', 'mcp', 'egress'],
		warrant: {
			id: 'war-1',
			sessionID: 'ses-risky',
			task: 'Triage the flaky test in obot-platform/obot',
			scopes: ['mcp:github:*', 'egress:api.github.com'],
			issuedAt: minutesAgo(38),
			expiresAt: minutesFromNow(40),
			drifted: true,
			driftSummary: 'tools/call salesforce.query'
		},
		lastSummary: 'POST paste.example'
	}
};

export const quietSession: Session = {
	id: 'ses-quiet',
	userID: 'u-bob',
	email: 'bob@example.com',
	agent: 'cursor',
	actor: { agent: 'cursor' },
	title: 'Refactor billing module',
	startedAt: minutesAgo(90),
	lastActivityAt: minutesAgo(60),
	endedAt: minutesAgo(60),
	state: {
		labels: ['source-code'],
		risk: 15,
		events: 12,
		toolCalls: 8,
		denied: 0,
		shadowed: 0,
		approvals: 0,
		externalOut: 0,
		surfaces: ['device'],
		lastSummary: 'file.write src/billing.ts'
	}
};

export const sessions: Session[] = [riskySession, quietSession];

export const riskyEvents: ProvenanceEvent[] = [
	{
		id: 'ev-1',
		sessionID: 'ses-risky',
		surface: 'llm',
		kind: 'llm.turn',
		at: minutesAgo(38),
		userID: 'u-alice',
		actor: { agent: 'claude_code' },
		summary: 'anthropic/claude-sonnet-4',
		action: {
			surface: 'llm',
			kind: 'llm.turn',
			model: 'claude-sonnet-4',
			provider: 'anthropic',
			sink: { external: false, writes: false, exec: false },
			source: { private: false, untrusted: false }
		},
		decision: { allow: true, effect: 'allow', reason: 'no policy matched', risk: 0 }
	},
	{
		id: 'ev-2',
		sessionID: 'ses-risky',
		surface: 'mcp',
		kind: 'tool.call',
		at: minutesAgo(35),
		userID: 'u-alice',
		actor: { agent: 'claude_code' },
		parentID: 'ev-1',
		summary: 'tools/call github.search_issues',
		action: {
			surface: 'mcp',
			kind: 'tool.call',
			tool: 'search_issues',
			server: 'github',
			capabilities: ['reads-untrusted'],
			sink: { external: false, writes: false, exec: false },
			source: { private: false, untrusted: true }
		},
		decision: { allow: true, effect: 'allow', reason: 'no policy matched', risk: 0 }
	},
	{
		id: 'ev-3',
		sessionID: 'ses-risky',
		surface: 'mcp',
		kind: 'tool.result',
		at: minutesAgo(34),
		userID: 'u-alice',
		actor: { agent: 'claude_code' },
		parentID: 'ev-2',
		summary: 'github.search_issues result',
		labels: ['untrusted'],
		bytes: 4210
	},
	{
		id: 'ev-4',
		sessionID: 'ses-risky',
		surface: 'mcp',
		kind: 'tool.call',
		at: minutesAgo(20),
		userID: 'u-alice',
		actor: { agent: 'claude_code' },
		parentID: 'ev-1',
		summary: 'tools/call salesforce.query',
		action: {
			surface: 'mcp',
			kind: 'tool.call',
			tool: 'query',
			server: 'salesforce',
			capabilities: ['reads-private'],
			sink: { external: false, writes: false, exec: false },
			source: { private: true, untrusted: false }
		},
		decision: {
			allow: true,
			effect: 'judge',
			policyID: 'builtin-warrant-drift',
			policyName: 'Outside the declared task',
			reason: 'the action is outside the warrant scopes; the Supervisor allowed it',
			shadow: true,
			risk: 30,
			matched: [
				{
					policyID: 'builtin-warrant-drift',
					policyName: 'Outside the declared task',
					effect: 'judge',
					shadow: true
				}
			]
		}
	},
	{
		id: 'ev-5',
		sessionID: 'ses-risky',
		surface: 'egress',
		kind: 'net.egress',
		at: minutesAgo(2),
		userID: 'u-alice',
		actor: { agent: 'claude_code' },
		parentID: 'ev-1',
		summary: 'POST paste.example',
		action: {
			surface: 'egress',
			kind: 'net.egress',
			host: 'paste.example',
			method: 'POST',
			sink: { external: true, destination: 'paste.example', writes: true, exec: false },
			source: { private: false, untrusted: false }
		},
		lineage: [
			{
				originEventID: 'ev-4',
				originSummary: 'salesforce.query',
				label: 'pii',
				count: 14,
				exact: true
			}
		],
		decision: {
			allow: false,
			effect: 'require_approval',
			policyID: 'builtin-pii-lineage',
			policyName: 'PII lineage to external destination',
			reason:
				'session read github issue #4411 (untrusted) and 14 PII records from salesforce.query, and now wants to POST to paste.example',
			approvalID: 'apr-1',
			risk: 72,
			labels: ['untrusted', 'pii', 'confidential'],
			matched: [
				{
					policyID: 'builtin-pii-lineage',
					policyName: 'PII lineage to external destination',
					effect: 'require_approval',
					shadow: false
				},
				{
					policyID: 'builtin-lethal-trifecta',
					policyName: 'Lethal trifecta',
					effect: 'require_approval',
					shadow: true
				}
			]
		}
	}
];

export const narrative: Narrative = {
	summary:
		'The session read an untrusted GitHub issue, then queried Salesforce for customer records, and tried to post them to paste.example.',
	timeline: [
		'Read github issue #4411 (untrusted)',
		'Queried salesforce (14 PII records)',
		'Attempted POST to paste.example, held for approval'
	],
	concerns: ['PII lineage reaches an external host', 'Action is outside the declared task'],
	model: 'claude-sonnet-4'
};

export const policies: Policy[] = [
	{
		id: 'builtin-honeytoken',
		name: 'Honeytoken exfiltration',
		description: 'A planted canary is leaving the session.',
		condition: 'action.honeytoken',
		effect: 'deny',
		mode: 'enforce',
		priority: 1000,
		enabled: true,
		builtIn: true,
		createdAt: minutesAgo(10000),
		updatedAt: minutesAgo(10000)
	},
	{
		id: 'builtin-lethal-trifecta',
		name: 'Lethal trifecta',
		description: 'Untrusted content plus confidential data plus an external write.',
		condition: 'action.trifecta',
		effect: 'require_approval',
		mode: 'shadow',
		priority: 900,
		enabled: true,
		builtIn: true,
		createdAt: minutesAgo(10000),
		updatedAt: minutesAgo(10000)
	},
	{
		id: 'pol-custom',
		name: 'No pastes from CI',
		description: 'Egress to paste sites from environments.',
		condition: 'action.kind == "net.egress" && host_matches(action.host, ["*.paste.example"])',
		effect: 'deny',
		mode: 'shadow',
		priority: 100,
		surfaces: ['egress'],
		enabled: false,
		createdAt: minutesAgo(500),
		updatedAt: minutesAgo(100),
		source: 'Never let anything reach a paste site from CI.'
	}
];

export const pendingApproval: Approval = {
	id: 'apr-1',
	sessionID: 'ses-risky',
	eventID: 'ev-5',
	userID: 'u-alice',
	policyID: 'builtin-pii-lineage',
	policyName: 'PII lineage to external destination',
	action: {
		surface: 'egress',
		kind: 'net.egress',
		host: 'paste.example',
		method: 'POST',
		sink: { external: true, destination: 'paste.example', writes: true, exec: false },
		source: { private: false, untrusted: false }
	},
	reason:
		'session read github issue #4411 (untrusted) and 14 PII records from salesforce.query, and now wants to POST to paste.example',
	approvers: ['admin'],
	createdAt: minutesAgo(2),
	expiresAt: minutesFromNow(8),
	status: 'pending',
	triage: {
		recommendation: 'deny',
		rationale: 'PII from Salesforce is leaving to a paste site, which does not serve the task.',
		confidence: 0.92,
		model: 'claude-sonnet-4',
		at: minutesAgo(1)
	},
	risk: 72,
	labels: ['untrusted', 'pii', 'confidential']
};

export const approvedApproval: Approval = {
	...pendingApproval,
	id: 'apr-2',
	status: 'approved',
	resolvedBy: 'admin@example.com',
	resolvedAt: minutesAgo(30),
	note: 'Known test fixture',
	triage: undefined,
	createdAt: minutesAgo(45),
	expiresAt: minutesAgo(35)
};

export const kills: Kill[] = [
	{
		id: 'kill-1',
		scope: 'agent',
		target: 'rogue_agent',
		reason: 'Exfiltration attempt',
		createdBy: 'admin@example.com',
		createdAt: minutesAgo(5),
		expiresAt: minutesFromNow(55)
	}
];

export const overview: Overview = {
	coverage: {
		sessions: 12,
		surfaces: { llm: 9, mcp: 11, device: 4, egress: 3, copilot: 0 },
		blind: 2
	},
	decisions: { allowed: 140, denied: 3, shadowed: 11, approvals_pending: 1 },
	byEffect: { allow: 140, deny: 3, require_approval: 4, judge: 7 },
	bySurface: { llm: 60, mcp: 70, device: 15, egress: 9 },
	riskiest: [riskySession, quietSession],
	policies: { enforce: 2, shadow: 5 },
	supervisor: { enabled: true, model: 'claude-sonnet-4' }
};

export const simulation: SimulationResult = {
	policy: policies[2],
	since: minutesAgo(30 * 24 * 60),
	until: minutesAgo(0),
	examined: 154,
	matched: 3,
	byEffect: { deny: 3 },
	changed: 2,
	sessions: 2,
	users: 2,
	samples: [riskyEvents[4]],
	errors: 0,
	falsePositiveHint: 0.33
};

export const draft: PolicyDraft = {
	policy: {
		name: 'Customer data never reaches paste sites',
		description: 'Drafted by the Supervisor',
		condition: '"customer-data" in action.lineage.labels && action.sink.external',
		effect: 'deny',
		mode: 'shadow',
		priority: 500,
		enabled: true,
		source: 'Never let customer data reach a paste site'
	},
	explanation: 'Matches outbound actions carrying customer data lineage to an external host.',
	tests: [
		{
			name: 'customer data to paste.example',
			expected: true,
			actual: true,
			summary: 'POST paste.example after salesforce.query'
		},
		{
			name: 'customer data to internal wiki',
			expected: false,
			actual: false,
			summary: 'POST wiki.corp.example'
		}
	]
};

export const redTeam: RedTeamResult = {
	ranAt: minutesAgo(0),
	scenarios: [
		{
			scenario: 'Prompt injection via issue comment',
			surface: 'mcp',
			observed: true,
			detected: true,
			blocked: true,
			remediated: true,
			notes: 'Held by PII lineage policy'
		},
		{
			scenario: 'Honeytoken in shell history',
			surface: 'device',
			observed: true,
			detected: true,
			blocked: false,
			remediated: false,
			notes: 'Device hook not enrolled'
		}
	]
};
