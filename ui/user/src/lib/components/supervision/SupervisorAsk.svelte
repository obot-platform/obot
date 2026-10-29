<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { askSupervisor } from '$lib/services/provenance';
	import SupervisorAvatar from './SupervisorAvatar.svelte';
	import { MessageCircleQuestionMark, Send } from '@lucide/svelte';

	// Ask the Supervisor a question, optionally about one session. A plain request/response:
	// each exchange renders the question and the Supervisor's answer as messages.

	interface Props {
		sessionId?: string;
		suggestions?: string[];
		intro?: string;
		/** Render without the surrounding card, for embedding in another panel. */
		embedded?: boolean;
	}

	let { sessionId, suggestions = [], intro, embedded = false }: Props = $props();

	interface Exchange {
		id: number;
		question: string;
		answer?: string;
		model?: string;
		context?: { sessions: number; events: number };
		error?: string;
		pending: boolean;
	}

	let question = $state('');
	let exchanges = $state<Exchange[]>([]);
	let asking = $derived(exchanges.some((e) => e.pending));
	let nextID = 0;

	async function ask(q: string) {
		const text = q.trim();
		if (!text || asking) return;
		question = '';
		const id = ++nextID;
		exchanges = [...exchanges, { id, question: text, pending: true }];
		const update = (patch: Partial<Exchange>) => {
			exchanges = exchanges.map((e) => (e.id === id ? { ...e, ...patch, pending: false } : e));
		};
		try {
			const res = await askSupervisor(text, sessionId, { dontLogErrors: true });
			update({ answer: res.answer, model: res.model, context: res.context });
		} catch (e) {
			const parsed = parseErrorContent(e);
			update({
				error:
					parsed.status === 503 || parsed.status === 404
						? m.supervision_ask_unavailable()
						: parsed.message
			});
		}
	}

	function onsubmit(e: SubmitEvent) {
		e.preventDefault();
		ask(question);
	}
</script>

{#snippet body()}
	{#if intro}
		<p class="text-muted-content text-sm">{intro}</p>
	{/if}

	{#if exchanges.length}
		<ol class="flex flex-col gap-3" aria-label={m.supervision_ask_conversation()}>
			{#each exchanges as ex (ex.id)}
				<li class="flex flex-col gap-2">
					<div class="flex justify-end">
						<p
							class="bg-base-200 dark:bg-base-300 max-w-[85%] rounded-2xl rounded-br-sm px-3 py-2 text-sm"
						>
							<span class="sr-only">{m.supervision_ask_you()}:</span>
							{ex.question}
						</p>
					</div>
					<div class="flex items-start gap-2">
						<SupervisorAvatar size="sm" />
						{#if ex.pending}
							<p class="text-muted-content flex items-center gap-2 py-1 text-sm">
								<Loading class="size-4" />
								{m.supervision_ask_thinking()}
							</p>
						{:else if ex.error}
							<p class="bg-warning/10 rounded-2xl rounded-tl-sm px-3 py-2 text-sm" role="alert">
								{ex.error}
							</p>
						{:else}
							<div class="flex min-w-0 flex-col gap-1">
								<p
									class="bg-primary/5 rounded-2xl rounded-tl-sm px-3 py-2 text-sm leading-relaxed whitespace-pre-wrap"
								>
									{ex.answer}
								</p>
								<span class="text-muted-content px-1 text-xs">
									{#if ex.model}{m.supervision_ask_answered_by({ model: ex.model })}{/if}
									{#if ex.model && ex.context}
										·
									{/if}
									{#if ex.context}
										{m.supervision_ask_context({
											sessions: ex.context.sessions,
											events: ex.context.events
										})}
									{/if}
								</span>
							</div>
						{/if}
					</div>
				</li>
			{/each}
		</ol>
	{/if}

	{#if suggestions.length}
		<div class="flex flex-wrap gap-2">
			{#each suggestions as s (s)}
				<button
					type="button"
					class="btn btn-secondary btn-xs h-auto min-h-6 py-1 text-left text-xs font-normal"
					disabled={asking}
					onclick={() => ask(s)}
				>
					{s}
				</button>
			{/each}
		</div>
	{/if}

	<form class="flex items-center gap-2" {onsubmit}>
		<label for={`supervisor-ask-${sessionId ?? 'all'}`} class="sr-only">
			{m.supervision_ask_label()}
		</label>
		<input
			id={`supervisor-ask-${sessionId ?? 'all'}`}
			class="text-input-filled flex-1"
			placeholder={m.supervision_ask_placeholder()}
			autocomplete="off"
			bind:value={question}
		/>
		<button
			type="submit"
			class="btn btn-primary btn-square"
			aria-label={m.supervision_ask_submit()}
			disabled={asking || !question.trim()}
		>
			{#if asking}<Loading class="size-4" />{:else}<Send class="size-4" />{/if}
		</button>
	</form>
{/snippet}

{#if embedded}
	<div class="flex flex-col gap-3">{@render body()}</div>
{:else}
	<section class="paper gap-3" aria-label={m.supervision_ask_title()}>
		<h2 class="flex items-center gap-2 text-base font-semibold">
			<MessageCircleQuestionMark class="text-primary size-4" />
			{m.supervision_ask_title()}
		</h2>
		{@render body()}
	</section>
{/if}
