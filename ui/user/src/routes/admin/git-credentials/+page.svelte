<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { AdminService, type GitCredential, type GitCredentialManifest } from '$lib/services';
	import { profile } from '$lib/stores';
	import { KeyRound, Plus, Trash2, TriangleAlert, X } from '@lucide/svelte';
	import { untrack } from 'svelte';

	const { data }: { data: { gitCredentials: GitCredential[] } } = $props();

	let gitCredentials = $state<GitCredential[]>(untrack(() => data.gitCredentials));
	let editingCredential = $state<GitCredential>();
	let deletingCredential = $state<GitCredential>();
	let displayName = $state('');
	let host = $state('');
	let token = $state('');
	let formError = $state('');
	let saving = $state(false);
	let dialog = $state<HTMLDialogElement>();
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let tokenRequired = $derived(!editingCredential?.tokenConfigured);

	let tableData = $derived(
		gitCredentials.map((credential) => ({
			...credential,
			status: credential.tokenConfigured ? 'Configured' : 'Missing token'
		}))
	);

	function openCreate() {
		editingCredential = undefined;
		displayName = '';
		host = '';
		token = '';
		formError = '';
		dialog?.showModal();
	}

	function openEdit(credential: GitCredential) {
		editingCredential = credential;
		displayName = credential.displayName;
		host = credential.host;
		token = '';
		formError = '';
		dialog?.showModal();
	}

	function closeDialog() {
		dialog?.close();
		formError = '';
	}

	async function saveCredential() {
		if (isReadonly || !displayName.trim() || !host.trim() || (tokenRequired && !token)) {
			return;
		}

		saving = true;
		formError = '';
		try {
			const input: GitCredentialManifest = {
				displayName: displayName.trim(),
				host: host.trim()
			};
			if (token) input.token = token;

			const saved = editingCredential
				? await AdminService.updateGitCredential(editingCredential.id, input, {
						dontLogErrors: true
					})
				: await AdminService.createGitCredential(input, { dontLogErrors: true });

			const index = gitCredentials.findIndex((credential) => credential.id === saved.id);
			gitCredentials =
				index === -1
					? [saved, ...gitCredentials]
					: gitCredentials.map((credential) => (credential.id === saved.id ? saved : credential));
			closeDialog();
		} catch (error) {
			formError = error instanceof Error ? error.message : 'Unable to save Git credential';
		} finally {
			saving = false;
		}
	}
</script>

<Layout title="Git Credentials" showBackButton>
	{#if gitCredentials.length === 0}
		<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
			<KeyRound class="text-muted-content size-24 opacity-25" />
			<h4 class="text-muted-content text-lg font-semibold">No Git credentials</h4>
			<p class="text-muted-content text-sm font-light">
				Create a host-bound credential to reuse a personal access token across Git repositories.
			</p>
			{#if !isReadonly}
				<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreate}>
					<Plus class="size-4" />
					Create Git Credential
				</button>
			{/if}
		</div>
	{:else}
		<Table
			data={tableData}
			fields={['displayName', 'host', 'status', 'id']}
			headers={[
				{ title: 'Name', property: 'displayName' },
				{ title: 'Host', property: 'host' },
				{ title: 'Token', property: 'status' },
				{ title: 'Credential', property: 'id' }
			]}
			sortable={['displayName', 'host', 'status']}
			filterable={['displayName', 'host']}
			onClickRow={(row) => openEdit(row)}
		>
			{#snippet actions(credential)}
				<DotDotDot ariaLabel={`Actions for ${credential.displayName}`}>
					{#snippet children({ toggle })}
						<button
							class="menu-button-destructive"
							disabled={isReadonly}
							onclick={(event) => {
								event.stopPropagation();
								deletingCredential = credential;
								toggle(false);
							}}
						>
							<Trash2 class="size-4" />
							Delete
						</button>
					{/snippet}
				</DotDotDot>
			{/snippet}
		</Table>
	{/if}

	{#snippet rightNavActions()}
		{#if !isReadonly && gitCredentials.length > 0}
			<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreate}>
				<Plus class="size-4" />
				Create Git Credential
			</button>
		{/if}
	{/snippet}
</Layout>

<dialog bind:this={dialog} class="dialog">
	<div class="dialog-container w-full max-w-md p-4">
		<h3 class="dialog-title">
			{editingCredential ? 'Edit Git Credential' : 'Create Git Credential'}
			<IconButton onclick={closeDialog} class="btn-sm dialog-close-btn">
				<X class="size-5" />
			</IconButton>
		</h3>

		<div class="my-4 flex flex-col gap-4">
			<div class="flex flex-col gap-1">
				<label for="git-credential-name" class="text-sm font-light">Name</label>
				<input
					id="git-credential-name"
					bind:value={displayName}
					disabled={isReadonly}
					class="text-input-filled"
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label for="git-credential-host" class="text-sm font-light">Git host</label>
				<input
					id="git-credential-host"
					bind:value={host}
					disabled={isReadonly || Boolean(editingCredential)}
					placeholder="github.com"
					class="text-input-filled"
				/>
				<span class="text-muted-content text-xs">Enter a hostname without a scheme or path.</span>
			</div>
			<div class="flex flex-col gap-1">
				<label for="git-credential-token" class="text-sm font-light">
					Personal access token {editingCredential?.tokenConfigured
						? '(leave blank to keep current)'
						: ''}
				</label>
				<SensitiveInput name="git-credential-token" bind:value={token} disabled={isReadonly} />
			</div>
		</div>

		{#if formError}
			<div class="mb-4 flex items-start gap-2 text-error">
				<TriangleAlert class="size-5 shrink-0" />
				<span class="text-sm break-all">{formError}</span>
			</div>
		{/if}

		<div class="flex justify-end gap-2">
			<button class="btn btn-secondary" disabled={saving} onclick={closeDialog}>Cancel</button>
			<button
				class="btn btn-primary"
				disabled={isReadonly ||
					saving ||
					!displayName.trim() ||
					!host.trim() ||
					(tokenRequired && !token)}
				onclick={saveCredential}
			>
				{saving ? 'Saving...' : editingCredential ? 'Save' : 'Create'}
			</button>
		</div>
	</div>
	<form class="dialog-backdrop">
		<button type="button" onclick={closeDialog}>close</button>
	</form>
</dialog>

<Confirm
	msg={`Delete ${deletingCredential?.displayName ?? 'this Git credential'}?`}
	note="Credentials in use by a skill repository or MCP catalog cannot be deleted."
	show={Boolean(deletingCredential)}
	loading={saving}
	onsuccess={async () => {
		if (!deletingCredential) return;
		saving = true;
		try {
			await AdminService.deleteGitCredential(deletingCredential.id);
			gitCredentials = gitCredentials.filter(
				(credential) => credential.id !== deletingCredential?.id
			);
			deletingCredential = undefined;
		} finally {
			saving = false;
		}
	}}
	oncancel={() => (deletingCredential = undefined)}
/>
