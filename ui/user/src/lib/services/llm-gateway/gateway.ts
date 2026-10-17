// Client for Obot's LLM gateway API. Obot embeds Bifrost (https://github.com/maximhq/bifrost)
// and exposes its management API under /api/llm-gateway/admin, alongside Obot-specific
// endpoints for status, model sync, budgets and guardrails.
import { m } from '$lib/i18n';
import { doDelete, doGet, doPost, doPut, type Fetcher } from '../http';

type Opts = { fetch?: Fetcher; signal?: AbortSignal; dontLogErrors?: boolean };

const B = '/llm-gateway/admin';

// ---------- Obot endpoints ----------

export interface GatewayStatus {
	enabled: boolean;
	ready: boolean;
	error?: string;
	hasProviders: boolean;
	modelProviderName: string;
	proxyPath: string;
}

export async function getStatus(opts?: Opts): Promise<GatewayStatus> {
	return (await doGet('/llm-gateway/status', opts)) as GatewayStatus;
}

// Settings Obot enforces itself, rather than the engine's configuration.
export type LLMGatewaySettings = { subscriptionPassthrough: boolean };

export async function getSettings(opts?: Opts): Promise<LLMGatewaySettings> {
	return (await doGet('/llm-gateway/settings', opts)) as LLMGatewaySettings;
}

export async function updateSettings(settings: LLMGatewaySettings): Promise<LLMGatewaySettings> {
	return (await doPut('/llm-gateway/settings', settings)) as LLMGatewaySettings;
}

export async function syncModels(opts?: Opts): Promise<void> {
	await doPost('/llm-gateway/sync-models', {}, opts);
}

// ---------- Budgets (enforced by Obot) ----------

export type BudgetMetric = 'spend' | 'tokens' | 'input_tokens' | 'output_tokens' | 'requests';

export interface BudgetLimit {
	metric: BudgetMetric;
	amount: number;
	window: string;
	calendar?: boolean;
}

export interface Budget {
	id?: string;
	name: string;
	description?: string;
	enabled: boolean;
	perUser: boolean;
	exemptAdmins: boolean;
	// Users' personal daily token limits replace this per-user budget's token limits.
	userTokenLimits?: boolean;
	limits: BudgetLimit[];
	models?: string[];
	// The gateway profile the budget belongs to; it applies to that profile's users.
	profileID: string;
	created?: string;
	updated?: string;
}

export interface BudgetUsageEntry {
	userID?: string;
	used: number[];
}

// listBudgets lists the budgets of one profile, or every budget.
export async function listBudgets(opts?: Opts & { profileID?: string }): Promise<Budget[]> {
	const query = opts?.profileID ? `?profileID=${encodeURIComponent(opts.profileID)}` : '';
	const resp = (await doGet(`/llm-gateway/budgets${query}`, opts)) as { items: Budget[] };
	return resp.items ?? [];
}

export async function getBudget(id: string, opts?: Opts): Promise<Budget> {
	return (await doGet(`/llm-gateway/budgets/${encodeURIComponent(id)}`, opts)) as Budget;
}

export async function createBudget(budget: Budget, opts?: Opts): Promise<Budget> {
	return (await doPost('/llm-gateway/budgets', budget, opts)) as Budget;
}

export async function updateBudget(id: string, budget: Budget, opts?: Opts): Promise<Budget> {
	return (await doPut(`/llm-gateway/budgets/${encodeURIComponent(id)}`, budget, opts)) as Budget;
}

export async function deleteBudget(id: string, opts?: Opts): Promise<void> {
	await doDelete(`/llm-gateway/budgets/${encodeURIComponent(id)}`, opts);
}

export async function getBudgetUsage(id: string, opts?: Opts): Promise<BudgetUsageEntry[]> {
	const resp = (await doGet(`/llm-gateway/budgets/${encodeURIComponent(id)}/usage`, opts)) as {
		items: BudgetUsageEntry[];
	};
	return resp.items ?? [];
}

export const METRIC_LABELS: Record<BudgetMetric, string> = {
	spend: m.models_metric_spend(),
	tokens: m.models_metric_tokens(),
	input_tokens: m.models_metric_input_tokens(),
	output_tokens: m.models_metric_output_tokens(),
	requests: m.models_metric_requests()
};

// METRIC_SUMMARY_LABELS name metrics within a sentence, such as a budget's summary:
// "$50.00 spend per calendar month".
export const METRIC_SUMMARY_LABELS: Record<BudgetMetric, string> = {
	spend: m.models_metric_summary_spend(),
	tokens: m.models_metric_summary_tokens(),
	input_tokens: m.models_metric_summary_input_tokens(),
	output_tokens: m.models_metric_summary_output_tokens(),
	requests: m.models_metric_summary_requests()
};

