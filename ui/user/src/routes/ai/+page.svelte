<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import { chatWithAI, type AIMessage, type AIStep } from '$lib/services/ai';
	import { profile } from '$lib/stores';
	import { ArrowUp, ChevronRight, Plus, Sparkles, Wrench } from '@lucide/svelte';
	import { tick, untrack } from 'svelte';

	// Obot AI. The conversation lives in this tab's session storage so it survives navigating
	// away and back; the server keeps nothing.

	interface Turn extends AIMessage {
		steps?: AIStep[];
		error?: boolean;
	}

	const STORAGE_KEY = 'obot-ai-conversation';

	let isAdmin = $derived(!!profile.current.isAdmin?.());
	let turns = $state<Turn[]>(untrack(load));
	let input = $state('');
	let busy = $state(false);
	let scroller = $state<HTMLElement>();

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

	async function send(text = input) {
		const question = text.trim();
		if (!question || busy) return;
		input = '';
		turns = [...turns, { role: 'user', content: question }];
		busy = true;
		scrollToEnd();
		try {
			const history: AIMessage[] = turns
				.filter((t) => !t.error)
				.map(({ role, content }) => ({ role, content }));
			const reply = await chatWithAI(history);
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

			{#if busy}
				<p class="text-muted-content animate-pulse self-start text-sm" role="status">
					{m.ai_thinking()}
				</p>
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
