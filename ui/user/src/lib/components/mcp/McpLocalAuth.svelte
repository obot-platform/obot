<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import McpDeprecatedNotice from './McpDeprecatedNotice.svelte';
	import McpLogin from './McpLogin.svelte';
	import { Server } from '@lucide/svelte';

	let {
		url,
		serverName = '',
		icon,
		deprecated = false,
		loading = false,
		onComplete
	}: {
		url: string;
		serverName?: string;
		icon?: string;
		deprecated?: boolean;
		loading?: boolean;
		onComplete?: () => void | Promise<void>;
	} = $props();
	let expanded = $state(false);
	let expired = $state(false);
	const id = $props.id();
	let name = $derived.by(() => {
		try {
			return (
				new URL(url, 'https://obot.invalid').searchParams.get('serverName') ||
				serverName ||
				m.mcps_login_server()
			);
		} catch {
			return serverName || m.mcps_login_server();
		}
	});
</script>

<div class="w-full min-w-0">
	<div class="flex min-w-0 flex-wrap items-center gap-3 p-3">
		{#if icon}
			<img src={icon} alt={m.mcps_oauth_icon_alt()} class="size-8 shrink-0" />
		{:else}
			<Server class="size-8 shrink-0" />
		{/if}
		<span class="min-w-0 flex-1 break-words text-sm font-medium">{name}</span>
		<McpDeprecatedNotice {deprecated} child />
		{#if expanded}
			<span role="status" class="flex items-center gap-2 text-sm">
				{#if !expired}<Loading class="size-4 shrink-0" />{/if}
				{expired ? m.mcps_login_expired_status() : m.mcps_oauth_composite_oauth_checking()}
			</span>
		{:else}
			<button
				type="button"
				class="btn btn-primary shrink-0"
				aria-expanded={expanded}
				aria-controls={id}
				disabled={loading}
				onclick={() => (expanded = true)}
			>
				{m.mcps_oauth_authenticate()}
			</button>
		{/if}
	</div>
	{#if expanded}
		<div {id} class="border-base-300 border-t p-3">
			<McpLogin
				{url}
				serverName={name}
				{loading}
				{onComplete}
				showTitle={false}
				showStatus={false}
				onExpiredChange={(value) => (expired = value)}
			/>
		</div>
	{/if}
</div>
