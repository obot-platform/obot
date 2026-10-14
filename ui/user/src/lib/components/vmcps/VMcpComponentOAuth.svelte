<script lang="ts">
	import StaticOAuthConfigureModal from '$lib/components/mcp/StaticOAuthConfigureModal.svelte';
	import { m } from '$lib/i18n';
	import { AdminService, type VMCPComponent } from '$lib/services';
	import type { MCPServerOAuthCredentialStatus } from '$lib/services/admin/types';
	import { profile } from '$lib/stores';
	import { tick } from 'svelte';

	let {
		vmcpID,
		component,
		disabled = false,
		onChanged
	}: {
		vmcpID?: string;
		component?: VMCPComponent;
		disabled?: boolean;
		onChanged?: () => void;
	} = $props();
	let modal = $state<ReturnType<typeof StaticOAuthConfigureModal>>();
	let status = $state<MCPServerOAuthCredentialStatus>();
	let error = $state<string>();
	let loading = $state(false);
	let visible = $derived(
		Boolean(
			profile.current.isAdmin?.() &&
			!disabled &&
			vmcpID &&
			component?.id &&
			component.catalogEntry.manifest.runtime === 'remote'
		)
	);
	let localStatus = $derived({
		configured: status?.source === 'component',
		clientID: status?.source === 'component' ? status.clientID : undefined
	});
	let description = $derived(
		status?.source === 'catalog'
			? m.vmcps_oauth_inherited({ clientID: status.clientID ?? '' }) + ' ' + m.vmcps_oauth_scope()
			: m.vmcps_oauth_scope()
	);

	async function open() {
		if (!vmcpID || !component?.id) return;
		loading = true;
		error = undefined;
		try {
			status = await AdminService.getVMCPComponentOAuthCredentials(vmcpID, component.id);
			await tick();
			modal?.open();
		} catch (err) {
			error = err instanceof Error ? err.message : m.mcps_oauth_static_oauth_save_failed();
		} finally {
			loading = false;
		}
	}
</script>

{#if visible}
	<button type="button" class="btn btn-secondary w-full" disabled={loading} onclick={open}>
		{m.mcps_oauth_static_oauth_title()}
	</button>
	{#if error}<p class="notification-error text-sm" role="alert">{error}</p>{/if}
	<StaticOAuthConfigureModal
		bind:this={modal}
		oauthStatus={localStatus}
		{description}
		deleteConfirmation={m.vmcps_oauth_clear_confirmation()}
		onSave={async (credentials) => {
			if (!vmcpID || !component?.id) return;
			status = await AdminService.setVMCPComponentOAuthCredentials(
				vmcpID,
				component.id,
				credentials
			);
			onChanged?.();
		}}
		onDelete={async () => {
			if (!vmcpID || !component?.id) return;
			status = await AdminService.deleteVMCPComponentOAuthCredentials(vmcpID, component.id);
			onChanged?.();
		}}
	/>
{/if}
