<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type AccessControlRule,
		type AccessControlRuleManifest,
		type CatalogComponentServer,
		type MCPCatalogEntry,
		type RuntimeFormData
	} from '$lib/services';
	import { initVMcp } from '$lib/services/vmcps/utils';
	import { errors, mcpServersAndEntries, profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import SearchAccessPolicies from './SearchAccessPolicies.svelte';
	import { BookOpenText, Plus, Trash2, X } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onCreated?: (created: MCPCatalogEntry) => void | Promise<void>;
	}

	let { onCreated }: Props = $props();

	let creatingVMcp = $state<RuntimeFormData>(initVMcp());
	let showRequired = $state<Record<string, boolean>>({});
	let accessPolicies = $state<AccessControlRule[]>([]);
	let initialAccessPolicies = $state<AccessControlRule[]>([]);
	let saving = $state(false);

	let createVMcpDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editVMcpDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedVMcp = $state<MCPCatalogEntry>();
	let editingVMcp = $state<RuntimeFormData>();
	let loadingVMcpAccess = $state(false);
	let accessLoadController: AbortController | undefined;

	let addAccessPolicyDialog = $state<ReturnType<typeof SearchAccessPolicies>>();
	let confirmDeleteVMcp = $state<MCPCatalogEntry>();
	let deletingVMcp = $state(false);

	async function handleCreateVMcp() {
		showRequired = {};
		if (creatingVMcp.name.trim() === '') {
			showRequired.name = true;
		}

		if (!creatingVMcp.shortDescription || creatingVMcp.shortDescription.trim() === '') {
			showRequired.shortDescription = true;
		}

		if (Object.keys(showRequired).length > 0) {
			return;
		}

		await saveVMcpAndAccessPolicy();
	}

	async function saveVMcpAndAccessPolicy() {
		saving = true;
		try {
			const composite = await AdminService.createMCPCatalogEntry(
				DEFAULT_MCP_CATALOG_ID,
				creatingVMcp
			);

			await mcpServersAndEntries.refreshEntries();
			success.add(`${composite.manifest.name} vMCP added.`);

			closeCreate();

			const created =
				mcpServersAndEntries.current.entries.find((entry) => entry.id === composite.id) ??
				composite;
			await onCreated?.(created);
		} finally {
			saving = false;
		}
	}

	export function openCreate(componentServers: CatalogComponentServer[] = []) {
		if (saving) return;
		closeEdit();
		creatingVMcp = {
			...initVMcp(),
			compositeConfig: { componentServers }
		};

		if (componentServers.length === 1) {
			creatingVMcp.name = componentServers[0].manifest?.name ?? '';
			creatingVMcp.shortDescription = componentServers[0].manifest?.shortDescription ?? '';
		}

		resetAccessPolicies();
		showRequired = {};
		createVMcpDialog?.open();
	}

	function closeCreate() {
		creatingVMcp = initVMcp();
		resetAccessPolicies();
		showRequired = {};
		createVMcpDialog?.close();
	}

	function resetAccessPolicies() {
		accessPolicies = [];
		initialAccessPolicies = [];
	}

	function abortAccessLoad() {
		accessLoadController?.abort();
		accessLoadController = undefined;
	}

	function ruleIncludesVMcp(rule: AccessControlRule, vmcpId: string) {
		return (
			rule.resources?.some(
				(resource) =>
					(resource.type === 'mcpServerCatalogEntry' && resource.id === vmcpId) ||
					resource.id === '*'
			) ?? false
		);
	}

	function toPolicyManifest(
		policy: AccessControlRule,
		resources: AccessControlRuleManifest['resources']
	): AccessControlRuleManifest {
		return {
			displayName: policy.displayName,
			subjects: policy.subjects,
			resources
		};
	}

	function isGlobalAccessPolicy(policy: AccessControlRule) {
		return policy.resources?.some((resource) => resource.id === '*') ?? false;
	}

	function subjectCountLabel(policy: AccessControlRule) {
		const count = policy.subjects?.length ?? 0;
		const isEveryone = policy.subjects?.some((subject) => subject.id === '*');
		if (count === 0) return 'No subjects';
		if (isEveryone) return 'Everyone';
		return count === 1 ? '1 subject' : `${count} subjects`;
	}

	function vmcpToFormData(vmcp: MCPCatalogEntry): RuntimeFormData {
		const manifest = vmcp.manifest;
		return {
			...initVMcp(),
			...manifest,
			name: manifest.name ?? '',
			shortDescription: manifest.shortDescription ?? '',
			description: manifest.description ?? '',
			icon: manifest.icon ?? '',
			env: manifest.env ?? [],
			categories: manifest.metadata?.categories?.split(',').filter(Boolean) ?? ['']
		};
	}

	export async function openEdit(vmcp: MCPCatalogEntry) {
		closeCreate();
		abortAccessLoad();
		const controller = new AbortController();
		accessLoadController = controller;
		selectedVMcp = vmcp;
		editingVMcp = vmcpToFormData(vmcp);
		resetAccessPolicies();
		showRequired = {};
		loadingVMcpAccess = true;
		editVMcpDialog?.open();

		const editingId = vmcp.id;
		try {
			const rules = await AdminService.listAccessControlRules({ signal: controller.signal });
			if (controller.signal.aborted || selectedVMcp?.id !== editingId) return;
			const assigned = rules.filter((rule) => ruleIncludesVMcp(rule, editingId));
			initialAccessPolicies = [...assigned];
			accessPolicies = [...assigned];
		} catch {
			if (!controller.signal.aborted && selectedVMcp?.id === editingId) resetAccessPolicies();
		} finally {
			if (accessLoadController === controller) {
				accessLoadController = undefined;
				loadingVMcpAccess = false;
			}
		}
	}

	function closeEdit() {
		abortAccessLoad();
		selectedVMcp = undefined;
		editingVMcp = undefined;
		resetAccessPolicies();
		showRequired = {};
		loadingVMcpAccess = false;
		editVMcpDialog?.close();
	}

	async function handleDeleteVMcp() {
		if (!confirmDeleteVMcp) return;

		deletingVMcp = true;
		try {
			await AdminService.deleteMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, confirmDeleteVMcp.id);
			mcpServersAndEntries.current.entries = mcpServersAndEntries.current.entries.filter(
				(entry) => entry.id !== confirmDeleteVMcp?.id
			);
			if (selectedVMcp?.id === confirmDeleteVMcp.id) {
				closeEdit();
			}
			success.add(`${confirmDeleteVMcp.manifest.name} vMCP deleted.`);
		} catch {
			errors.append('Failed to delete vMCP.');
		} finally {
			deletingVMcp = false;
			confirmDeleteVMcp = undefined;
			mcpServersAndEntries.refreshEntries();
		}
	}

	async function handleUpdateVMcp() {
		if (!selectedVMcp || !editingVMcp) return;

		showRequired = {};
		if (editingVMcp.name.trim() === '') {
			showRequired.name = true;
		}
		if (!editingVMcp.shortDescription?.trim()) {
			showRequired.shortDescription = true;
		}
		if (Object.keys(showRequired).length > 0) return;

		saving = true;
		try {
			const updatedVMcp = await AdminService.updateMCPCatalogEntry(
				DEFAULT_MCP_CATALOG_ID,
				selectedVMcp.id,
				editingVMcp
			);
			const vmcpId = updatedVMcp.id;
			const initialIds = new Set(initialAccessPolicies.map((policy) => policy.id));
			const currentIds = new Set(accessPolicies.map((policy) => policy.id));

			await Promise.all([
				...accessPolicies
					.filter((policy) => !initialIds.has(policy.id))
					.map((policy) =>
						AdminService.updateAccessControlRule(
							policy.id,
							toPolicyManifest(policy, [
								...(policy.resources ?? []),
								{ type: 'mcpServerCatalogEntry', id: vmcpId }
							])
						)
					),
				...initialAccessPolicies
					.filter((policy) => !currentIds.has(policy.id) && !isGlobalAccessPolicy(policy))
					.map((policy) =>
						AdminService.updateAccessControlRule(
							policy.id,
							toPolicyManifest(
								policy,
								(policy.resources ?? []).filter(
									(resource) =>
										!(resource.type === 'mcpServerCatalogEntry' && resource.id === vmcpId)
								)
							)
						)
					)
			]);

			mcpServersAndEntries.current.entries = mcpServersAndEntries.current.entries.map((entry) =>
				entry.id === updatedVMcp.id ? updatedVMcp : entry
			);
			success.add(`${updatedVMcp.manifest.name} vMCP updated.`);
			closeEdit();
		} finally {
			saving = false;
		}
	}

	function updateRequired(field: string) {
		delete showRequired[field];
	}

	function canManageAccess() {
		return Boolean(profile.current.hasAdminAccess?.() && !profile.current.isAdminReadonly?.());
	}
