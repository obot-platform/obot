<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import McpCompositeOauth from '$lib/components/mcp/McpCompositeOauth.svelte';
	import Tester from '$lib/components/mcp/tester/Tester.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { testerChatAvailability } from '$lib/services/mcp/tester.svelte';
	import { version } from '$lib/stores';
	import { Server, ArrowLeft } from '@lucide/svelte';
	import type { Component } from 'svelte';

	let { data } = $props();
	let tester = $state<ReturnType<typeof Tester>>();
	let managingAuthentication = $state(false);

	function authenticationComplete(): void {
		managingAuthentication = false;
		tester?.reconnect();
	}

	let serverName = $derived(data.server.alias || data.server.manifest.name || data.server.id);
	let chatAvailability = $derived(
		testerChatAvailability(version.current, data.defaultModelAliases, data.models)
	);
	let chatAvailable = $derived(chatAvailability.available);
	let chatUnavailableMessage = $derived(chatAvailability.unavailableMessage);
</script>

<Layout
	classes={{ container: 'min-h-0 overflow-hidden', childrenContainer: 'min-h-0' }}
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: {
			class: 'overflow-hidden',
			as: 'main',
			itemHeight: 56,
			overscan: 5,
			disabled: true
		}
	}}
	title={serverName}
	showBackButton
	onBackButtonClick={() => goto(resolve(data.backTarget as `/${string}`))}
>
	<!-- Keep the tester mounted so authentication preserves its conversation and session. -->
	<div class="h-full min-h-0" class:hidden={managingAuthentication}>
		<Tester
			bind:this={tester}
			server={data.server}
			{serverName}
			{chatAvailable}
			{chatUnavailableMessage}
		>
			{#snippet icon()}
				{#if data.server.manifest.icon}
					<img class="size-7 rounded-md object-contain" src={data.server.manifest.icon} alt="" />
				{:else}
					<div
						class="bg-base-200 dark:bg-base-300 flex size-7 items-center justify-center rounded-md"
					>
						<Server class="size-4" aria-hidden="true" />
					</div>
				{/if}
			{/snippet}

			{#snippet headerActions()}
				<a class="btn btn-secondary btn-sm" href={resolve(data.backTarget as `/${string}`)}>
					<ArrowLeft class="size-4" aria-hidden="true" />
					Back to {serverName}
				</a>
			{/snippet}

			{#snippet accessDeniedAction()}
				<a class="btn btn-secondary btn-sm" href={resolve(data.backTarget as `/${string}`)}
					>Back to server management</a
				>
			{/snippet}

			{#snippet reauthenticationAction()}
				{#if data.vmcpID}
					<button
						type="button"
						class="btn btn-primary btn-sm"
						onclick={() => (managingAuthentication = true)}
					>
						Manage authentication
					</button>
				{:else}
					<a class="btn btn-primary btn-sm" href={resolve(data.backTarget as `/${string}`)}
						>Manage authentication</a
					>
				{/if}
			{/snippet}

			{#snippet setupRequiredAction()}
				<a class="btn btn-primary btn-sm" href={resolve(data.backTarget as `/${string}`)}
					>Manage server</a
				>
			{/snippet}

			{#snippet unhealthySecondaryAction()}
				<a class="btn btn-secondary btn-sm" href={resolve(data.backTarget as `/${string}`)}
					>Manage server</a
				>
			{/snippet}
		</Tester>
	</div>

	{#if managingAuthentication && data.vmcpID}
		<section class="h-full min-h-0 overflow-y-auto">
			<button
				type="button"
				class="btn btn-secondary btn-sm mb-3"
				onclick={() => (managingAuthentication = false)}
			>
				<ArrowLeft class="size-4" aria-hidden="true" /> Back to tester
			</button>
			<McpCompositeOauth
				class="min-h-0"
				compositeMcpId={data.server.id}
				vmcpId={data.vmcpID}
				onComplete={authenticationComplete}
			/>
		</section>
	{/if}
</Layout>

<svelte:head>
	<title>Obot | {data.server.id.startsWith('vmcp') ? 'vMCP' : 'MCP'} Tester | {serverName}</title>
</svelte:head>
