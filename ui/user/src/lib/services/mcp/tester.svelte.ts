import type { MCPCatalogServer } from '$lib/services/user/types';
import { LoggingTransport } from './logging-transport';
import { MCPTesterLog } from './tester-log.svelte';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import {
	StreamableHTTPClientTransport,
	StreamableHTTPError
} from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import {
	ErrorCode,
	McpError,
	type CallToolResult,
	type GetPromptResult,
	type Implementation,
	type Prompt,
	type PromptMessage,
	type ReadResourceResult,
	type Resource,
	type ResourceContents,
	type Tool
} from '@modelcontextprotocol/sdk/types.js';

export type TesterSection = 'chat' | 'tools' | 'prompts' | 'resources' | 'logs';
export type TesterCapabilitySection = Extract<TesterSection, 'tools' | 'prompts' | 'resources'>;
export type TesterStatus =
	| 'idle'
	| 'initializing'
	| 'ready'
	| 'access-denied'
	| 'unhealthy'
	| 'reauthentication-required'
	| 'setup-required'
	| 'error'
	| 'closed';
export type TesterWorkflowKind = 'chat' | 'direct';
export type DirectOperationStatus =
	| 'success'
	| 'mcp-error'
	| 'denied'
	| 'transport-error'
	| 'timeout'
	| 'cancelled';

export interface DirectOperationResult<T> {
	status: DirectOperationStatus;
	durationMs: number;
	value?: T;
	message?: string;
}

export interface StagedPromptContext {
	id: string;
	type: 'prompt';
	name: string;
	messages: PromptMessage[];
}

export interface StagedResourceContext {
	id: string;
	type: 'resource';
	name: string;
	contents: Array<{ uri: string; mimeType?: string; text: string }>;
}

export type StagedTesterContext = StagedPromptContext | StagedResourceContext;

export interface StageResult {
	ok: boolean;
	message?: string;
}

export function normalizeTesterSection(value: string | null | undefined): TesterSection {
	switch (value) {
		case 'tools':
		case 'prompts':
		case 'resources':
		case 'logs':
			return value;
		default:
			return 'chat';
	}
}

export interface TesterWorkflow {
	id: symbol;
	kind: TesterWorkflowKind;
	label: string;
	abort: AbortController;
}

export interface CapabilityCache<T = unknown> {
	loaded: boolean;
	loading: boolean;
	unsupported: boolean;
	items: T[];
	pages: unknown[];
	error?: string;
	errorStatus?: DirectOperationStatus;
}

export interface MCPPageCollection<T, TPage = unknown> {
	items: T[];
	pages: TPage[];
}

type ToolsPage = Awaited<ReturnType<Client['listTools']>>;
type PromptsPage = Awaited<ReturnType<Client['listPrompts']>>;
type ResourcesPage = Awaited<ReturnType<Client['listResources']>>;

export interface TesterCapabilityCaches {
	tools: CapabilityCache<Tool>;
	prompts: CapabilityCache<Prompt>;
	resources: CapabilityCache<Resource>;
}

export interface TesterInspectorStates {
	tools: {
		selectedName?: string;
		argumentsValue?: Record<string, unknown>;
		result?: DirectOperationResult<CallToolResult>;
	};
	prompts: {
		selectedName?: string;
		argumentsValue: Record<string, string>;
		result?: DirectOperationResult<GetPromptResult>;
		stageError?: string;
	};
	resources: {
		selectedURI?: string;
		result?: DirectOperationResult<ReadResourceResult>;
		stageError?: string;
	};
}

function emptyInspectorStates(): TesterInspectorStates {
	return {
		tools: {},
		prompts: { argumentsValue: {} },
		resources: {}
	};
}

const MAX_STAGED_CONTEXT_BYTES = 100 * 1024;

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

function classifyOperationError(
	error: unknown,
	signal?: AbortSignal
): Pick<DirectOperationResult<never>, 'status' | 'message'> {
	if (
		signal?.aborted ||
		(error instanceof DOMException && error.name === 'AbortError') ||
		(error instanceof Error && error.name === 'AbortError')
	) {
		return { status: 'cancelled', message: 'Cancelled by user' };
	}
	if (error instanceof McpError && error.code === ErrorCode.RequestTimeout) {
		return { status: 'timeout', message: 'The MCP operation timed out' };
	}
	if (error instanceof StreamableHTTPError && error.code === 403) {
		return { status: 'denied', message: 'Access to this MCP operation was denied' };
	}
	return { status: 'transport-error', message: errorMessage(error) };
}

function byteLength(value: string): number {
	return new TextEncoder().encode(value).byteLength;
}