</script>

{#snippet peopleAccessSection()}
	<div class="divider mb-2"></div>
	<div class="flex items-center justify-between">
		<h2 class="text-sm font-semibold">Access Policies</h2>
		{#if canManageAccess()}
			<IconButton
				variant="primary"
				tooltip={{ text: 'Add access policy', disablePortal: true }}
				class="btn-sm"
				onclick={() => addAccessPolicyDialog?.open()}
			>
				<Plus class="size-4" />
			</IconButton>
		{/if}
	</div>
	<ul class="list">
		{#if loadingVMcpAccess}
			<li class="flex min-h-14 items-center justify-center">
				<Loading class="text-primary size-4" />
			</li>
		{:else if accessPolicies.length > 0}
			{#each accessPolicies as policy (policy.id)}
				<li class="list-row px-0">
					<div>
						<div
							class="bg-base-300 dark:bg-base-200 flex size-8 items-center justify-center rounded-full text-white"
						>
							<BookOpenText class="size-4" />
						</div>
					</div>
					<div>
						<div>{policy.displayName}</div>
						<div class="text-xs font-semibold uppercase opacity-60">
							{subjectCountLabel(policy)}
						</div>
					</div>
					{#if canManageAccess() && !isGlobalAccessPolicy(policy)}
						<IconButton
							variant="danger"
							class="btn-sm"
							tooltip={{ text: 'Remove access policy', disablePortal: true }}
							onclick={() => {
								accessPolicies = accessPolicies.filter((candidate) => candidate.id !== policy.id);
							}}
						>
							<X class="size-4" />
						</IconButton>
					{/if}
				</li>
			{/each}
		{:else}
			<li class="text-muted-content inline-flex min-h-14 items-center justify-center gap-1 text-xs">
				No assigned access policies.
			</li>
		{/if}
	</ul>
{/snippet}

<Confirm
	show={Boolean(confirmDeleteVMcp)}
	onsuccess={handleDeleteVMcp}
	oncancel={() => (confirmDeleteVMcp = undefined)}
	msg=""
	loading={deletingVMcp}
	title="Confirm Delete"
>
	{#snippet note()}
		Are you sure you want to delete "<b>{confirmDeleteVMcp?.manifest.name ?? 'this vMCP'}</b>"? This
		cannot be undone.
	{/snippet}
</Confirm>

<SearchAccessPolicies
	bind:this={addAccessPolicyDialog}
	filterIds={accessPolicies.map((policy) => policy.id)}
	onAdd={(policies) => {
		accessPolicies = [...accessPolicies, ...policies];
	}}
/>

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
			class={twMerge('text-sm font-light', showRequired.name && 'error')}
		>
			Name <span class={showRequired.name ? 'text-error' : ''} aria-hidden="true">*</span>
		</label>
		<input
			id="create-vmcp-name"
			class={twMerge('text-input-filled', showRequired.name && 'error')}
			bind:value={creatingVMcp.name}
			aria-required="true"
			oninput={() => updateRequired('name')}
		/>
		{#if showRequired.name}
			<p class="text-error text-xs" role="alert">Name is required</p>
		{/if}
	</div>

	<div class="flex flex-col gap-1">
		<label
			for="create-vmcp-description"
			class={twMerge('text-sm font-light', showRequired.shortDescription && 'error')}
		>
			Description
			<span class={showRequired.shortDescription ? 'text-error' : ''} aria-hidden="true">*</span>
		</label>
		<textarea
			id="create-vmcp-description"
			rows="3"
			class={twMerge('text-input-filled resize-none', showRequired.shortDescription && 'error')}
			bind:value={creatingVMcp.shortDescription}
			aria-required="true"
			oninput={() => updateRequired('shortDescription')}
		></textarea>
		{#if showRequired.shortDescription}
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

<ResponsiveDialog
	class="w-md"
	bind:this={editVMcpDialog}
	title={`Edit ${editingVMcp?.name ?? 'vMCP'}`}
	onClose={closeEdit}
>
	{#if editingVMcp}
		<div class="mb-4 flex flex-col gap-1">
			<label
				for="edit-vmcp-name"
				class={twMerge('text-sm font-light', showRequired.name && 'error')}
			>
				Name <span class={showRequired.name ? 'text-error' : ''} aria-hidden="true">*</span>
			</label>
			<input
				id="edit-vmcp-name"
				class={twMerge('text-input-filled', showRequired.name && 'error')}
				bind:value={editingVMcp.name}
				aria-required="true"
				oninput={() => updateRequired('name')}
			/>
			{#if showRequired.name}
				<p class="text-error text-xs" role="alert">Name is required</p>
			{/if}
		</div>

		<div class="flex flex-col gap-1">
			<label
				for="edit-vmcp-description"
				class={twMerge('text-sm font-light', showRequired.shortDescription && 'error')}
			>
				Description
				<span class={showRequired.shortDescription ? 'text-error' : ''} aria-hidden="true">*</span>
			</label>
			<textarea
				id="edit-vmcp-description"
				rows="3"
				class={twMerge('text-input-filled resize-none', showRequired.shortDescription && 'error')}
				bind:value={editingVMcp.shortDescription}
				aria-required="true"
				oninput={() => updateRequired('shortDescription')}
			></textarea>
			{#if showRequired.shortDescription}
				<p class="text-error text-xs" role="alert">Description is required</p>
			{/if}
		</div>

		{@render peopleAccessSection()}
		<div class="divider mt-0 mb-2"></div>
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
				<button
					class="btn btn-primary btn-sm text-xs"
					onclick={handleUpdateVMcp}
					disabled={saving || loadingVMcpAccess}
				>
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