export function formatMetric(metric: BudgetMetric, value: number): string {
	return metric === 'spend' ? formatCost(value) : formatNumber(value);
}

// Window units, by the suffix of a budget window such as "1d"; singular and plural.
export const WINDOW_UNIT_LABELS: Record<string, { one: string; other: string }> = {
	m: { one: m.models_window_unit_minute(), other: m.models_window_unit_minutes() },
	h: { one: m.models_window_unit_hour(), other: m.models_window_unit_hours() },
	d: { one: m.models_window_unit_day(), other: m.models_window_unit_days() },
	w: { one: m.models_window_unit_week(), other: m.models_window_unit_weeks() },
	M: { one: m.models_window_unit_month(), other: m.models_window_unit_months() }
};

export function windowLabel(limit: Pick<BudgetLimit, 'window' | 'calendar'>): string {
	const match = /^(\d+)([mhdwM])$/.exec(limit.window);
	if (!match) return limit.window;
	const n = Number(match[1]);
	const unit = WINDOW_UNIT_LABELS[match[2]];
	if (limit.calendar) return m.models_window_per_calendar({ unit: unit.one });
	return n === 1
		? m.models_window_per_rolling({ unit: unit.one })
		: m.models_window_per_rolling_count({ count: n, unit: unit.other });
}

// ---------- Providers & keys ----------

export interface NetworkConfig {
	base_url?: string;
	extra_headers?: Record<string, string>;
	default_request_timeout_in_seconds?: number;
	max_retries?: number;
	retry_backoff_initial?: number;
	retry_backoff_max?: number;
	stream_idle_timeout_in_seconds?: number;
	allow_private_network?: boolean;
	insecure_skip_verify?: boolean;
}

export interface ConcurrencyAndBufferSize {
	concurrency: number;
	buffer_size: number;
}

export interface CustomProviderConfig {
	is_key_less?: boolean;
	base_provider_type: string;
	// The request types the provider serves; unset serves every type its API format has.
	allowed_requests?: Record<string, boolean>;
}

export interface Provider {
	name: string;
	network_config: NetworkConfig;
	concurrency_and_buffer_size: ConcurrencyAndBufferSize;
	custom_provider_config?: CustomProviderConfig;
	send_back_raw_request?: boolean;
	send_back_raw_response?: boolean;
	provider_status?: string;
	status?: string;
	description?: string;
}

export interface SecretValue {
	value: string;
	type?: string;
	env_var?: string;
}

export interface ProviderKey {
	id?: string;
	name: string;
	value: SecretValue | string;
	models: string[];
	blacklisted_models?: string[];
	weight: number;
	enabled?: boolean;
	status?: string;
	description?: string;
	azure_key_config?: Record<string, unknown>;
	vertex_key_config?: Record<string, unknown>;
	bedrock_key_config?: Record<string, unknown>;
	vllm_key_config?: Record<string, unknown>;
	ollama_key_config?: Record<string, unknown>;
	sgl_key_config?: Record<string, unknown>;
	databricks_key_config?: Record<string, unknown>;
	use_for_batch_api?: boolean;
}

export async function listProviders(opts?: Opts): Promise<Provider[]> {
	const resp = (await doGet(`${B}/providers`, opts)) as { providers: Provider[] };
	return resp.providers ?? [];
}

export async function getProvider(name: string, opts?: Opts): Promise<Provider> {
	return (await doGet(`${B}/providers/${encodeURIComponent(name)}`, opts)) as Provider;
}

export async function createProvider(
	body: Partial<Provider> & { provider: string },
	opts?: Opts
): Promise<Provider> {
	return (await doPost(`${B}/providers`, body, opts)) as Provider;
}

// updateProvider saves a provider's settings. A change of base URL is refused while the
// provider has keys, so they aren't sent to the new address, unless removeKeys confirms the
// keys are removed with it.
export async function updateProvider(
	name: string,
	body: Partial<Provider>,
	{ removeKeys, ...opts }: Opts & { removeKeys?: boolean } = {}
): Promise<Provider> {
	const query = removeKeys ? '?removeKeys=true' : '';
	return (await doPut(
		`${B}/providers/${encodeURIComponent(name)}${query}`,
		body,
		opts
	)) as Provider;
}

export async function deleteProvider(name: string, opts?: Opts): Promise<void> {
	await doDelete(`${B}/providers/${encodeURIComponent(name)}`, opts);
}

export async function listProviderKeys(name: string, opts?: Opts): Promise<ProviderKey[]> {
	const resp = (await doGet(`${B}/providers/${encodeURIComponent(name)}/keys`, opts)) as {
		keys: ProviderKey[];
	};
	return resp.keys ?? [];
}