export function isTextualMimeType(mimeType?: string): boolean {
	if (!mimeType) return true;
	const normalized = mimeType.toLowerCase().split(';', 1)[0];
	return (
		normalized.startsWith('text/') ||
		normalized === 'application/json' ||
		normalized.endsWith('+json') ||
		normalized === 'application/xml' ||
		normalized.endsWith('+xml') ||
		normalized === 'application/javascript' ||
		normalized === 'application/typescript' ||
		normalized === 'application/yaml' ||
		normalized === 'application/x-yaml'
	);
}

async function collectPages<T, TPage extends { nextCursor?: string }>(
	loadPage: (cursor?: string) => Promise<TPage>,
	selectItems: (page: TPage) => T[]
): Promise<MCPPageCollection<T, TPage>> {
	const items: T[] = [];
	const pages: TPage[] = [];
	let cursor: string | undefined;

	do {
		const page = await loadPage(cursor);
		pages.push(page);
		items.push(...selectItems(page));
		cursor = page.nextCursor || undefined;
	} while (cursor);

	return { items, pages };
}

function requestMessage(init?: RequestInit): {
	method?: string;
	params?: Record<string, unknown>;
} {
	if (typeof init?.body !== 'string') return {};

	try {
		const body = JSON.parse(init.body) as
			| { method?: string; params?: Record<string, unknown> }
			| Array<{ method?: string; params?: Record<string, unknown> }>;
		return Array.isArray(body) ? (body.find((message) => message.method) ?? {}) : body;
	} catch {
		return {};
	}
}

function withAuditMetadata(fetcher: typeof fetch): typeof fetch {
	return async (input, init) => {
		const message = requestMessage(init);
		if (!message.method) return fetcher(input, init);

		// This URL is request-local transport metadata, not reactive UI state.
		const url = new URL(
			typeof input === 'string' ? input : input instanceof URL ? input.href : input.url,
			typeof window === 'undefined' ? 'http://localhost' : window.location.origin
		);
		url.searchParams.set('method', message.method);
		if (message.method === 'tools/call' && typeof message.params?.name === 'string') {
			url.searchParams.set('toolcallname', message.params.name);
		}
		return fetcher(url, init);
	};
}

function emptyCache<T>(): CapabilityCache<T> {
	return {
		loaded: false,
		loading: false,
		unsupported: false,
		items: [],
		pages: []
	};
}

export class MCPTesterSession {
	client: Client;
	readonly server: MCPCatalogServer;
	readonly #clientInfo: Implementation;
	readonly #fetcher: typeof fetch;
	transport: LoggingTransport;
	readonly log = new MCPTesterLog();
	#connectionAttempted = false;
	status = $state<TesterStatus>('idle');
	error = $state<string>();
	activeWorkflow = $state<TesterWorkflow>();
	cache = $state<TesterCapabilityCaches>({
		tools: emptyCache(),
		prompts: emptyCache(),
		resources: emptyCache()
	});
	stagedContext = $state<StagedTesterContext[]>([]);
	inspectors = $state<TesterInspectorStates>(emptyInspectorStates());

