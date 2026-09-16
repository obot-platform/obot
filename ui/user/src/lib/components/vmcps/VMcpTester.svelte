<script lang="ts">
	import Tester from '$lib/components/mcp/tester/Tester.svelte';
	import VMcpIcon from '$lib/components/vmcps/VMcpIcon.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import type { VMCP, VMCPInstance } from '$lib/services';
	import { testerChatAvailability, type TesterStatus } from '$lib/services/mcp/tester.svelte';
	import { vmcpTesterServer } from '$lib/services/vmcps/tester';
	import {
		resolveVMcpComponents,
		vmcpHasUserAllowedConfiguration
	} from '$lib/services/vmcps/utils';
	import {
		accessibleModels,
		defaultModelAliases,
		profile,
		version,
		vmcpInstances
	} from '$lib/stores';
	import { Layers, Settings } from '@lucide/svelte';

	interface Props {
		vmcp: VMCP;
		onLaunch: () => void;
		loading?: boolean;
		openEditInstanceConfiguration?: (vmcp: VMCP, instance: VMCPInstance) => void;
	}

	let { vmcp, onLaunch, loading = false, openEditInstanceConfiguration }: Props = $props();

	let componentViews = $derived(resolveVMcpComponents(vmcp));
	let instance = $derived(
		vmcpInstances.current.items.find(
			(candidate) => candidate.vmcpID === vmcp.id && candidate.userID === profile.current.id
		)
	);
	let launched = $derived(Boolean(instance));
	let serverName = $derived(vmcp.displayName || vmcp.id);
	let server = $derived(vmcpTesterServer(vmcp, instance?.id ?? vmcp.id, instance));
	let chatAvailability = $derived(
		testerChatAvailability(version.current, defaultModelAliases.current, accessibleModels.current)
	);
	let chatAvailable = $derived(chatAvailability.available);
	let chatUnavailableMessage = $derived(chatAvailability.unavailableMessage);
	let hasUserProvidedConfiguration = $derived(vmcpHasUserAllowedConfiguration(vmcp));
	let credentialUpdateRequired = $state(false);

	$effect(() => {
		void vmcp.id;
		void instance?.id;
		void loading;
		credentialUpdateRequired = false;
	});

	function handleTesterStatus(status: TesterStatus, error?: string) {
		if (!instance?.status?.configured || !hasUserProvidedConfiguration || error === undefined) {
			return;
		}
		if (status === 'error' || status === 'unhealthy') {
			credentialUpdateRequired = true;
		}
	}

	function openInstanceConfiguration() {
		if (!instance) return;
		openEditInstanceConfiguration?.(vmcp, instance);
	}

	let showTester = $derived(Boolean(instance?.status?.configured) && !credentialUpdateRequired);
	let needsConfigurationUpdate = $derived(
		Boolean(instance && (!instance.status?.configured || credentialUpdateRequired))
	);
</script>

<div class="py-4 h-full w-full">
	{#if showTester}
		<Tester
			{server}
			{serverName}
			{chatAvailable}
			{chatUnavailableMessage}
			active={launched}
			loading={loading || (vmcpInstances.current.loading && !launched)}
			onStatus={handleTesterStatus}
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
				class="border-base-300 dark:border-base-400 bg-base-100 dark:bg-base-300 m-4 w-xs rounded-lg border p-6 text-center"
				role="status"
			>
				<div class="relative z-10 flex flex-col items-center gap-4">
					{#if needsConfigurationUpdate}
						<div class="indicator p-2 rounded-full bg-warning/10">
							<Settings class="text-warning size-12" />
						</div>
					{:else}
						<Layers class="text-muted-content size-12" />
					{/if}
					<p class="text-muted-content max-w-md text-sm font-light">
						{#if credentialUpdateRequired}
							vMCP requires valid credential, verify information provided and try again.
						{:else if instance && !instance.status?.configured}
							Before you can continue inspecting this vMCP, an update is required.
						{:else}
							Start your vMCP to use chat and inspect tools.
						{/if}
					</p>
					{#if needsConfigurationUpdate}
						<button type="button" class="btn btn-primary" onclick={openInstanceConfiguration}>
							Update Configuration
						</button>
					{:else}
						<button
							type="button"
							class="btn btn-primary"
							onclick={onLaunch}
							disabled={vmcpInstances.current.loading}
						>
							{#if vmcpInstances.current.loading}
								<Loading class="text-primary" />
							{:else}
								Start Session
							{/if}
						</button>
					{/if}
				</div>
			</section>
		</div>
	{/if}
</div>
