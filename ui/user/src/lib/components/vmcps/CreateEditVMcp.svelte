<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type VMCP, type VMCPComponent } from '$lib/services';
	import { initVMcp, vmcpManifest, type VMcpFormData } from '$lib/services/vmcps/utils';
	import { errors } from '$lib/stores';
	import { success } from '$lib/stores/success';
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

			success.add(`${created.displayName} vMCP added.`);
			await onCreated?.(created);
			closeDialog();
		} catch {
			errors.append('Failed to create vMCP.');
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
			success.add(`${updatedVMcp.displayName} vMCP updated.`);
			closeDialog();
			await onUpdated?.(updatedVMcp);
		} catch {
			errors.append('Failed to update vMCP.');
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
			success.add(`${deleted.displayName} vMCP deleted.`);
			await onDeleted?.(deleted);
		} catch {
			errors.append('Failed to delete vMCP.');
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
	title="Confirm Delete"
>
	{#snippet note()}
		Are you sure you want to delete "<b>{confirmDeleteVMcp?.displayName ?? 'this vMCP'}</b>"? This
		cannot be undone.
	{/snippet}
</Confirm>

<ResponsiveDialog
	animate="slide"
	class="w-md"
	bind:this={vmcpDialog}
	title={editing ? 'Edit vMCP' : 'Create vMCP'}
	onClose={handleDialogClose}
>
	<div class="mb-4 flex flex-col gap-1">
		<label
			for="vmcp-name"
			class={twMerge('text-sm font-light', showRequired.displayName && 'error')}
		>
			Name <span class={showRequired.displayName ? 'text-error' : ''} aria-hidden="true">*</span>
		</label>
		<input
			id="vmcp-name"
			class={twMerge('text-input-filled', showRequired.displayName && 'error')}
			bind:value={form.displayName}
			aria-required="true"
			oninput={() => updateRequired('displayName')}
		/>
		{#if showRequired.displayName}
			<p class="text-error text-xs" role="alert">Name is required</p>
		{/if}
	</div>

	<div class="flex flex-col gap-1">
		<label
			for="vmcp-description"
			class={twMerge('text-sm font-light', showRequired.description && 'error')}
		>
			Description
			<span class={showRequired.description ? 'text-error' : ''} aria-hidden="true">*</span>
		</label>
		<textarea
			id="vmcp-description"
			rows="3"
			class={twMerge('text-input-filled resize-none', showRequired.description && 'error')}
			bind:value={form.description}
			aria-required="true"
			oninput={() => updateRequired('description')}
		></textarea>
		{#if showRequired.description}
			<p class="text-error text-xs" role="alert">Description is required</p>
		{/if}
	</div>

	<div class="flex justify-end gap-2 mt-4">
		<button class="btn btn-ghost rounded-full" onclick={closeDialog} disabled={saving}>
			Cancel
		</button>
		<button class="btn btn-primary" onclick={handleSubmit} disabled={saving}>
			{#if saving}
				<Loading class="text-primary-content size-4" />
			{:else}
				{editing ? 'Save' : 'Create'}
			{/if}
		</button>
	</div>
</ResponsiveDialog>
