<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import { AdminService, type GitCredential, type GitCredentialManifest } from '$lib/services';
	import { errors, profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { Pencil, Trash2, TriangleAlert, X } from '@lucide/svelte';
	import { untrack } from 'svelte';

	interface Props {
		gitCredentials: GitCredential[];
		dirty?: boolean;
		saving?: boolean;
	}

	type DraftCreate = {
		id: string;
		displayName: string;
		host: string;
		token: string;
	};

	type DraftEdit = {
		displayName: string;
		host: string;
		token: string;
	};

	const draftPrefix = 'draft-';

	let {
		gitCredentials = $bindable(),
		dirty = $bindable(false),
		saving: settingsSaving = false
	}: Props = $props();
	let editingCredential = $state<GitCredential>();
	let viewingCredential = $state<GitCredential>();
	let displayName = $state('');
	let host = $state('');
	let token = $state('');
	let clearToken = $state(false);
	let formError = $state('');
	let requestPending = $state(false);
	let dialog = $state<HTMLDialogElement>();
	let baseline = $state<GitCredential[]>(untrack(() => cloneCredentials(gitCredentials)));
	let pendingCreates = $state<DraftCreate[]>([]);
	let pendingEdits = $state<Record<string, DraftEdit>>({});
	let pendingDeletes = $state<string[]>([]);
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let editingDraft = $derived(pendingCreates.find((draft) => draft.id === editingCredential?.id));
	let editingPersisted = $derived(Boolean(editingCredential) && !editingDraft);
	let showExistingToken = $derived(
		editingPersisted &&
			Boolean(editingCredential?.tokenConfigured) &&
			!clearToken &&
			!pendingEdits[editingCredential?.id ?? '']?.token
	);
	let tokenRequired = $derived(
		!editingPersisted || !editingCredential?.tokenConfigured || clearToken
	);
	let inputsDisabled = $derived(isReadonly || settingsSaving || requestPending);
	let isDirty = $derived(
		pendingCreates.length > 0 || Object.keys(pendingEdits).length > 0 || pendingDeletes.length > 0
	);
	let rows = $derived([
		...pendingCreates.map((draft) => credentialFromDraft(draft)),
		...baseline
			.filter((credential) => !pendingDeletes.includes(credential.id))
			.map((credential) => {
				const edit = pendingEdits[credential.id];
				return edit
					? { ...credential, displayName: edit.displayName, host: edit.host }
					: credential;
			})
	]);
	let tableData = $derived(
		rows.map((credential) => ({
			...credential,
			usedBy:
				(credential.uses.skillRepositories?.length ?? 0) +
				(credential.uses.mcpCatalogs?.length ?? 0) +
				(credential.uses.systemMcpCatalogs?.length ?? 0)
		}))
	);

	$effect(() => {
		if (dirty !== isDirty) dirty = isDirty;
	});

	$effect(() => {
		const incoming = gitCredentials;
		if (
			pendingCreates.length > 0 ||
			Object.keys(pendingEdits).length > 0 ||
			pendingDeletes.length > 0
		) {
			return;
		}
		baseline = cloneCredentials(incoming);
	});

	function useGroups(credential?: GitCredential) {
		return [
			{
				label: m.platform_settings_git_credentials_skill_repositories(),
				uses: credential?.uses.skillRepositories ?? []
			},
			{
				label: m.platform_settings_git_credentials_mcp_catalogs(),
				uses: credential?.uses.mcpCatalogs ?? []
			},
			{
				label: m.platform_settings_git_credentials_system_mcp_catalogs(),
				uses: credential?.uses.systemMcpCatalogs ?? []
			}
		].filter((group) => group.uses.length > 0);
	}

	function hasUses(credential?: GitCredential) {
		return useGroups(credential).length > 0;
	}

	function conflictStatus(error: unknown) {
		if (typeof error !== 'object' || error === null) return false;
		if ('statusCode' in error && Number(error.statusCode) === 409) return true;
		return error instanceof Error && error.message.startsWith('409 ');
	}

	function emptyUses() {
		return { skillRepositories: [], mcpCatalogs: [], systemMcpCatalogs: [] };
	}

	function cloneCredentials(items: GitCredential[]) {
		return items.map((item) => ({
			...item,
			uses: {
				skillRepositories: [...(item.uses?.skillRepositories ?? [])],
				mcpCatalogs: [...(item.uses?.mcpCatalogs ?? [])],
				systemMcpCatalogs: [...(item.uses?.systemMcpCatalogs ?? [])]
			}
		}));
	}

	function credentialFromDraft(draft: DraftCreate): GitCredential {
		return {
			id: draft.id,
			displayName: draft.displayName,
			host: draft.host,
			tokenConfigured: true,
			uses: emptyUses()
		};
	}

	export function openCreate() {
		editingCredential = undefined;
		displayName = '';
		host = '';
		token = '';
		clearToken = false;
		formError = '';
		dialog?.showModal();
	}

	function openEdit(credential: GitCredential) {
		const draft = pendingCreates.find((item) => item.id === credential.id);
		const edit = pendingEdits[credential.id];
		editingCredential = credential;
		displayName = draft?.displayName ?? edit?.displayName ?? credential.displayName;
		host = draft?.host ?? edit?.host ?? credential.host;
		token = draft?.token ?? edit?.token ?? '';
		clearToken = false;
		formError = '';
		dialog?.showModal();
	}

	function stageDelete(credential: GitCredential) {
		if (inputsDisabled || hasUses(credential)) return;
		if (credential.id.startsWith(draftPrefix)) {
			pendingCreates = pendingCreates.filter((draft) => draft.id !== credential.id);
			return;
		}
		if (pendingDeletes.includes(credential.id)) return;
		pendingDeletes = [...pendingDeletes, credential.id];
		delete pendingEdits[credential.id];
	}

	function openUses(credential: GitCredential) {
		viewingCredential = credential;
	}

	function closeDialog() {
		dialog?.close();
	}

	function handleDialogClose() {
		token = '';
		clearToken = false;
		formError = '';
	}

	function stageCredential() {
		const normalizedName = displayName.trim();
		const normalizedHost = host.trim();
		const normalizedToken = token.trim();
		if (
			inputsDisabled ||
			!normalizedName ||
			!normalizedHost ||
			(tokenRequired && !normalizedToken)
		) {
			return;
		}

		formError = '';
		if (!editingCredential || editingCredential.id.startsWith(draftPrefix)) {
			const draft: DraftCreate = {
				id: editingDraft?.id ?? `${draftPrefix}${crypto.randomUUID()}`,
				displayName: normalizedName,
				host: normalizedHost,
				token: normalizedToken || editingDraft?.token || ''
			};
			pendingCreates = editingDraft
				? pendingCreates.map((item) => (item.id === draft.id ? draft : item))
				: [draft, ...pendingCreates];
		} else {
			const original = baseline.find((credential) => credential.id === editingCredential?.id);
			if (
				original &&
				original.displayName === normalizedName &&
				original.host === normalizedHost &&
				!normalizedToken
			) {
				delete pendingEdits[editingCredential.id];
			} else {
				pendingEdits = {
					...pendingEdits,
					[editingCredential.id]: {
						displayName: normalizedName,
						host: normalizedHost,
						token: normalizedToken
					}
				};
			}
		}
		closeDialog();
	}

	export function reset() {
		pendingCreates = [];
		pendingEdits = {};
		pendingDeletes = [];
		formError = '';
	}

	export async function save() {
		if (!isDirty || isReadonly || requestPending) return false;

		requestPending = true;
		const failedCreates: DraftCreate[] = [];
		const failedEdits: Record<string, DraftEdit> = {};
		const failedDeletes: string[] = [];
		const created: GitCredential[] = [];
		const deletedIds: string[] = [];
		const conflicted: Record<string, GitCredential> = {};
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const edited = new Map<string, GitCredential>();
		try {
			await Promise.all([
				...pendingCreates.map(async (draft) => {
					try {
						created.push(
							await AdminService.createGitCredential(
								{
									displayName: draft.displayName,
									host: draft.host,
									token: draft.token
								},
								{ dontLogErrors: true }
							)
						);
					} catch (error) {
						failedCreates.push(draft);
						errors.append(
							error instanceof Error
								? error.message
								: m.platform_settings_git_credentials_save_failed()
						);
					}
				}),
				...Object.entries(pendingEdits)
					.filter(([id]) => !pendingDeletes.includes(id))
					.map(async ([id, edit]) => {
						try {
							const input: GitCredentialManifest = {
								displayName: edit.displayName,
								host: edit.host
							};
							if (edit.token) input.token = edit.token;
							edited.set(
								id,
								await AdminService.updateGitCredential(id, input, { dontLogErrors: true })
							);
						} catch (error) {
							failedEdits[id] = edit;
							errors.append(
								error instanceof Error
									? error.message
									: m.platform_settings_git_credentials_save_failed()
							);
						}
					}),
				...pendingDeletes.map(async (id) => {
					try {
						await AdminService.deleteGitCredential(id, { dontLogErrors: true });
						deletedIds.push(id);
					} catch (error) {
						errors.append(m.platform_settings_git_credentials_delete_failed({ error: `${error}` }));
						if (conflictStatus(error)) {
							const current = baseline.find((credential) => credential.id === id);
							if (current) {
								conflicted[id] = {
									...current,
									uses: {
										skillRepositories: [
											{
												id: 'resource',
												displayName: m.platform_settings_git_credentials_unknown_resource()
											}
										],
										mcpCatalogs: [],
										systemMcpCatalogs: []
									}
								};
							}
							return;
						}
						failedDeletes.push(id);
					}
				})
			]);

			const next = [
				...created,
				...baseline
					.filter((credential) => !deletedIds.includes(credential.id))
					.map((credential) => conflicted[credential.id] ?? edited.get(credential.id) ?? credential)
			];
			baseline = next;
			gitCredentials = next;
			pendingCreates = failedCreates;
			pendingEdits = failedEdits;
			pendingDeletes = failedDeletes;
			const succeeded =
				failedCreates.length === 0 &&
				Object.keys(failedEdits).length === 0 &&
				failedDeletes.length === 0;
			if (succeeded && Object.keys(conflicted).length === 0) {
				success.add('Git credentials updated successfully.');
			}
			return succeeded;
		} finally {
			requestPending = false;
		}
	}
</script>

<Table
	data={tableData}
	fields={['displayName', 'host', 'usedBy']}
	headers={[
		{ title: m.core_name(), property: 'displayName' },
		{ title: m.platform_settings_git_credentials_col_host(), property: 'host' },
		{ title: m.platform_settings_git_credentials_col_used_by(), property: 'usedBy' }
	]}
	sortable={['displayName', 'host']}
	filterable={['displayName', 'host']}
	onClickRow={(row) => openEdit(row)}
	noDataMessage={m.platform_settings_git_credentials_no_data()}
>
	{#snippet onRenderColumn(field, credential)}
		{#if field === 'displayName'}
			<span class="flex items-center gap-2">
				{credential.displayName}
				{#if hasUses(credential)}
					<button
						aria-label={m.platform_settings_git_credentials_view_sources()}
						type="button"
						class="pill-warning border-warning/30 hover:border-warning/60 hover:bg-warning/20 focus-visible:ring-warning/40 cursor-pointer border transition-colors focus-visible:ring-2 focus-visible:outline-none"
						onclick={(event) => {
							event.stopPropagation();
							openUses(credential);
						}}
					>
						{m.platform_settings_git_credentials_in_use()}
					</button>
				{/if}
			</span>
		{:else if field === 'host'}
			{credential.host}
		{:else if field === 'usedBy'}
			{#if credential.usedBy}
				<button
					type="button"
					aria-label={m.platform_settings_git_credentials_view_sources()}
					class="text-left hover:underline"
					onclick={(event) => {
						event.stopPropagation();
						openUses(credential);
					}}
				>
					{credential.usedBy > 1
						? m.platform_settings_git_credentials_sources_other({ count: credential.usedBy })
						: m.platform_settings_git_credentials_sources_one({ count: credential.usedBy })}
				</button>
			{:else}
				<span class="text-muted-content">—</span>
			{/if}
		{/if}
	{/snippet}
	{#snippet actions(credential)}
		{#if !isReadonly}
			<IconButton
				aria-label={m.platform_settings_git_credentials_edit_aria()}
				onclick={(event) => {
					event.stopPropagation();
					openEdit(credential);
				}}
			>
				<Pencil class="size-4" />
			</IconButton>
			<div
				class="shrink-0"
				use:tooltip={hasUses(credential)
					? {
							text: m.platform_settings_git_credentials_in_use_cannot_delete(),
							placement: 'left'
						}
					: undefined}
			>
				<IconButton
					aria-label={m.platform_settings_git_credentials_delete_aria()}
					variant="danger"
					disabled={hasUses(credential)}
					onclick={(event) => {
						event.stopPropagation();
						stageDelete(credential);
					}}
				>
					<Trash2 class="size-4" />
				</IconButton>
			</div>
		{/if}
	{/snippet}
</Table>

<dialog bind:this={dialog} class="dialog" onclose={handleDialogClose}>
	<div class="dialog-container w-full max-w-md p-4">
		<h3 class="dialog-title">
			{editingCredential
				? m.platform_settings_git_credentials_edit_title()
				: m.platform_settings_add_git_credential()}
			<IconButton onclick={closeDialog} class="btn-sm dialog-close-btn">
				<X class="size-5" />
			</IconButton>
		</h3>

		<div class="my-4 flex flex-col gap-4">
			{@render credentialForm()}
		</div>

		{#if formError}
			<div class="mb-4 flex items-start gap-2 text-error">
				<TriangleAlert class="size-5 shrink-0" />
				<span class="text-sm break-all">{formError}</span>
			</div>
		{/if}

		<div class="flex justify-end gap-2">
			<button class="btn btn-secondary" disabled={inputsDisabled} onclick={closeDialog}
				>{m.common_cancel()}</button
			>
			<button
				class="btn btn-primary"
				disabled={inputsDisabled ||
					!displayName.trim() ||
					!host.trim() ||
					(tokenRequired && !token.trim())}
				onclick={stageCredential}
			>
				{editingPersisted ? m.core_update() : m.platform_add()}
			</button>
		</div>
	</div>
	<form class="dialog-backdrop">
		<button type="button" onclick={closeDialog}>{m.common_close()}</button>
	</form>
</dialog>

{#snippet credentialForm()}
	<div class="flex flex-col gap-1">
		<label for="git-credential-name" class="text-sm font-light">{m.core_name()}</label>
		<input
			id="git-credential-name"
			bind:value={displayName}
			disabled={inputsDisabled}
			class="text-input-filled"
		/>
	</div>
	<div class="flex flex-col gap-1">
		<label for="git-credential-host" class="text-sm font-light"
			>{m.platform_settings_git_credentials_host()}</label
		>
		<input
			id="git-credential-host"
			bind:value={host}
			disabled={inputsDisabled || editingPersisted}
			placeholder="github.com"
			class="text-input-filled"
		/>
		<span class="text-muted-content text-xs">{m.platform_settings_git_credentials_host_hint()}</span
		>
	</div>
	<div class="flex flex-col gap-1">
		<div class="flex items-center justify-between gap-4">
			<label for="git-credential-token" class="text-sm font-light"
				>{m.platform_settings_git_credentials_pat()}</label
			>
			{#if showExistingToken && !inputsDisabled}
				<button
					type="button"
					class="text-xs text-error hover:underline"
					onclick={() => {
						clearToken = true;
						token = '';
					}}
				>
					{m.platform_settings_git_credentials_clear_token()}
				</button>
			{/if}
		</div>
		{#if showExistingToken}
			<input
				id="git-credential-token"
				type="text"
				readonly
				aria-readonly="true"
				data-1p-ignore
				value="****"
				class="text-muted-content min-h-10 w-full border-none bg-transparent p-0 text-sm outline-none focus:ring-0"
			/>
		{:else}
			<SensitiveInput name="git-credential-token" bind:value={token} disabled={inputsDisabled} />
		{/if}
	</div>
{/snippet}

{#snippet useSections(credential: GitCredential)}
	{#each useGroups(credential) as group (group.label)}
		<section class="flex flex-col gap-1">
			<h4 class="text-xs font-semibold">{group.label}</h4>
			<ul class="bg-surface-1 divide-border divide-y rounded-md border">
				{#each group.uses as use (`${use.id}:${use.displayName ?? ''}`)}
					<li class="px-3 py-2 text-sm break-all">{use.displayName || use.id}</li>
				{/each}
			</ul>
		</section>
	{/each}
{/snippet}

{#snippet usesNote()}
	{#if viewingCredential}
		<div class="flex w-full flex-col gap-2 text-left">
			{@render useSections(viewingCredential)}
		</div>
	{/if}
{/snippet}

<Confirm
	title={m.platform_settings_git_credentials_credential_uses()}
	msg={m.platform_settings_git_credentials_used_by_msg({
		name: viewingCredential?.displayName ?? m.platform_settings_git_credentials_this_credential()
	})}
	note={usesNote}
	type="info"
	show={Boolean(viewingCredential)}
	cancelText={m.core_close()}
	oncancel={() => (viewingCredential = undefined)}
	classes={{ note: 'w-full' }}
/>
