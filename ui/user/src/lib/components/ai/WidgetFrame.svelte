<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import type { Snippet } from 'svelte';

	// The card every Obot AI widget sits in, with its loading and error states.

	interface Props {
		loading?: boolean;
		error?: string;
		children?: Snippet;
	}

	let { loading = false, error = '', children }: Props = $props();
</script>

<section
	class="border-base-300 dark:border-base-400 bg-base-200/40 dark:bg-base-300/40 not-prose flex flex-col gap-3 rounded-xl border p-3"
	data-testid="ai-widget"
>
	{#if loading}
		<div class="flex justify-center py-4"><Loading class="size-5" /></div>
	{:else if error}
		<p class="text-muted-content text-xs" role="alert">{m.ai_widget_error({ error })}</p>
	{:else}
		{@render children?.()}
	{/if}
</section>
