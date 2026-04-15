<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { AdminService, type MCPCatalog } from '$lib/services';
	import { Info, TriangleAlert, X } from 'lucide-svelte';

	interface Props {
		defaultCatalog?: MCPCatalog;
		defaultCatalogId?: string;
		onSync?: () => void;
	}

	let { defaultCatalog, onSync, defaultCatalogId }: Props = $props();

	let saving = $state(false);
	let sourceError = $state<string>();
	let editingSource = $state<{ index: number; value: string; token?: string }>();
	let sourceDialog = $state<HTMLDialogElement>();

	export function open() {
		sourceError = undefined;
		editingSource = {
			index: -1,
			value: '',
			token: ''
		};
		sourceDialog?.showModal();
	}

	export function edit(url: string, index: number) {
		sourceError = undefined;
		editingSource = {
			index,
			value: url,
			token: undefined
		};
		sourceDialog?.showModal();
	}

	function closeSourceDialog() {
		editingSource = undefined;
		sourceError = undefined;
		sourceDialog?.close();
	}
</script>

<dialog bind:this={sourceDialog} class="dialog">
	<div class="dialog-container w-full max-w-md p-4">
		{#if editingSource}
			<h3 class="dialog-title">
				{editingSource.index === -1 ? 'Add Source URL' : 'Edit Source URL'}
				<button onclick={closeSourceDialog} class="icon-button dialog-close-btn">
					<X class="size-5" />
				</button>
			</h3>

			<div class="my-4 flex flex-col gap-1">
				<label for="catalog-source-name" class="flex flex-1 items-center gap-1 text-sm font-light">
					Source URL
					<span
						use:tooltip={{
							text: 'Supported formats:\n• https://github.com/org/repo\n• https://gitlab.com/org/repo\n• https://gitlab.com/group/subgroup/repo.git\n• https://self-hosted.example.com/org/repo.git\n\nFor GitHub and GitLab a .git suffix is optional. For self-hosted instances it is required.',
							classes: ['max-w-xs', 'whitespace-pre-line'],
							disablePortal: true
						}}
					>
						<Info class="text-surface3 size-3.5" />
					</span>
				</label>
				<input
					id="catalog-source-name"
					bind:value={editingSource.value}
					class="text-input-filled"
				/>
			</div>

			<div class="mb-4 flex flex-col gap-1">
				<label for="catalog-source-token" class="flex-1 text-sm font-light"
					>Personal access token (optional)
				</label>
				<input
					id="catalog-source-token"
					type="password"
					placeholder={defaultCatalog?.sourceURLCredentials?.[editingSource.value]
						? 'Token is set — enter a new value to replace it'
						: ''}
					bind:value={editingSource.token}
					class="text-input-filled"
				/>
			</div>

			{#if sourceError}
				<div class="mb-4 flex flex-col gap-2 text-red-500 dark:text-red-400">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-6 shrink-0 self-start" />
						<p class="my-0.5 flex flex-col text-sm font-semibold">Error adding source URL:</p>
					</div>
					<span class="font-sm font-light break-all">{sourceError}</span>
				</div>
			{/if}

			<div class="flex w-full justify-end gap-2">
				<button class="button" disabled={saving} onclick={closeSourceDialog}>Cancel</button>
				<button
					class="button-primary"
					disabled={saving}
					onclick={async () => {
						if (!editingSource || (!defaultCatalog && !defaultCatalogId)) {
							return;
						}

						let catalogToUse = defaultCatalog;
						if (!catalogToUse && defaultCatalogId) {
							catalogToUse = await AdminService.getMCPCatalog(defaultCatalogId);
						}

						if (!catalogToUse) {
							sourceError = 'Failed to fetch catalog';
							return;
						}

						saving = true;
						sourceError = undefined;

						try {
							const updatingCatalog = { ...catalogToUse };

							if (editingSource.index === -1) {
								updatingCatalog.sourceURLs = [
									...(updatingCatalog.sourceURLs ?? []),
									editingSource.value
								];
							} else {
								updatingCatalog.sourceURLs[editingSource.index] = editingSource.value;
							}

							// Only send a credential update when the user has typed a value.
							// Leaving the field empty (add or edit) leaves any existing credential unchanged.
							if (editingSource.token) {
								updatingCatalog.sourceURLCredentials = {
									...(updatingCatalog.sourceURLCredentials ?? {}),
									[editingSource.value]: editingSource.token
								};
							}

							const response = await AdminService.updateMCPCatalog(
								catalogToUse.id,
								updatingCatalog,
								{
									dontLogErrors: true
								}
							);
							defaultCatalog = response;
							await onSync?.();
							closeSourceDialog();
						} catch (error) {
							sourceError = error instanceof Error ? error.message : 'An unexpected error occurred';
						} finally {
							saving = false;
						}
					}}
				>
					{editingSource.index === -1 ? 'Add' : 'Save'}
				</button>
			</div>
		{/if}
	</div>
	<form class="dialog-backdrop">
		<button type="button" onclick={closeSourceDialog}>close</button>
	</form>
</dialog>
