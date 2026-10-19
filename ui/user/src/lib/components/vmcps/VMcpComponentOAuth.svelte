<script lang="ts">
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { m } from '$lib/i18n';
	import { AdminService, type VMCPComponent } from '$lib/services';
	import type { MCPServerOAuthCredentialStatus } from '$lib/services/admin/types';
	import { profile } from '$lib/stores';
	import { onMount } from 'svelte';

	let {
		vmcpID,
		component,
		disabled = false,
		saving = false
	}: {
		vmcpID?: string;
		component?: VMCPComponent;
		disabled?: boolean;
		saving?: boolean;
	} = $props();
	let status = $state<MCPServerOAuthCredentialStatus>();
	let error = $state<string>();
	let loading = $state(true);
	let enabled = $state(false);
	let clientID = $state('');
	let clientSecret = $state('');
	let visible = $derived(
		Boolean(
			profile.current.isAdmin?.() &&
			!disabled &&
			vmcpID &&
			component?.id &&
			component.catalogEntry.manifest.runtime === 'remote'
		)
	);

	onMount(async () => {
		if (!visible || !vmcpID || !component?.id) return;
		try {
			status = await AdminService.getVMCPComponentOAuthCredentials(vmcpID, component.id);
			enabled = status.source === 'component';
			clientID = enabled ? (status.clientID ?? '') : '';
		} catch (err) {
			error = err instanceof Error ? err.message : m.mcps_oauth_static_oauth_save_failed();
		} finally {
			loading = false;
		}
	});

	export async function save() {
		if (!visible || !vmcpID || !component?.id) return;
		if (loading || !status) throw new Error(error ?? m.mcps_oauth_static_oauth_save_failed());
		if (enabled) {
			if (!clientID.trim()) throw new Error(m.vmcps_complete_fixed_fields());
			if (
				status.source === 'component' &&
				clientID.trim() === status.clientID &&
				!clientSecret.trim()
			)
				return;
			status = await AdminService.setVMCPComponentOAuthCredentials(vmcpID, component.id, {
				clientID: clientID.trim(),
				clientSecret: clientSecret.trim()
			});
			clientSecret = '';
		} else if (status.source === 'component') {
			status = await AdminService.deleteVMCPComponentOAuthCredentials(vmcpID, component.id);
		}
	}
</script>

{#if visible}
	<div class="flex flex-col gap-3">
		<label class="flex items-center gap-2">
			<input
				type="checkbox"
				class="checkbox checkbox-sm"
				bind:checked={enabled}
				disabled={loading || saving || !status}
			/>
			<span>{m.mcps_catalog_remote_remote_static_oauth()}</span>
		</label>
		<p class="text-xs font-light text-muted-content">{m.vmcps_oauth_scope()}</p>
		{#if status?.source === 'catalog'}
			<p class="text-xs font-light text-muted-content">
				{m.vmcps_oauth_inherited({ clientID: status.clientID ?? '' })}
			</p>
		{/if}
		{#if error}<p class="notification-error text-sm" role="alert">{error}</p>{/if}
		<div class="ml-6 flex flex-col gap-3" class:opacity-50={!enabled}>
			<label class="flex flex-col gap-1">
				<span>{m.mcps_oauth_static_oauth_client_id()}</span>
				<input
					type="text"
					class="text-input-filled"
					bind:value={clientID}
					disabled={!enabled || loading || saving || !status}
					autocomplete="off"
				/>
			</label>
			<div class="flex flex-col gap-1">
				<label for="componentClientSecret"
					>{m.mcps_oauth_static_oauth_client_secret_optional()}</label
				>
				<SensitiveInput
					name="componentClientSecret"
					bind:value={clientSecret}
					disabled={!enabled || loading || saving || !status}
					placeholder={status?.source === 'component' ? '••••••••' : ''}
				/>
				{#if status?.source === 'component'}
					<p class="text-xs font-light text-muted-content">{m.vmcps_oauth_replacement_help()}</p>
				{/if}
			</div>
		</div>
		{#if status?.source === 'component'}
			<p
				class="text-xs font-light text-muted-content"
				class:invisible={enabled}
				aria-hidden={enabled}
			>
				{m.vmcps_oauth_clear_confirmation()}
			</p>
		{/if}
	</div>
{/if}
