<script lang="ts">
	import type { OAuthMetadata } from '$lib/services/chat/types';

	interface Props {
		metadata?: OAuthMetadata;
	}

	let { metadata }: Props = $props();

	let hasMetadata = $derived(Boolean(metadata && Object.keys(metadata).length > 0));

	function formatJSON(value: unknown) {
		if (value === undefined || value === null) return '';
		return JSON.stringify(value, null, 2);
	}
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
>
	<div class="flex items-center justify-between gap-3">
		<h2 class="text-lg font-semibold">OAuth Metadata</h2>
		{#if metadata}
			<span class="text-on-surface1 text-xs">
				{hasMetadata ? 'Discovered' : 'No metadata discovered'}
			</span>
		{/if}
	</div>

	{#if !metadata}
		<p class="text-sm text-on-surface1">OAuth metadata has not been reconciled yet.</p>
	{:else if !hasMetadata}
		<p class="text-sm text-on-surface1">No OAuth metadata was returned by this MCP server.</p>
	{:else}
		<div class="grid gap-3 text-sm">
			{#if metadata.protectedResourceUrl}
				<div class="grid gap-1">
					<p class="font-medium">Protected Resource URL</p>
					<p class="break-all text-on-surface1">{metadata.protectedResourceUrl}</p>
				</div>
			{/if}

			{#if metadata.authorizationServerUrl}
				<div class="grid gap-1">
					<p class="font-medium">Authorization Server URL</p>
					<p class="break-all text-on-surface1">
						{metadata.authorizationServerUrl}
					</p>
				</div>
			{/if}

			<div class="grid gap-1">
				<p class="font-medium">Dynamic Client Registration</p>
				<p class="text-on-surface1">
					{metadata.dynamicClientRegistration ? 'Supported' : 'Not advertised'}
				</p>
			</div>

			{#if metadata.protectedResourceMetadata}
				<div class="grid gap-1">
					<p class="font-medium">Protected Resource Metadata</p>
					<pre
						class="bg-surface1 dark:bg-surface2 mt-1 overflow-auto rounded-md p-3 text-xs">{formatJSON(
							metadata.protectedResourceMetadata
						)}</pre>
				</div>
			{/if}

			{#if metadata.authorizationServerMetadata}
				<div class="grid gap-1">
					<p class="font-medium">Authorization Server Metadata</p>
					<pre
						class="bg-surface1 dark:bg-surface2 mt-1 overflow-auto rounded-md p-3 text-xs">{formatJSON(
							metadata.authorizationServerMetadata
						)}</pre>
				</div>
			{/if}
		</div>
	{/if}
</div>
