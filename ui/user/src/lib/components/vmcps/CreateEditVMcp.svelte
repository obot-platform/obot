<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type VMCP, type VMCPComponent } from '$lib/services';
	import {
		initVMcp,
		isCatalogSyncedVMcp,
		vmcpManifest,
		type VMcpFormData
	} from '$lib/services/vmcps/utils';
	import { errors } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import VMcpCatalogSyncedIndicator from './VMcpCatalogSyncedIndicator.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onCreated?: (created: VMCP) => void | Promise<void>;
		onDeleted?: (deleted: VMCP) => void | Promise<void>;
		onUpdated?: (updated: VMCP) => void | Promise<void>;
	}

	let { onCreated, onDeleted, onUpdated }: Props = $props();

	let form = $state<VMcpFormData>(initVMcp());
	let creatingComponents = $state<VMCPComponent[]>([]);
	let showRequired = $state<Record<string, boolean>>({});
	let saving = $state(false);
	let dialogOpen = $state(false);

	let vmcpDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedVMcp = $state<VMCP>();

	let confirmDeleteVMcp = $state<VMCP>();
	let deletingVMcp = $state(false);

	const editing = $derived(Boolean(selectedVMcp));
	const readonly = $derived(isCatalogSyncedVMcp(selectedVMcp));

	function resetForm() {
		form = initVMcp();
		creatingComponents = [];
		selectedVMcp = undefined;
		showRequired = {};
	}

	function openDialog() {
		if (dialogOpen) return;
		dialogOpen = true;
		vmcpDialog?.open();
	}

	function closeDialog() {
		resetForm();
		if (!dialogOpen) return;
		dialogOpen = false;
		vmcpDialog?.close();
	}

	function handleDialogClose() {
		dialogOpen = false;
		resetForm();
	}

	function validateForm() {
		showRequired = {};
		if (form.displayName.trim() === '') {
			showRequired.displayName = true;
		}
		if (!form.description?.trim()) {
			showRequired.description = true;
		}
		return Object.keys(showRequired).length === 0;
	}

	async function handleSubmit() {
		if (!validateForm()) return;
		if (selectedVMcp) {
			await updateVMcp(selectedVMcp);
		} else {
			await createVMcp();
		}
	}

	async function createVMcp() {
		saving = true;
		try {
			const created = await UserService.createVMCP({
				displayName: form.displayName.trim(),
				description: form.description.trim(),
				components: creatingComponents
			});

			success.add(m.vmcps_vmcp_added({ name: created.displayName }));
			await onCreated?.(created);
			closeDialog();
		} catch {
			errors.append(m.vmcps_failed_to_create());
		} finally {
			saving = false;
		}
	}

	async function updateVMcp(vmcp: VMCP) {
		saving = true;
		try {
			const updatedVMcp = await UserService.updateVMCP(vmcp.id, {
				...vmcpManifest(vmcp),
				displayName: form.displayName.trim(),
				description: form.description.trim()
			});
			success.add(m.vmcps_vmcp_updated({ name: updatedVMcp.displayName }));
			closeDialog();
			await onUpdated?.(updatedVMcp);
		} catch {
			errors.append(m.vmcps_failed_to_update());
		} finally {
			saving = false;
		}
	}

	export function openCreate(components: VMCPComponent[] = []) {
		if (saving) return;
		selectedVMcp = undefined;
		creatingComponents = components;
		form = initVMcp();

		if (components.length === 1) {
			form.displayName = components[0].name ?? '';
			form.description = components[0].catalogEntry?.manifest?.shortDescription ?? '';
		}

		showRequired = {};
		openDialog();
	}

	export function openDelete(vmcp: VMCP) {
		confirmDeleteVMcp = vmcp;
	}

	export function openEdit(vmcp: VMCP) {
		if (saving) return;
		creatingComponents = [];
		selectedVMcp = vmcp;
		form = {
			displayName: vmcp.displayName ?? '',
			description: vmcp.description ?? ''
		};
		showRequired = {};
		openDialog();
	}

	async function handleDeleteVMcp() {
		if (!confirmDeleteVMcp) return;

		const deleted = confirmDeleteVMcp;
		deletingVMcp = true;
		try {
			await UserService.deleteVMCP(deleted.id);
			if (selectedVMcp?.id === deleted.id) {
				closeDialog();
			}
			success.add(m.vmcps_vmcp_deleted({ name: deleted.displayName }));
			await onDeleted?.(deleted);
		} catch {
			errors.append(m.vmcps_failed_to_delete());
		} finally {
			deletingVMcp = false;
			confirmDeleteVMcp = undefined;
		}
	}

	function updateRequired(field: string) {
		delete showRequired[field];
	}
