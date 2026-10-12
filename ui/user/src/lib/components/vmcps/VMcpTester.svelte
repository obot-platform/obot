<script lang="ts">
	import { resolve } from '$app/paths';
	import Tester from '$lib/components/mcp/tester/Tester.svelte';
	import VMcpIcon from '$lib/components/vmcps/VMcpIcon.svelte';
	import { m } from '$lib/i18n';
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
		vmcpMissingStaticOAuthComponent,
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
	let missingOAuthComponent = $derived(vmcpMissingStaticOAuthComponent(vmcp));
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
	let showTester = $derived(
		Boolean(instance?.status?.configured) && !sessionStartFailed && !missingOAuthComponent
	);
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
					>{m.vmcps_manage_authentication()}</button
				>
			{/snippet}

			{#snippet setupRequiredAction()}
				<button type="button" class="btn btn-primary btn-sm" onclick={onLaunch}
					>{m.vmcps_launch()}</button
				>
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
						<p class="text-muted-content max-w-md text-sm font-light">
							{m.vmcps_starting_session()}
						</p>
					{:else}
						{#if needsConfigurationUpdate}
							<div class="indicator p-2 rounded-full bg-warning/10">
								<Settings class="text-warning size-12" />
							</div>
						{:else}
							<Layers class="text-muted-content size-12" />
						{/if}
						<p class="text-muted-content max-w-md text-sm font-light">
							{#if missingOAuthComponent}
								{m.vmcps_tester_requires_oauth_setup({ name: missingOAuthComponent.name })}
							{:else if sessionStartFailed}
								{m.vmcps_tester_session_start_failed()}
							{:else if instance && !instance.status?.configured}
								{m.vmcps_tester_update_required()}
							{:else}
								{m.vmcps_tester_start_prompt()}
							{/if}
						</p>
						{#if missingOAuthComponent}
							{#if profile.current.isAdmin?.()}
								<a
									class="btn btn-primary"
									href={resolve(
										`/mcp-servers/c/${encodeURIComponent(missingOAuthComponent.mcpServerCatalogEntryID)}?configure-oauth=true`
									)}>{m.vmcps_configure_named_oauth({ name: missingOAuthComponent.name })}</a
								>
							{:else}
								<p>{m.vmcps_ask_admin_configure_oauth()}</p>
							{/if}
						{:else if needsConfigurationUpdate}
							<button type="button" class="btn btn-primary" onclick={openInstanceConfiguration}>
								{m.vmcps_update_configuration()}
							</button>
						{:else}
							<button type="button" class="btn btn-primary" onclick={onLaunch}>
								{m.vmcps_start_session()}
							</button>
						{/if}
					{/if}
				</div>
			</section>
		</div>
	{/if}
</div>
