<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import { streamChatWithAI, type AIEvent, type AIMessage, type AIStep } from '$lib/services/ai';
	import { profile } from '$lib/stores';
	import {
		ArrowUp,
		Check,
		ChevronRight,
		LoaderCircle,
		Plus,
		Sparkles,
		TriangleAlert,
		Wrench
	} from '@lucide/svelte';
	import { tick, untrack } from 'svelte';

	// Obot AI. The conversation lives in this tab's session storage so it survives navigating
	// away and back; the server keeps nothing.

	interface Turn extends AIMessage {
		steps?: AIStep[];
		error?: boolean;
	}

	// The answer being streamed: the tools it calls as they run, and its text so far.
	interface LiveStep {
		tool: string;
		arguments?: Record<string, unknown>;
		state: 'running' | 'done' | 'error';
	}
	interface Live {
		steps: LiveStep[];
		text: string;
	}

	const thinkingWords = [
		m.ai_thinking_1,
		m.ai_thinking_2,
		m.ai_thinking_3,
		m.ai_thinking_4,
		m.ai_thinking_5,
		m.ai_thinking_6
	];
	const toolLabels: Record<string, () => string> = {
		llm_usage: m.ai_tool_llm_usage,
		adoption_summary: m.ai_tool_adoption_summary,
		ask_supervisor: m.ai_tool_ask_supervisor,
		list_claude_sessions: m.ai_tool_list_claude_sessions,
		search_mcp_servers: m.ai_tool_search_mcp_servers,
		list_mcp_server_requests: m.ai_tool_list_mcp_server_requests,
		get_mcp_server_request: m.ai_tool_get_mcp_server_request,
		list_shared_vmcps: m.ai_tool_list_shared_vmcps,
		approve_mcp_server_request: m.ai_tool_approve_mcp_server_request,
		deny_mcp_server_request: m.ai_tool_deny_mcp_server_request,
		request_mcp_server: m.ai_tool_request_mcp_server,
		list_my_mcp_server_requests: m.ai_tool_list_my_mcp_server_requests
	};
	function toolLabel(tool: string) {
		return toolLabels[tool]?.() ?? m.ai_tool_other({ tool });
	}

	const STORAGE_KEY = 'obot-ai-conversation';

	let isAdmin = $derived(!!profile.current.isAdmin?.());
	let turns = $state<Turn[]>(untrack(load));
	let input = $state('');
	let busy = $state(false);
	let scroller = $state<HTMLElement>();
	let live = $state<Live>();
	let thinkingIndex = $state(0);

	// Rotate the stand-in words while the model works and nothing new is on screen.
	$effect(() => {
		if (!busy) return;
		thinkingIndex = Math.floor(Math.random() * thinkingWords.length);
		const timer = setInterval(() => {
			thinkingIndex = (thinkingIndex + 1) % thinkingWords.length;
		}, 1800);
		return () => clearInterval(timer);
	});

	let waiting = $derived(
		!!live && !live.text && live.steps.every((step) => step.state !== 'running')
	);

	let suggestions = $derived(
		isAdmin
			? [
					m.ai_suggestion_briefing(),
					m.ai_suggestion_cost(),
					m.ai_suggestion_security(),
					m.ai_suggestion_requests()
				]
			: [m.ai_suggestion_find(), m.ai_suggestion_my_requests()]
	);

	function load(): Turn[] {
		try {
			return JSON.parse(sessionStorage.getItem(STORAGE_KEY) ?? '[]') as Turn[];
		} catch {
			return [];
		}
	}

	$effect(() => {
		const snapshot = JSON.stringify(turns);
		try {
			sessionStorage.setItem(STORAGE_KEY, snapshot);
		} catch {
			// Storage is a convenience only.
		}
	});

	async function scrollToEnd() {
		await tick();
		scroller?.scrollTo({ top: scroller.scrollHeight, behavior: 'smooth' });
	}

	function onEvent(event: AIEvent) {
		if (!live) return;
		switch (event.type) {
			case 'thinking':
				// A new model turn: text from an earlier turn was a preamble to tool calls.
				live.text = '';
				break;
			case 'tool_start':
				live.text = '';
				live.steps.push({ tool: event.tool, arguments: event.arguments, state: 'running' });
				break;
			case 'tool_end': {
				const step = live.steps.findLast((s) => s.tool === event.tool && s.state === 'running');
				if (step) step.state = event.isError ? 'error' : 'done';
				break;
			}
			case 'delta':
				live.text += event.text;
				break;
		}
		scrollToEnd();
	}

	async function send(text = input) {
		const question = text.trim();
		if (!question || busy) return;
		input = '';
		turns = [...turns, { role: 'user', content: question }];
		busy = true;
		live = { steps: [], text: '' };
		scrollToEnd();
		try {
			const history: AIMessage[] = turns
				.filter((t) => !t.error)
				.map(({ role, content }) => ({ role, content }));
			const reply = await streamChatWithAI(history, onEvent);
			turns = [...turns, { role: 'assistant', content: reply.reply, steps: reply.steps }];
		} catch (e) {
			turns = [
				...turns,
				{
					role: 'assistant',
					content: m.ai_error({ error: parseErrorContent(e).message }),
					error: true
				}
			];
		} finally {
			busy = false;
			live = undefined;
			scrollToEnd();
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
			e.preventDefault();
			send();
		}
	}

	function formatArguments(args?: Record<string, unknown>) {
		if (!args || Object.keys(args).length === 0) return '';
		return JSON.stringify(args);
	}
