<script lang="ts">
	import { m } from '$lib/i18n';
	import type { GatewayStatus } from '$lib/services/llm-gateway/gateway';
	import { resolve } from '$lib/url';
	import { CircleAlert, Info, LoaderCircle } from '@lucide/svelte';

	interface Props {
		status?: GatewayStatus;
		showNoProviders?: boolean;
	}

	let { status, showNoProviders = true }: Props = $props();
</script>

{#if status?.error}
	<div class="notification-error flex items-center gap-2 rounded-md p-3 text-sm">
		<CircleAlert class="size-4 shrink-0" />
		{m.models_status_failed_to_start({ error: status.error })}
	</div>
{:else if status && !status.ready}
	<div class="notification-info flex items-center gap-2 rounded-md p-3 text-sm">
		<LoaderCircle class="size-4 shrink-0 animate-spin" />
		{m.models_status_starting()}
	</div>
{:else if status && showNoProviders && !status.hasProviders}
	<div class="notification-info flex items-center gap-2 rounded-md p-3 text-sm">
		<Info class="size-4 shrink-0" />
		<span>
			{m.models_status_no_providers()}
			<a href={resolve('/models?view=providers')} class="text-link"
				>{m.models_status_add_provider_link()}</a
			>
			{m.models_status_add_provider_suffix()}
		</span>
	</div>
{/if}
