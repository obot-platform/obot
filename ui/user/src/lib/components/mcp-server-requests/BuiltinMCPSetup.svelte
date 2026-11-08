<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { m } from '$lib/i18n';
	import {
		BUILTIN_ADMIN_MCP_ID,
		BUILTIN_USER_MCP_ID,
		builtinMCPURL,
		managedSettingsSnippets
	} from '$lib/services/mcpserverrequests';
	import { Bot } from '@lucide/svelte';

	// How to connect an agent to Obot's built-in MCP servers, and for administrators, the
	// Claude managed settings that push the user server to everyone.
	let { admin = false }: { admin?: boolean } = $props();

	let origin = $derived(typeof window === 'undefined' ? '' : window.location.origin);
	let servers = $derived([
		{ label: m.server_requests_user_server(), url: builtinMCPURL(BUILTIN_USER_MCP_ID, origin) },
		...(admin
			? [
					{
						label: m.server_requests_admin_server(),
						url: builtinMCPURL(BUILTIN_ADMIN_MCP_ID, origin)
					}
				]
			: [])
	]);
	let snippets = $derived(admin ? managedSettingsSnippets(origin) : []);
</script>

<section
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg p-4 shadow-sm"
	aria-labelledby="builtin-mcp-setup-title"
>
	<div class="flex items-start gap-3">
		<Bot class="text-primary mt-0.5 size-5 shrink-0" />
		<div class="flex flex-col gap-1">
			<h2 id="builtin-mcp-setup-title" class="font-semibold">{m.server_requests_setup_title()}</h2>
			<p class="text-muted-content text-sm">
				{m.server_requests_setup_description()}
				{#if admin}
					{m.server_requests_setup_admin_description()}
				{/if}
			</p>
		</div>
	</div>

	<dl class="grid gap-2 sm:grid-cols-[max-content_1fr]">
		{#each servers as server (server.url)}
			<dt class="text-sm font-medium">{server.label}</dt>
			<dd class="flex min-w-0 items-center gap-2">
				<code class="bg-base-200 dark:bg-base-300 truncate rounded px-2 py-1 text-xs"
					>{server.url}</code
				>
				<CopyButton text={server.url} />
			</dd>
		{/each}
	</dl>

	{#if snippets.length > 0}
		<div class="grid gap-3 md:grid-cols-2">
			{#each snippets as snippet (snippet.id)}
				<div class="flex min-w-0 flex-col gap-1">
					<div class="flex items-center justify-between">
						<span class="text-xs font-medium">{snippet.label}</span>
						<CopyButton text={snippet.value} />
					</div>
					<pre
						class="bg-base-200 dark:bg-base-300 overflow-x-auto rounded p-2 text-xs">{snippet.value}</pre>
				</div>
			{/each}
		</div>
		<p class="text-muted-content text-xs">{m.server_requests_slack_hint()}</p>
	{/if}
</section>
