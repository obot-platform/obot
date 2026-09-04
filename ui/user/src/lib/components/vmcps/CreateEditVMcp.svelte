<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type VMCP, type VMCPComponent, type VMCPManifest } from '$lib/services';
	import { initVMcp } from '$lib/services/vmcps/utils';
	import { errors } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { Trash2 } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onCreated?: (created: VMCP) => void | Promise<void>;
		onChanged?: (changed: VMCP) => void | Promise<void>;
		onDeleted?: (id: string) => void | Promise<void>;
	}

	let { onCreated, onChanged, onDeleted }: Props = $props();

	let creatingVMcp = $state<VMCPManifest>(initVMcp());
	let showRequired = $state<Record<string, boolean>>({});
	let saving = $state(false);

	let createVMcpDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editVMcpDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedVMcp = $state<VMCP>();
	let editingVMcp = $state<VMCPManifest>();
	let confirmDeleteVMcp = $state<VMCP>();
	let deletingVMcp = $state(false);

	function validateManifest(manifest: VMCPManifest) {
		showRequired = {};
		if (!manifest.displayName.trim()) showRequired.displayName = true;
		if (!manifest.description?.trim()) showRequired.description = true;
		return Object.keys(showRequired).length === 0;
	}

	async function handleCreateVMcp() {
		if (!validateManifest(creatingVMcp)) return;

		saving = true;
		try {
			const created = await UserService.createVMCP(creatingVMcp);
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
		creatingVMcp = {
			...initVMcp(),
			components: [...components]
		};

		if (components.length === 1) {
			const manifest = components[0].catalogEntry.manifest;
			creatingVMcp.displayName =
				components[0].name || manifest.name || components[0].mcpServerCatalogEntryID;
			creatingVMcp.description = manifest.shortDescription || manifest.description;
		}

		showRequired = {};
		createVMcpDialog?.open();
	}

	function closeCreate() {
		creatingVMcp = initVMcp();
		showRequired = {};
		createVMcpDialog?.close();
	}

	function vmcpToManifest(vmcp: VMCP): VMCPManifest {
		return {
			displayName: vmcp.displayName,
			description: vmcp.description,
			icon: vmcp.icon,
			components: vmcp.components,
			profiles: vmcp.profiles,
			forceSingleUser: vmcp.forceSingleUser
		};
	}

	export function openEdit(vmcp: VMCP) {
		closeCreate();
		selectedVMcp = vmcp;
		editingVMcp = vmcpToManifest(vmcp);
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

		deletingVMcp = true;
		try {
			const id = confirmDeleteVMcp.id;
			await UserService.deleteVMCP(id);
			if (selectedVMcp?.id === id) closeEdit();
			await onDeleted?.(id);
			success.add(`${confirmDeleteVMcp.displayName} vMCP deleted.`);
		} catch {
			errors.append('Failed to delete vMCP.');
		} finally {
			deletingVMcp = false;
			confirmDeleteVMcp = undefined;
		}
	}

	async function handleUpdateVMcp() {
		if (!selectedVMcp || !editingVMcp || !validateManifest(editingVMcp)) return;

		saving = true;
		try {
			const updated = await UserService.updateVMCP(selectedVMcp.id, editingVMcp);
			await onChanged?.(updated);
			success.add(`${updated.displayName} vMCP updated.`);
			closeEdit();
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

	<p class="mt-3 text-xs">Add an MCP server before connecting to this vMCP.</p>

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

<ResponsiveDialog
	class="w-md"
	bind:this={editVMcpDialog}
	title={`Edit ${editingVMcp?.displayName ?? 'vMCP'}`}
	onClose={closeEdit}
>
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

		<div class="divider mt-4 mb-2"></div>
		<div class="flex items-center justify-between gap-2">
			<button
				class="btn btn-error btn-soft btn-sm"
				onclick={() => {
					if (!selectedVMcp) return;
					editVMcpDialog?.close();
					confirmDeleteVMcp = selectedVMcp;
				}}
				disabled={saving || deletingVMcp}
			>
				<Trash2 class="size-4" /> Delete
			</button>
			<div class="flex gap-2">
				<button class="btn btn-ghost btn-sm text-xs" onclick={closeEdit} disabled={saving}>
					Cancel
				</button>
				<button class="btn btn-primary btn-sm text-xs" onclick={handleUpdateVMcp} disabled={saving}>
					{#if saving}
						<Loading class="text-primary-content size-4" />
					{:else}
						Save changes
					{/if}
				</button>
			</div>
		</div>
	{/if}
</ResponsiveDialog>
