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

	let creatingVMcp = $state<VMcpFormData>(initVMcp());
	let creatingComponents = $state<VMCPComponent[]>([]);
	let showRequired = $state<Record<string, boolean>>({});
	let saving = $state(false);

	let createVMcpDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editVMcpDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedVMcp = $state<VMCP>();
	let editingVMcp = $state<VMcpFormData>();

	let confirmDeleteVMcp = $state<VMCP>();
	let deletingVMcp = $state(false);

	async function handleCreateVMcp() {
		showRequired = {};
		if (creatingVMcp.displayName.trim() === '') {
			showRequired.displayName = true;
		}

		if (!creatingVMcp.description || creatingVMcp.description.trim() === '') {
			showRequired.description = true;
		}

		if (Object.keys(showRequired).length > 0) {
			return;
		}

		await saveVMcp();
	}

	async function saveVMcp() {
		saving = true;
		try {
			const created = await UserService.createVMCP({
				displayName: creatingVMcp.displayName.trim(),
				description: creatingVMcp.description.trim(),
				components: creatingComponents
			});

			success.add(`${created.displayName} vMCP added.`);
			closeCreate();
			await onCreated?.(created);
		} catch {
			errors.append('Failed to create vMCP.');
		} finally {
			saving = false;
		}
	}

	export function openCreate(components: VMCPComponent[] = []) {
		if (saving) return;
		closeEdit();
		creatingComponents = components;
		creatingVMcp = initVMcp();

		if (components.length === 1) {
			creatingVMcp.displayName = components[0].name ?? '';
			creatingVMcp.description = components[0].catalogEntry?.manifest?.shortDescription ?? '';
		}

		showRequired = {};
		createVMcpDialog?.open();
	}

	export function openDelete(vmcp: VMCP) {
		selectedVMcp = vmcp;
		confirmDeleteVMcp = vmcp;
	}

	function closeCreate() {
		creatingVMcp = initVMcp();
		creatingComponents = [];
		showRequired = {};
		createVMcpDialog?.close();
	}

	export function openEdit(vmcp: VMCP) {
		closeCreate();
		selectedVMcp = vmcp;
		editingVMcp = {
			displayName: vmcp.displayName ?? '',
			description: vmcp.description ?? ''
		};
		showRequired = {};
		editVMcpDialog?.open();
	}

	function closeEdit() {
		selectedVMcp = undefined;
		editingVMcp = undefined;
		showRequired = {};
		editVMcpDialog?.close();
	}

	async function handleDeleteVMcp() {
		if (!confirmDeleteVMcp) return;

		const deleted = confirmDeleteVMcp;
		deletingVMcp = true;
		try {
			await UserService.deleteVMCP(deleted.id);
			if (selectedVMcp?.id === deleted.id) {
				closeEdit();
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

	async function handleUpdateVMcp() {
		if (!selectedVMcp || !editingVMcp) return;

		showRequired = {};
		if (editingVMcp.displayName.trim() === '') {
			showRequired.displayName = true;
		}
		if (!editingVMcp.description?.trim()) {
			showRequired.description = true;
		}
		if (Object.keys(showRequired).length > 0) return;

		saving = true;
		try {
			const updatedVMcp = await UserService.updateVMCP(selectedVMcp.id, {
				...vmcpManifest(selectedVMcp),
				displayName: editingVMcp.displayName.trim(),
				description: editingVMcp.description.trim()
			});
			success.add(`${updatedVMcp.displayName} vMCP updated.`);
			closeEdit();
			await onUpdated?.(updatedVMcp);
		} catch {
			errors.append('Failed to update vMCP.');
		} finally {
			saving = false;
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
	bind:this={createVMcpDialog}
	title="Create vMCP"
	onClose={closeCreate}
>
	<div class="mb-4 flex flex-col gap-1">
		<label
			for="create-vmcp-name"
			class={twMerge('text-sm font-light', showRequired.displayName && 'error')}
		>
			Name <span class={showRequired.displayName ? 'text-error' : ''} aria-hidden="true">*</span>
		</label>
		<input
			id="create-vmcp-name"
			class={twMerge('text-input-filled', showRequired.displayName && 'error')}
			bind:value={creatingVMcp.displayName}
			aria-required="true"
			oninput={() => updateRequired('displayName')}
		/>
		{#if showRequired.displayName}
			<p class="text-error text-xs" role="alert">Name is required</p>
		{/if}
	</div>

	<div class="flex flex-col gap-1">
		<label
			for="create-vmcp-description"
			class={twMerge('text-sm font-light', showRequired.description && 'error')}
		>
			Description
			<span class={showRequired.description ? 'text-error' : ''} aria-hidden="true">*</span>
		</label>
		<textarea
			id="create-vmcp-description"
			rows="3"
			class={twMerge('text-input-filled resize-none', showRequired.description && 'error')}
			bind:value={creatingVMcp.description}
			aria-required="true"
			oninput={() => updateRequired('description')}
		></textarea>
		{#if showRequired.description}
			<p class="text-error text-xs" role="alert">Description is required</p>
		{/if}
	</div>

	<div class="flex justify-end gap-2 mt-4">
		<button class="btn btn-ghost btn-sm text-xs" onclick={closeCreate} disabled={saving}>
			Cancel
		</button>
		<button class="btn btn-primary btn-sm text-xs" onclick={handleCreateVMcp} disabled={saving}>
			{#if saving}
				<Loading class="text-primary-content size-4" />
			{:else}
				Create
			{/if}
		</button>
	</div>
</ResponsiveDialog>

<ResponsiveDialog class="w-md" bind:this={editVMcpDialog} title="Edit vMCP" onClose={closeEdit}>
	{#if editingVMcp}
		<div class="mb-4 flex flex-col gap-1">
			<label
				for="edit-vmcp-name"
				class={twMerge('text-sm font-light', showRequired.displayName && 'error')}
			>
				Name <span class={showRequired.displayName ? 'text-error' : ''} aria-hidden="true">*</span>
			</label>
			<input
				id="edit-vmcp-name"
				class={twMerge('text-input-filled', showRequired.displayName && 'error')}
				bind:value={editingVMcp.displayName}
				aria-required="true"
				oninput={() => updateRequired('displayName')}
			/>
			{#if showRequired.displayName}
				<p class="text-error text-xs" role="alert">Name is required</p>
			{/if}
		</div>

		<div class="flex flex-col gap-1">
			<label
				for="edit-vmcp-description"
				class={twMerge('text-sm font-light', showRequired.description && 'error')}
			>
				Description
				<span class={showRequired.description ? 'text-error' : ''} aria-hidden="true">*</span>
			</label>
			<textarea
				id="edit-vmcp-description"
				rows="3"
				class={twMerge('text-input-filled resize-none', showRequired.description && 'error')}
				bind:value={editingVMcp.description}
				aria-required="true"
				oninput={() => updateRequired('description')}
			></textarea>
			{#if showRequired.description}
				<p class="text-error text-xs" role="alert">Description is required</p>
			{/if}
		</div>

		<div class="flex justify-end gap-2 mt-4">
			<button class="btn btn-ghost btn-sm text-xs" onclick={closeEdit} disabled={saving}>
				Cancel
			</button>
			<button class="btn btn-primary btn-sm text-xs" onclick={handleUpdateVMcp} disabled={saving}>
				{#if saving}
					<Loading class="text-primary-content size-4" />
				{:else}
					Save
				{/if}
			</button>
		</div>
	{/if}
</ResponsiveDialog>
