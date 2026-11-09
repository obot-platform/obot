<script lang="ts">
	import { m } from '$lib/i18n';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import { splitAnswer } from '$lib/services/ai';
	import ApprovalsWidget from './ApprovalsWidget.svelte';
	import ServerRequestWidget from './ServerRequestWidget.svelte';
	import SessionWidget from './SessionWidget.svelte';
	import SessionsWidget from './SessionsWidget.svelte';
	import WidgetFrame from './WidgetFrame.svelte';

	// An Obot AI answer: markdown with live widgets placed where the model put them.

	let { content }: { content: string } = $props();

	let segments = $derived(splitAnswer(content));
</script>

<div class="flex flex-col gap-3">
	{#each segments as segment, i (i)}
		{#if segment.kind === 'markdown'}
			<div class="milkdown-content">
				{@html toHTMLFromMarkdownWithNewTabLinks(segment.text)}
			</div>
		{:else if segment.kind === 'pending'}
			<WidgetFrame>
				<p class="text-muted-content animate-pulse text-xs">{m.ai_widget_preparing()}</p>
			</WidgetFrame>
		{:else if segment.kind === 'invalid'}
			<p class="text-muted-content text-xs italic">{m.ai_widget_invalid()}</p>
		{:else if segment.widget.type === 'session'}
			{#key segment.widget.sessionId}
				<SessionWidget sessionId={segment.widget.sessionId} />
			{/key}
		{:else if segment.widget.type === 'sessions'}
			{#key segment.widget.sessionIds.join(',')}
				<SessionsWidget sessionIds={segment.widget.sessionIds} />
			{/key}
		{:else if segment.widget.type === 'mcp_server_request'}
			{#key segment.widget.requestId}
				<ServerRequestWidget requestId={segment.widget.requestId} />
			{/key}
		{:else if segment.widget.type === 'approvals'}
			<ApprovalsWidget />
		{/if}
	{/each}
</div>
