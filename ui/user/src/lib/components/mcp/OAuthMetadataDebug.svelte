<script lang="ts">
	import { m } from '$lib/i18n';
	import type { OAuthMetadata } from '$lib/services/user/types';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		metadata?: OAuthMetadata;
		compact?: boolean;
	}

	let { metadata, compact }: Props = $props();

	let hasMetadata = $derived(Boolean(metadata && Object.keys(metadata).length > 0));

	function formatJSON(value: unknown) {
		if (value === undefined || value === null) return '';
		return JSON.stringify(value, null, 2);
	}
</script>

<div
	class={twMerge(
		'dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm',
		compact ? 'rounded-none border-transparent dark:border-transparent' : ''
	)}
>
	{#if !compact}
		<div class="flex items-center justify-between gap-3">
			<h2 class="text-lg font-semibold">{m.mcps_oauth_metadata_title()}</h2>
			{#if metadata}
				<span class="text-muted-content text-xs">
					{hasMetadata
						? m.mcps_oauth_metadata_discovered()
						: m.mcps_oauth_metadata_none_discovered()}
				</span>
			{/if}
		</div>
	{/if}

	{#if !metadata}
		<p class="text-sm text-muted-content">
			{m.mcps_oauth_metadata_not_reconciled()}
		</p>
	{:else if !hasMetadata}
		<p class="text-sm text-muted-content">
			{m.mcps_oauth_metadata_none_returned()}
		</p>
	{:else}
		<div class="grid gap-3 text-sm">
			{#if metadata.protectedResourceUrl}
				<div class="grid gap-1">
					<p class="font-medium">{m.mcps_oauth_protected_resource_url()}</p>
					<p class="break-all text-muted-content">{metadata.protectedResourceUrl}</p>
				</div>
			{/if}

			{#if metadata.authorizationServerUrl}
				<div class="grid gap-1">
					<p class="font-medium">{m.mcps_oauth_authorization_server_url()}</p>
					<p class="break-all text-muted-content">
						{metadata.authorizationServerUrl}
					</p>
				</div>
			{/if}

			<div class="grid gap-1">
				<p class="font-medium">{m.mcps_oauth_dynamic_client_registration()}</p>
				<p class="text-muted-content">
					{metadata.dynamicClientRegistration
						? m.mcps_oauth_supported()
						: m.mcps_oauth_not_advertised()}
				</p>
			</div>

			<div class="grid gap-1">
				<p class="font-medium">{m.mcps_oauth_client_id_metadata_document_supported()}</p>
				<p class="text-muted-content">
					{metadata.clientIdMetadataDocumentSupported
						? m.mcps_oauth_supported()
						: m.mcps_oauth_unsupported()}
				</p>
			</div>

			{#if metadata.protectedResourceMetadata}
				<div class="grid gap-1">
					<p class="font-medium">{m.mcps_oauth_protected_resource_metadata()}</p>
					<pre class="mt-1 overflow-auto rounded-md p-3 text-xs">{formatJSON(
							metadata.protectedResourceMetadata
						)}</pre>
				</div>
			{/if}

			{#if metadata.authorizationServerMetadata}
				<div class="grid gap-1">
					<p class="font-medium">{m.mcps_oauth_authorization_server_metadata()}</p>
					<pre class="mt-1 overflow-auto rounded-md p-3 text-xs">{formatJSON(
							metadata.authorizationServerMetadata
						)}</pre>
				</div>
			{/if}

			{#if metadata.clientRegistration}
				<div class="grid gap-1">
					<p class="font-medium">{m.mcps_oauth_client_registration()}</p>
					<pre class="mt-1 overflow-auto rounded-md p-3 text-xs">{formatJSON(
							metadata.clientRegistration
						)}</pre>
				</div>
			{/if}
		</div>
	{/if}
</div>