</script>

<Confirm
	show={Boolean(confirmDeleteVMcp)}
	onsuccess={handleDeleteVMcp}
	oncancel={() => (confirmDeleteVMcp = undefined)}
	msg=""
	loading={deletingVMcp}
	title={m.vmcps_deployments_confirm_delete()}
>
	{#snippet note()}
		{m.vmcps_delete_confirm_prefix()}<b
			>{confirmDeleteVMcp?.displayName ?? m.vmcps_deployments_this_vmcp()}</b
		>{m.vmcps_delete_confirm_suffix()}
	{/snippet}
</Confirm>

<ResponsiveDialog
	animate="slide"
	class="md:w-md"
	bind:this={vmcpDialog}
	title={editing ? m.vmcps_edit_details() : m.vmcps_create_vmcp()}
	onClose={handleDialogClose}
>
	<div class="flex grow flex-col p-4 md:p-0">
		{#if selectedVMcp}
			<VMcpCatalogSyncedIndicator vmcp={selectedVMcp} inline />
		{/if}

		<div class="mb-4 flex flex-col gap-1">
			<label
				for="vmcp-name"
				class={twMerge('text-sm font-light', showRequired.displayName && 'error')}
			>
				{m.core_name()}
				<span class={showRequired.displayName ? 'text-error' : ''} aria-hidden="true">*</span>
			</label>
			<input
				id="vmcp-name"
				class={twMerge('text-input-filled', showRequired.displayName && 'error')}
				bind:value={form.displayName}
				aria-required="true"
				oninput={() => updateRequired('displayName')}
				disabled={readonly}
				aria-disabled={readonly}
			/>
			{#if showRequired.displayName}
				<p class="text-error text-xs" role="alert">{m.vmcps_name_required()}</p>
			{/if}
		</div>

		<div class="flex flex-col gap-1">
			<label
				for="vmcp-description"
				class={twMerge('text-sm font-light', showRequired.description && 'error')}
			>
				{m.core_description()}
				<span class={showRequired.description ? 'text-error' : ''} aria-hidden="true">*</span>
			</label>
			<textarea
				id="vmcp-description"
				rows="3"
				class={twMerge('text-input-filled resize-none', showRequired.description && 'error')}
				bind:value={form.description}
				aria-required="true"
				oninput={() => updateRequired('description')}
				disabled={readonly}
				aria-disabled={readonly}></textarea>
			{#if showRequired.description}
				<p class="text-error text-xs" role="alert">{m.vmcps_description_required()}</p>
			{/if}
		</div>

		<div class="flex grow"></div>

		{#if !readonly}
			<div class="flex md:flex-row flex-col justify-end gap-2 mt-4">
				<button class="btn btn-ghost rounded-full" onclick={closeDialog} disabled={saving}>
					{m.common_cancel()}
				</button>
				<button class="btn btn-primary" onclick={handleSubmit} disabled={saving}>
					{#if saving}
						<Loading class="text-primary-content size-4" />
					{:else}
						{editing ? m.core_save() : m.vmcps_create()}
					{/if}
				</button>
			</div>
		{/if}
	</div>
</ResponsiveDialog>
