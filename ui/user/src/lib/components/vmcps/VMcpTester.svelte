<script lang="ts">
	import Tester from '$lib/components/mcp/tester/Tester.svelte';
	import VMcpIcon from '$lib/components/vmcps/VMcpIcon.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import type { VMCP, VMCPInstance } from '$lib/services';
	import {
		isUnavailableTesterFailure,
		testerChatAvailability,
		type TesterStatus
	} from '$lib/services/mcp/tester.svelte';
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
	let sessionStartFailed = $state(false);

	$effect(() => {
		void vmcp.id;
		void instance?.id;
		void loading;
		sessionStartFailed = false;
	});

	function handleTesterStatus(status: TesterStatus) {
		if (!instance?.status?.configured || !hasUserProvidedConfiguration) {
			return;
		}
		if (isUnavailableTesterFailure(status)) {
			sessionStartFailed = true;
		}
	}

	function openInstanceConfiguration() {
		if (!instance) return;
		openEditInstanceConfiguration?.(vmcp, instance);
	}

	let launching = $derived(loading || (vmcpInstances.current.loading && !launched));
	let showTester = $derived(Boolean(instance?.status?.configured) && !sessionStartFailed);
	let needsConfigurationUpdate = $derived(
		Boolean(!launching && instance && (!instance.status?.configured || sessionStartFailed))
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
			loading={launching}
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
					{#if launching}
						<Loading class="size-12" />
						<p class="text-muted-content max-w-md text-sm font-light">Starting session...</p>
					{:else}
						{#if needsConfigurationUpdate}
							<div class="indicator p-2 rounded-full bg-warning/10">
								<Settings class="text-warning size-12" />
							</div>
						{:else}
							<Layers class="text-muted-content size-12" />
						{/if}
						<p class="text-muted-content max-w-md text-sm font-light">
							{#if sessionStartFailed}
								There was an issue starting the session. Please verify configuration or contact
								support if the issue persists.
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
							<button type="button" class="btn btn-primary" onclick={onLaunch}>
								Start Session
							</button>
						{/if}
					{/if}
				</div>
			</section>
		</div>
	{/if}
</div>
