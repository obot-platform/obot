<script lang="ts">
	import type { MCPTesterSession } from '$lib/services/mcp/tester.svelte';
	import { X } from '@lucide/svelte';

	interface Props {
		session: MCPTesterSession;
	}

	let { session }: Props = $props();
</script>

{#if session.stagedContext.length}
	<section class="mt-4 space-y-3" aria-label="Staged Chat context">
		<h3 class="font-medium">Staged context</h3>
		<p class="text-sm text-muted-content">Nothing is sent until you send a Chat message.</p>
		{#each session.stagedContext as context (context.id)}
			<article class="border-base-300 dark:border-base-400 rounded-lg border p-3">
				<div class="flex items-start justify-between gap-3">
					<div class="min-w-0">
						<p class="text-sm font-medium break-all">{context.name}</p>
						<p class="text-xs text-muted-content">
							{context.type === 'prompt' ? 'Prompt' : 'Resource'}
						</p>
					</div>
					<button
						type="button"
						class="btn btn-ghost btn-square btn-sm"
						onclick={() => session.removeStagedContext(context.id)}
						aria-label={`Remove staged ${context.name}`}
					>
						<X class="size-4" aria-hidden="true" />
					</button>
				</div>
				<details class="mt-2">
					<summary class="cursor-pointer text-xs font-medium">Preview</summary>
					{#if context.type === 'prompt'}
						<div class="mt-2 space-y-2">
							{#each context.messages as message, index (index)}
								<div class="bg-base-200 dark:bg-base-300 rounded-lg p-2">
									<p class="text-xs font-semibold uppercase">{message.role}</p>
									{#if message.content.type === 'text'}
										<pre class="mt-1 text-sm whitespace-pre-wrap wrap-break-word">{message.content
												.text}</pre>
									{/if}
								</div>
							{/each}
						</div>
					{:else}
						<div class="mt-2 space-y-2">
							{#each context.contents as content (content.uri)}
								<div class="bg-base-200 dark:bg-base-300 rounded-lg p-2">
									<p class="text-xs break-all">{content.uri} · {content.mimeType || 'text'}</p>
									<pre class="mt-1 text-sm whitespace-pre-wrap wrap-break-word">{content.text}</pre>
								</div>
							{/each}
						</div>
					{/if}
				</details>
			</article>
		{/each}
	</section>
{/if}
