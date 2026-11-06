<script lang="ts">
	import { resolve } from '$app/paths';
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { getHttpStatusCode, httpErrorReason } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type VMCP, type VMCPComponent } from '$lib/services';
	import {
		getToolCounts,
		initVMcp,
		isCatalogSyncedVMcp,
		resolveVMcpComponents,
		vmcpManifest,
		type VMcpFormData
	} from '$lib/services/vmcps/utils';
	import { errors, profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import VMcpCatalogSyncedIndicator from './VMcpCatalogSyncedIndicator.svelte';
	import { CircleAlert } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	export type VMcpCreateDraft = VMcpFormData & {
		components: VMCPComponent[];
	};

	// SaveFailure is why the last create or update failed. The dialog shows it, so it stays open to fix and retry.
	type SaveFailure = {
		title: string;
		reason?: string;
		overPlan: boolean;
	};

	interface Props {
		onCreated?: (created: VMCP) => void | Promise<void>;
		onDeleted?: (deleted: VMCP) => void | Promise<void>;
		onUpdated?: (updated: VMCP) => void | Promise<void>;
		onClose?: () => void;
		onCreate?: (draft: VMcpCreateDraft) => boolean | Promise<boolean>;
	}

	let { onCreated, onDeleted, onUpdated, onClose, onCreate }: Props = $props();

	let form = $state<VMcpFormData>(initVMcp());
	let creatingComponents = $state<VMCPComponent[]>([]);
	let showRequired = $state<Record<string, boolean>>({});
	let saving = $state(false);
	let failure = $state<SaveFailure>();
	let dialogOpen = $state(false);
	let confirmToolSearch = $state(false);

	let vmcpDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedVMcp = $state<VMCP>();

	let confirmDeleteVMcp = $state<VMCP>();
	let deletingVMcp = $state(false);

	const editing = $derived(Boolean(selectedVMcp));
	const readonly = $derived(isCatalogSyncedVMcp(selectedVMcp));
	const toolCounts = $derived(
		getToolCounts(resolveVMcpComponents({ components: creatingComponents }))
	);
	const dialogTitle = $derived(
		confirmToolSearch
			? m.vmcps_tool_search_prompt_title()
			: editing
				? m.vmcps_edit_details()
				: m.vmcps_create_vmcp()
	);

	function resetForm() {
		form = initVMcp();
		creatingComponents = [];
		selectedVMcp = undefined;
		showRequired = {};
		confirmToolSearch = false;
		failure = undefined;
	}

	// saveFailure explains err: the server's reason when it gave one, and whether the plan had no room for it.
	function saveFailure(err: unknown, title: string): SaveFailure {
		return {
			title,
			reason: httpErrorReason(err),
			overPlan: getHttpStatusCode(err) === 402
		};
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
		onClose?.();
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

	function createDraft(): VMcpCreateDraft {
		return {
			displayName: form.displayName.trim(),
			description: form.description.trim(),
			toolSearch: form.toolSearch,
			components: creatingComponents
		};
	}

	async function handleSubmit() {
		if (saving || !validateForm()) return;
		if (selectedVMcp) {
			await updateVMcp(selectedVMcp);
			return;
		}

		if (!form.toolSearch && toolCounts.enabled > 50) {
			confirmToolSearch = true;
			return;
		}

		await submitCreate();
	}

	async function submitCreate() {
		const draft = createDraft();
		if (await onCreate?.(draft)) {
			closeDialog();
			return;
		}

		if (saving) return;
		saving = true;
		failure = undefined;
		try {
			await createVMcp(draft);
		} catch (err) {
			failure = saveFailure(err, m.vmcps_failed_to_create());
		} finally {
			saving = false;
		}
	}

	async function createVMcp(draft: VMcpCreateDraft) {
		const created = await UserService.createVMCP(
			{
				displayName: draft.displayName,
				description: draft.description,
				toolSearch: draft.toolSearch,
				components: draft.components
			},
			{ dontLogErrors: true }
		);

		success.add(m.vmcps_vmcp_added({ name: created.displayName }));
		await onCreated?.(created);
		closeDialog();
	}

	// saveCreate creates draft for a caller that collected it elsewhere. A failure is thrown, for the caller to show,
	// and so is a save that's already running, so the caller doesn't take it as done.
	export async function saveCreate(draft: VMcpCreateDraft) {
		if (saving) throw new Error(m.vmcps_failed_to_create());
		saving = true;
		try {
			await createVMcp(draft);
		} finally {
			saving = false;
		}
	}

	async function createWithToolSearch(enabled: boolean) {
		if (saving) return;
		form.toolSearch = enabled;
		await submitCreate();
	}

	async function updateVMcp(vmcp: VMCP) {
		saving = true;
		failure = undefined;
		try {
			const updatedVMcp = await UserService.updateVMCP(
				vmcp.id,
				{
					...vmcpManifest(vmcp),
					displayName: form.displayName.trim(),
					description: form.description.trim(),
					toolSearch: form.toolSearch
				},
				{ dontLogErrors: true }
			);
			success.add(m.vmcps_vmcp_updated({ name: updatedVMcp.displayName }));
			closeDialog();
			await onUpdated?.(updatedVMcp);
		} catch (err) {
			failure = saveFailure(err, m.vmcps_failed_to_update());
		} finally {
			saving = false;
		}
	}

	export function openCreate(components: VMCPComponent[] = []) {
		if (saving) return;
		selectedVMcp = undefined;
		creatingComponents = components;
		form = initVMcp();
		confirmToolSearch = false;

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
		confirmToolSearch = false;
		form = {
			displayName: vmcp.displayName ?? '',
			description: vmcp.description ?? '',
			toolSearch: vmcp.toolSearch ?? false
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
	title={dialogTitle}
	onClose={handleDialogClose}
>
	{#snippet titleContent()}
		<span class:font-normal={confirmToolSearch}>{dialogTitle}</span>
	{/snippet}
	<div class="flex grow flex-col p-4 md:p-0">
		{#if confirmToolSearch}
			<p class="text-sm text-muted-content">
				{toolCounts.approximate
					? m.vmcps_tool_search_prompt_preview()
					: m.vmcps_tool_search_prompt_description()}
			</p>
			{@render saveFailureNotice()}
			<div class="mt-4 flex flex-wrap justify-end gap-2">
				<button
					type="button"
					class="btn btn-ghost"
					disabled={saving}
					onclick={() => (confirmToolSearch = false)}>{m.common_back()}</button
				>
				<button
					type="button"
					class="btn btn-secondary"
					disabled={saving}
					onclick={() => createWithToolSearch(false)}>{m.vmcps_tool_search_keep_direct()}</button
				>
				<button
					type="button"
					class="btn btn-primary"
					disabled={saving}
					onclick={() => createWithToolSearch(true)}
				>
					{#if saving && form.toolSearch}<Loading class="text-primary-content size-4" />{/if}
					{m.vmcps_tool_search_enable()}
				</button>
			</div>
		{:else}
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

			<label
				for="vmcp-tool-search"
				class="border-base-300 dark:border-base-400 mt-4 flex items-center justify-between gap-4 rounded-lg border p-4"
			>
				<span>
					<span class="text-sm font-medium">{m.vmcps_tool_search()}</span>
					<span class="block text-xs opacity-70">
						{m.vmcps_tool_search_description()}
					</span>
				</span>
				<input
					id="vmcp-tool-search"
					type="checkbox"
					class="toggle shrink-0"
					bind:checked={form.toolSearch}
					disabled={readonly}
					aria-label={m.vmcps_tool_search()}
				/>
			</label>

			<div class="flex grow"></div>

			{@render saveFailureNotice()}

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
		{/if}
	</div>
</ResponsiveDialog>

{#snippet saveFailureNotice()}
	{#if failure}
		<div class="notification-error mt-4 flex items-start gap-2 text-sm" role="alert">
			<CircleAlert class="mt-0.5 size-4 shrink-0" />
			<div class="flex flex-col gap-1">
				<p class="font-medium">{failure.title}</p>
				{#if failure.reason}
					<p class="font-light">{failure.reason}</p>
				{/if}
				{#if failure.overPlan && profile.current.hasAdminAccess?.()}
					<a class="text-link self-start" href={resolve('/admin/platform?view=license')}>
						{m.platform_license_notice_view_plan()}
					</a>
				{/if}
			</div>
		</div>
	{/if}
{/snippet}