export async function createProviderKey(
	provider: string,
	key: ProviderKey,
	opts?: Opts
): Promise<ProviderKey> {
	return (await doPost(
		`${B}/providers/${encodeURIComponent(provider)}/keys`,
		key,
		opts
	)) as ProviderKey;
}

export async function updateProviderKey(
	provider: string,
	id: string,
	key: ProviderKey,
	opts?: Opts
): Promise<ProviderKey> {
	return (await doPut(
		`${B}/providers/${encodeURIComponent(provider)}/keys/${encodeURIComponent(id)}`,
		key,
		opts
	)) as ProviderKey;
}

export async function deleteProviderKey(provider: string, id: string, opts?: Opts): Promise<void> {
	await doDelete(
		`${B}/providers/${encodeURIComponent(provider)}/keys/${encodeURIComponent(id)}`,
		opts
	);
}

export async function refreshProviderModels(provider: string, opts?: Opts): Promise<void> {
	await doPost(`${B}/providers/${encodeURIComponent(provider)}/refresh-models`, {}, opts);
}

export interface GatewayModel {
	name: string;
	provider: string;
	is_deprecated?: boolean;
}

export async function listGatewayModels(
	params: { provider?: string; query?: string; limit?: number } = {},
	opts?: Opts
): Promise<GatewayModel[]> {
	const q = new URLSearchParams();
	if (params.provider) q.set('provider', params.provider);
	if (params.query) q.set('query', params.query);
	q.set('limit', String(params.limit ?? 500));
	const resp = (await doGet(`${B}/models?${q}`, opts)) as { models: GatewayModel[] };
	return resp.models ?? [];
}

// ---------- Routing rules ----------

export interface RoutingTarget {
	provider?: string | null;
	model?: string | null;
	key_id?: string | null;
	weight: number;
	// Streaming only: an attempt with no first token in this many milliseconds (1-300000)
	// is cut off and the next fallback runs. Omitted or 0 means no timeout.
	ttft_timeout_ms?: number | null;
}

export interface RoutingFallback {
	provider: string;
	model: string;
}

export interface RoutingRule {
	id?: string;
	name: string;
	description?: string;
	enabled?: boolean;
	cel_expression: string;
	targets: RoutingTarget[];
	// Bifrost returns unpinned fallbacks as "provider/model" strings.
	fallbacks?: (RoutingFallback | string)[];
	scope?: string;
	scope_id?: string | null;
	priority?: number;
	chain_rule?: boolean;
}

export async function listRoutingRules(opts?: Opts): Promise<RoutingRule[]> {
	const resp = (await doGet(`${B}/governance/routing-rules?limit=500`, opts)) as {
		rules: RoutingRule[] | null;
	};
	return resp.rules ?? [];
}

export async function createRoutingRule(rule: RoutingRule, opts?: Opts): Promise<RoutingRule> {
	const resp = (await doPost(`${B}/governance/routing-rules`, rule, opts)) as {
		rule: RoutingRule;
	};
	return resp.rule;
}

export async function updateRoutingRule(
	id: string,
	rule: Partial<RoutingRule>,
	opts?: Opts
): Promise<RoutingRule> {
	const resp = (await doPut(
		`${B}/governance/routing-rules/${encodeURIComponent(id)}`,
		rule,
		opts
	)) as { rule: RoutingRule };
	return resp.rule;
}

export async function deleteRoutingRule(id: string, opts?: Opts): Promise<void> {
	await doDelete(`${B}/governance/routing-rules/${encodeURIComponent(id)}`, opts);
}

export function parseFallback(f: RoutingFallback | string): RoutingFallback {
	if (typeof f !== 'string') return f;
	const i = f.indexOf('/');
	return i < 0 ? { provider: '', model: f } : { provider: f.slice(0, i), model: f.slice(i + 1) };
}

// ---------- Settings ----------

export interface ClientConfig {
	drop_excess_requests: boolean;
	max_request_body_size_mb: number;
	prometheus_labels: string[] | null;
	[key: string]: unknown;
}

export async function getConfig(opts?: Opts): Promise<{ client_config: ClientConfig }> {
	return (await doGet(`${B}/config`, opts)) as { client_config: ClientConfig };
}

export async function updateConfig(
	body: { client_config: Partial<ClientConfig> },
	opts?: Opts
): Promise<void> {
	await doPut(`${B}/config`, body, opts);
}

// ---------- Helpers ----------

export function formatCost(value?: number): string {
	if (!value) return '$0.00';
	if (value < 0.01) return `$${value.toFixed(6)}`;
	return `$${value.toFixed(2)}`;
}

export function formatNumber(value?: number): string {
	if (value === undefined || value === null) return '0';
	return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(
		value
	);
}

export function secretDisplay(value: SecretValue | string | undefined): string {
	if (!value) return '';
	if (typeof value === 'string') return value;
	return value.env_var ? `env.${value.env_var}` : value.value;
}
