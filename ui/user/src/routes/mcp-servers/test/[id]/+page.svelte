<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import Tester from '$lib/components/mcp/tester/Tester.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { Server, ArrowLeft } from '@lucide/svelte';
	import type { Component } from 'svelte';

	let { data } = $props();

	let serverName = $derived(data.server.alias || data.server.manifest.name || data.server.id);
	let configuredDefault = $derived(
		data.defaultModelAliases?.find((alias) => alias.alias === 'llm')
	);
	let defaultModel = $derived(
		configuredDefault?.model
			? data.models?.find(
					(model) =>
						model.active &&
						(model.id === configuredDefault?.model ||
							(model.aliasAssigned && model.alias === configuredDefault?.model))
				)
			: undefined
	);
	let chatAvailable = $derived(Boolean(configuredDefault?.model && defaultModel));
	let chatUnavailableMessage = $derived(
		!configuredDefault?.model
			? 'No default llm model is configured. Configure one to use Chat.'
			: 'The configured default llm model is inactive or unavailable to your account.'
	);
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
	<Tester server={data.server} {serverName} {chatAvailable} {chatUnavailableMessage}>
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
			<a class="btn btn-primary btn-sm" href={resolve(data.backTarget as `/${string}`)}
				>Manage authentication</a
			>
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
</Layout>

<svelte:head>
	<title>Obot | {data.server.id.startsWith('vmcp1') ? 'vMCP' : 'MCP'} Tester | {serverName}</title>
</svelte:head>