	constructor(server: MCPCatalogServer, clientInfo: Implementation, fetcher: typeof fetch = fetch) {
		this.server = server;
		this.#clientInfo = clientInfo;
		this.#fetcher = fetcher;
		({ client: this.client, transport: this.transport } = this.#createConnection());
	}

	#createConnection(): { client: Client; transport: LoggingTransport } {
		const origin = typeof window === 'undefined' ? 'http://localhost' : window.location.origin;
		const client = new Client(this.#clientInfo, { capabilities: {} });
		// The endpoint is immutable connection configuration, not reactive UI state.
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const endpoint = new URL(`/mcp-connect/${encodeURIComponent(this.server.id)}`, origin);
		const transport = new StreamableHTTPClientTransport(endpoint, {
			fetch: withAuditMetadata(this.#fetcher),
			requestInit: { credentials: 'same-origin' }
		});
		return { client, transport: new LoggingTransport(transport, this.log) };
	}

	async #resetConnection(): Promise<void> {
		this.transport.detach();
		await this.client.close();
		this.log.clear();
		({ client: this.client, transport: this.transport } = this.#createConnection());
	}

	async initialize(force = false): Promise<void> {
		if (this.status === 'closed' || this.status === 'initializing') return;
		if (!this.server.configured) {
			this.status = this.server.missingOAuthCredentials
				? 'reauthentication-required'
				: 'setup-required';
			return;
		}
		if (
			!force &&
			['Unavailable', 'Needs Attention', 'Shutdown', 'Unknown'].includes(
				this.server.deploymentStatus ?? ''
			)
		) {
			this.status = 'unhealthy';
			return;
		}

		this.status = 'initializing';
		this.error = undefined;
		try {
			if (this.#connectionAttempted) {
				await this.#resetConnection();
			}
			this.#connectionAttempted = true;
			await this.client.connect(this.transport);
			if (!this.isClosed()) {
				this.status = 'ready';
			}
		} catch (error) {
			if (this.isClosed()) return;
			this.error = error instanceof Error ? error.message : String(error);
			if (error instanceof StreamableHTTPError) {
				switch (error.code) {
					case 401:
						this.status = 'reauthentication-required';
						return;
					case 403:
						this.status = 'access-denied';
						return;
					case 412:
					case 424:
					case 503:
						this.status = 'unhealthy';
						return;
				}
			}
			this.status = 'error';
		}
	}

	listTools(
		signal?: AbortSignal
	): Promise<MCPPageCollection<ToolsPage['tools'][number], ToolsPage>> {
		return collectPages(
			(cursor) => this.client.listTools(cursor ? { cursor } : {}, { signal }),
			(page) => page.tools
		);
	}

	listPrompts(
		signal?: AbortSignal
	): Promise<MCPPageCollection<PromptsPage['prompts'][number], PromptsPage>> {
		return collectPages(
			(cursor) => this.client.listPrompts(cursor ? { cursor } : {}, { signal }),
			(page) => page.prompts
		);
	}

	listResources(
		signal?: AbortSignal
	): Promise<MCPPageCollection<ResourcesPage['resources'][number], ResourcesPage>> {
		return collectPages(
			(cursor) => this.client.listResources(cursor ? { cursor } : {}, { signal }),
			(page) => page.resources
		);
	}

	isCapabilitySupported(section: TesterCapabilitySection): boolean {
		return Boolean(this.client.getServerCapabilities()?.[section]);
	}

	async loadTools(force = false): Promise<void> {
		await this.#loadCapability('tools', force, (signal) => this.listTools(signal));
	}

	async loadPrompts(force = false): Promise<void> {
		await this.#loadCapability('prompts', force, (signal) => this.listPrompts(signal));
	}

	async loadResources(force = false): Promise<void> {
		await this.#loadCapability('resources', force, (signal) => this.listResources(signal));
	}

	async #loadCapability<K extends keyof TesterCapabilityCaches>(
		section: K,
		force: boolean,
		load: (
			signal: AbortSignal
		) => Promise<MCPPageCollection<TesterCapabilityCaches[K]['items'][number]>>
	): Promise<void> {
		const cache = this.cache[section];
		if (cache.loading || (!force && cache.loaded)) return;
		if (this.activeWorkflow) return;
		if (!this.isCapabilitySupported(section)) {
			cache.loaded = true;
			cache.unsupported = true;
			cache.items = [];
			cache.pages = [];
			cache.error = undefined;
			cache.errorStatus = undefined;
			return;
		}

		const workflow = this.beginWorkflow('direct', `${section} discovery`);
		cache.loading = true;
		cache.unsupported = false;
		cache.error = undefined;
		cache.errorStatus = undefined;
		try {
			const result = await load(workflow.abort.signal);
			if (workflow.abort.signal.aborted) return;
			cache.items = result.items;
			cache.pages = result.pages;
			cache.loaded = true;
		} catch (error) {
			const classified = classifyOperationError(error, workflow.abort.signal);
			cache.error = classified.message;
			cache.errorStatus = classified.status;
			if (classified.status === 'denied') this.status = 'access-denied';
		} finally {
			cache.loading = false;
			this.finishWorkflow(workflow);
		}
	}

	async executeDirect<T>(
		label: string,
		operation: (signal: AbortSignal) => Promise<T>
	): Promise<DirectOperationResult<T>> {
		const workflow = this.beginWorkflow('direct', label);
		const started = performance.now();
		try {
			const value = await operation(workflow.abort.signal);
			const status =
				typeof value === 'object' && value !== null && 'isError' in value && value.isError === true
					? 'mcp-error'
					: 'success';
			return { status, durationMs: performance.now() - started, value };
		} catch (error) {
			const classified = classifyOperationError(error, workflow.abort.signal);
			if (classified.status === 'denied') this.status = 'access-denied';
			return { ...classified, durationMs: performance.now() - started };
		} finally {
			this.finishWorkflow(workflow);
		}
	}

	callTool(
		name: string,
		args: Record<string, unknown>
	): Promise<DirectOperationResult<CallToolResult>> {
		return this.executeDirect('tool call', async (signal) => {
			return (await this.client.callTool({ name, arguments: args }, undefined, {
				signal
			})) as CallToolResult;
		});
	}

	async snapshotChatTools(signal: AbortSignal): Promise<Tool[]> {
		if (!this.isCapabilitySupported('tools')) return [];
		try {
			return (await this.listTools(signal)).items;
		} catch (error) {
			const classified = classifyOperationError(error, signal);
			if (classified.status === 'denied') this.status = 'access-denied';
			throw new Error(classified.message, { cause: error });
		}
	}

	async callChatTool(
		name: string,
		args: Record<string, unknown>,
		signal: AbortSignal
	): Promise<DirectOperationResult<CallToolResult>> {
		const started = performance.now();
		try {
			const value = (await this.client.callTool({ name, arguments: args }, undefined, {
				signal
			})) as CallToolResult;
			return {
				status: value.isError === true ? 'mcp-error' : 'success',
				durationMs: performance.now() - started,
				value
			};
		} catch (error) {
			return {
				...classifyOperationError(error, signal),
				durationMs: performance.now() - started
			};
		}
	}

	getPrompt(
		name: string,
		args: Record<string, string>
	): Promise<DirectOperationResult<GetPromptResult>> {
		return this.executeDirect('prompt get', (signal) =>
			this.client.getPrompt({ name, arguments: args }, { signal })
		);
	}

	readResource(uri: string): Promise<DirectOperationResult<ReadResourceResult>> {
		return this.executeDirect('resource read', (signal) =>
			this.client.readResource({ uri }, { signal })
		);
	}

	stagePrompt(name: string, result: GetPromptResult): StageResult {
		if (result.messages.some((message) => message.content.type !== 'text')) {
			return {
				ok: false,
				message: 'This prompt contains content that the tester cannot send to Chat.'
			};
		}
		const size = result.messages.reduce(
			(total, message) =>
				total + byteLength(message.content.type === 'text' ? message.content.text : ''),
			0
		);
		if (size > MAX_STAGED_CONTEXT_BYTES) {
			return { ok: false, message: 'This prompt is larger than the 100 KiB staging limit.' };
		}
		this.stagedContext.push({
			id: crypto.randomUUID(),
			type: 'prompt',
			name,
			messages: result.messages
		});
		return { ok: true };
	}

	stageResource(name: string, result: ReadResourceResult): StageResult {
		const textContents = result.contents.filter(
			(content): content is ResourceContents & { text: string } =>
				'text' in content && isTextualMimeType(content.mimeType)
		);
		if (textContents.length !== result.contents.length) {
			return {
				ok: false,
				message: 'Only textual resource content can be staged for Chat.'
			};
		}
		const size = textContents.reduce((total, content) => total + byteLength(content.text), 0);
		if (size > MAX_STAGED_CONTEXT_BYTES) {
			return { ok: false, message: 'This resource is larger than the 100 KiB staging limit.' };
		}
		this.stagedContext.push({
			id: crypto.randomUUID(),
			type: 'resource',
			name,
			contents: textContents.map((content) => ({
				uri: content.uri,
				mimeType: content.mimeType,
				text: content.text
			}))
		});
		return { ok: true };
	}

	removeStagedContext(id: string): void {
		this.stagedContext = this.stagedContext.filter((context) => context.id !== id);
	}

	private isClosed(): boolean {
		return this.status === 'closed';
	}

	beginWorkflow(kind: TesterWorkflowKind, label: string): TesterWorkflow {
		if (this.status !== 'ready') {
			throw new Error('The MCP tester is not ready');
		}
		if (this.activeWorkflow) {
			throw new Error(`Cannot start ${label} while ${this.activeWorkflow.label} is active`);
		}
		const workflow: TesterWorkflow = {
			id: Symbol(label),
			kind,
			label,
			abort: new AbortController()
		};
		this.activeWorkflow = workflow;
		return workflow;
	}

	finishWorkflow(workflow: TesterWorkflow): void {
		if (this.activeWorkflow?.id === workflow.id) {
			this.activeWorkflow = undefined;
		}
	}

	cancelActiveWorkflow(): void {
		this.activeWorkflow?.abort.abort('Cancelled by user');
		this.activeWorkflow = undefined;
	}

	close(): void {
		if (this.status === 'closed') return;
		this.cancelActiveWorkflow();
		void this.client.close();
		this.cache = {
			tools: emptyCache(),
			prompts: emptyCache(),
			resources: emptyCache()
		};
		this.stagedContext = [];
		this.inspectors = emptyInspectorStates();
		this.error = undefined;
		this.status = 'closed';
	}
}
