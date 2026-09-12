<script lang="ts">
	import Tester from '$lib/components/mcp/tester/Tester.svelte';
	import VMcpIcon from '$lib/components/vmcps/VMcpIcon.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import type { VMCP } from '$lib/services';
	import { vmcpTesterServer } from '$lib/services/vmcps/tester';
	import { resolveVMcpComponents } from '$lib/services/vmcps/utils';
	import { accessibleModels, defaultModelAliases, profile, vmcpInstances } from '$lib/stores';
	import { Layers } from '@lucide/svelte';

	interface Props {
		vmcp: VMCP;
		onLaunch: () => void;
	}

	let { vmcp, onLaunch }: Props = $props();

	let componentViews = $derived(resolveVMcpComponents(vmcp));
	let instance = $derived(
		vmcpInstances.current.items.find(
			(candidate) => candidate.vmcpID === vmcp.id && candidate.userID === profile.current.id
		)
	);
	let launched = $derived(Boolean(instance));
	let serverName = $derived(vmcp.displayName || vmcp.id);
	let server = $derived(vmcpTesterServer(vmcp, vmcp.id, instance));
	let configuredDefault = $derived(
		defaultModelAliases.current.find((alias) => alias.alias === 'llm')
	);
	let defaultModel = $derived(
		configuredDefault?.model
			? accessibleModels.current.find(
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

<div class="py-4 h-full w-full">
	{#if instance && instance.status?.configured}
		<Tester
			{server}
			{serverName}
			{chatAvailable}
			{chatUnavailableMessage}
			active={launched}
			loading={vmcpInstances.current.loading && !launched}
		>
			{#snippet icon()}
				<VMcpIcon components={componentViews} />
			{/snippet}

			{#snippet reauthenticationAction()}
				<button type="button" class="btn btn-primary btn-sm" onclick={onLaunch}
					>Manage authentication</button
				>
			{/snippet}

			{#snippet setupRequiredAction()}
				<button type="button" class="btn btn-primary btn-sm" onclick={onLaunch}>Launch</button>
			{/snippet}
		</Tester>
	{:else}
		<div class="flex h-full w-full items-center justify-center">
			<section
				class="border-base-300 dark:border-base-400 bg-base-100 dark:bg-base-300 m-4 w-sm rounded-lg border p-6 text-center"
				role="status"
			>
				<div class="relative z-10 flex flex-col items-center gap-4">
					<Layers class="text-muted-content size-12 opacity-25" />
					<p class="text-muted-content max-w-md text-sm font-light">
						In order to test this VMCP, you will need to launch it. Click below to begin launching
					</p>
					<button
						type="button"
						class="btn btn-primary"
						onclick={onLaunch}
						disabled={vmcpInstances.current.loading}
					>
						{#if vmcpInstances.current.loading}
							<Loading class="text-primary" />
						{:else}
							Launch VMCP
						{/if}
					</button>
				</div>
			</section>
		</div>
	{/if}
</div>