</script>

<svelte:head>
	<title>Obot | {m.ai_title()}</title>
</svelte:head>

<Layout title={m.ai_title()} rightNavActions={navActions}>
	<div class="flex h-full min-h-[60vh] flex-col gap-4 pb-4">
		<div bind:this={scroller} class="flex flex-1 flex-col gap-4 overflow-y-auto" aria-live="polite">
			{#if turns.length === 0}
				<div class="mt-8 flex flex-col items-center gap-4 text-center">
					<Sparkles class="text-primary size-12" />
					<p class="text-muted-content max-w-xl text-sm">
						{isAdmin ? m.ai_intro_admin() : m.ai_intro_user()}
					</p>
					<div class="flex max-w-2xl flex-wrap justify-center gap-2">
						{#each suggestions as suggestion (suggestion)}
							<button
								type="button"
								class="btn btn-secondary rounded-4xl text-left text-sm"
								onclick={() => send(suggestion)}
							>
								{suggestion}
							</button>
						{/each}
					</div>
				</div>
			{/if}

			{#each turns as turn, i (i)}
				{#if turn.role === 'user'}
					<div class="bg-primary/10 self-end rounded-2xl px-4 py-2 text-sm whitespace-pre-wrap">
						{turn.content}
					</div>
				{:else}
					<article
						class="dark:bg-base-200 bg-base-100 flex max-w-full flex-col gap-2 self-start rounded-2xl p-4 text-sm shadow-sm {turn.error
							? 'notification-error'
							: ''}"
					>
						{#if turn.steps?.length}
							<details class="text-muted-content text-xs">
								<summary class="flex cursor-pointer items-center gap-1">
									<Wrench class="size-3" />
									{m.ai_steps({ count: turn.steps.length })}
									<ChevronRight class="size-3" />
								</summary>
								<ul class="mt-2 flex flex-col gap-1">
									{#each turn.steps as step, j (j)}
										<li class="font-mono break-all">
											{step.tool}{formatArguments(step.arguments)}
											{#if step.isError}<span class="text-red-500">
													({m.ai_step_failed()})</span
												>{/if}
										</li>
									{/each}
								</ul>
							</details>
						{/if}
						<div class="milkdown-content">
							{@html toHTMLFromMarkdownWithNewTabLinks(turn.content)}
						</div>
					</article>
				{/if}
			{/each}

			{#if busy && live}
				<article
					class="dark:bg-base-200 bg-base-100 flex max-w-full flex-col gap-2 self-start rounded-2xl p-4 text-sm shadow-sm"
					aria-busy="true"
				>
					{#if live.steps.length}
						<ul
							class="text-muted-content flex flex-col gap-1 text-xs"
							aria-label={m.ai_steps({ count: live.steps.length })}
						>
							{#each live.steps as step, j (j)}
								<li class="flex items-center gap-2">
									{#if step.state === 'running'}
										<LoaderCircle class="text-primary size-3.5 animate-spin" />
									{:else if step.state === 'error'}
										<TriangleAlert class="size-3.5 text-red-500" />
									{:else}
										<Check class="size-3.5 text-green-600" />
									{/if}
									<span>{toolLabel(step.tool)}</span>
									<span class="sr-only">
										{step.state === 'running'
											? m.ai_step_running()
											: step.state === 'error'
												? m.ai_step_failed()
												: m.ai_step_done()}
									</span>
								</li>
							{/each}
						</ul>
					{/if}
					{#if live.text}
						<div class="milkdown-content">
							{@html toHTMLFromMarkdownWithNewTabLinks(live.text)}
						</div>
					{:else if waiting}
						<p class="text-muted-content animate-pulse" role="status">
							{thinkingWords[thinkingIndex]()}
						</p>
					{/if}
				</article>
			{/if}
		</div>

		<form
			class="dark:bg-base-200 bg-base-100 flex items-end gap-2 rounded-2xl p-2 shadow-sm"
			onsubmit={(e) => {
				e.preventDefault();
				send();
			}}
		>
			<textarea
				class="max-h-48 min-h-10 flex-1 resize-none bg-transparent px-2 py-2 text-sm outline-none"
				placeholder={m.ai_placeholder()}
				aria-label={m.ai_placeholder()}
				rows="1"
				bind:value={input}
				onkeydown={onKeydown}></textarea>
			<button
				type="submit"
				class="btn btn-primary size-10 rounded-full p-0"
				aria-label={m.ai_send()}
				disabled={busy || !input.trim()}
			>
				<ArrowUp class="size-4" />
			</button>
		</form>
	</div>
</Layout>

{#snippet navActions()}
	{#if turns.length > 0}
		<button
			type="button"
			class="btn btn-secondary rounded-4xl text-sm"
			disabled={busy}
			onclick={() => (turns = [])}
		>
			<Plus class="size-4" />
			{m.ai_new_chat()}
		</button>
	{/if}
{/snippet}
