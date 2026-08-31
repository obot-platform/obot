<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import AgentIcon from '$lib/components/hosted-agents/AgentIcon.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import Loading from '$lib/icons/Loading.svelte';
	import { type Harness } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { errors, profile } from '$lib/stores/index.js';
	import { Cpu, Pencil, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		harnesses: Harness[];
	}

	let { harnesses: initialHarnesses }: Props = $props();
	let harnesses = $state(untrack(() => initialHarnesses));
	let harnessToDelete = $state<Harness>();
	let harnessDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editingHarness = $state<Harness | undefined>();
	let savingHarness = $state(false);
	let harnessForm = $state({ name: '', description: '', icon: '', iconDark: '', image: '' });
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	const duration = PAGE_TRANSITION_DURATION;

	let harnessTableData = $derived(
		harnesses.map((harness) => ({
			id: harness.id,
			name: harness.name,
			description: harness.description ?? '',
			image: harness.image,
			icon: harness.icon ?? '',
			iconDark: harness.iconDark ?? ''
		}))
	);

	let canSaveHarness = $derived(Boolean(harnessForm.name && harnessForm.image));

	export function openCreate() {
		openCreateHarness();
	}

	function openCreateHarness() {
		editingHarness = undefined;
		harnessForm = { name: '', description: '', icon: '', iconDark: '', image: '' };
		harnessDialog?.open();
	}

	function openEditHarness(harness: Harness) {
		editingHarness = harness;
		harnessForm = {
			name: harness.name,
			description: harness.description ?? '',
			icon: harness.icon ?? '',
			iconDark: harness.iconDark ?? '',
			image: harness.image
		};
		harnessDialog?.open();
	}

	async function saveHarness() {
		savingHarness = true;
		try {
			const manifest = {
				name: harnessForm.name,
				description: harnessForm.description,
				icon: harnessForm.icon,
				iconDark: harnessForm.iconDark,
				image: harnessForm.image
			};
			if (editingHarness) {
				await AdminService.updateHarness(editingHarness.id, manifest);
			} else {
				await AdminService.createHarness(manifest);
			}
			harnesses = await AdminService.listHarnesses();
			harnessDialog?.close();
		} catch (err) {
			errors.append(`Failed to save harness: ${err}`);
		} finally {
			savingHarness = false;
		}
	}
</script>

<div class="flex flex-col gap-4" in:fade={{ duration }}>
	<p class="text-muted-content text-sm font-light">
		A harness is the container image an agent runs inside — its runtime and preinstalled tooling,
		such as Claude Code. Templates pick a harness; the harness decides the image and whether the
		agent gets an interactive shell.
	</p>

	{#if harnesses.length === 0}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<Cpu class="text-muted-content size-24 opacity-25" />
			<h4 class="text-muted-content text-lg font-semibold">No harnesses</h4>
			{#if !isReadonly}
				<p class="text-muted-content text-sm font-light">
					Add one before registering a template, which has to pick a harness.
				</p>
				<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreateHarness}>
					<Plus class="size-4" /> Add Harness
				</button>
			{/if}
		</div>
	{:else}
		<Table
			data={harnessTableData}
			fields={['name', 'description', 'image']}
			headers={[
				{ property: 'name', title: 'Name' },
				{ property: 'description', title: 'Description' },
				{ property: 'image', title: 'Image' }
			]}
			sortable={['name', 'image']}
			noDataMessage="No harnesses added."
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'name'}
					<div class="flex items-center gap-2">
						<AgentIcon icon={d.icon} iconDark={d.iconDark} alt="" />
						<span class="truncate">{d.name}</span>
					</div>
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				{#if !isReadonly}
					<IconButton
						onclick={(e) => {
							e.stopPropagation();
							const harness = harnesses.find((h) => h.id === d.id);
							if (harness) openEditHarness(harness);
						}}
						tooltip={{ text: 'Edit Harness' }}
					>
						<Pencil class="size-4" />
					</IconButton>
					<IconButton
						variant="danger"
						onclick={(e) => {
							e.stopPropagation();
							harnessToDelete = harnesses.find((h) => h.id === d.id);
						}}
						tooltip={{ text: 'Delete Harness' }}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			{/snippet}
		</Table>
	{/if}
</div>

<ResponsiveDialog
	bind:this={harnessDialog}
	title={editingHarness ? 'Edit Harness' : 'Add Harness'}
	class="md:max-w-md"
>
	<div class="flex flex-col gap-4">
		<div class="flex flex-col gap-2">
			<label for="harness-name" class="text-sm font-light">Name</label>
			<input
				id="harness-name"
				bind:value={harnessForm.name}
				class="text-input-filled"
				placeholder="Claude Code"
			/>
		</div>
		<div class="flex flex-col gap-2">
			<label for="harness-description" class="text-sm font-light">Description</label>
			<textarea
				id="harness-description"
				bind:value={harnessForm.description}
				class="text-input-filled"
				rows="2"
			></textarea>
		</div>
		<div class="flex flex-col gap-2">
			<label for="harness-image" class="text-sm font-light">Docker Image</label>
			<input
				id="harness-image"
				bind:value={harnessForm.image}
				class="text-input-filled"
				placeholder="ghcr.io/example/claude-code:latest"
				autocomplete="off"
			/>
		</div>
		<div class="flex flex-col gap-2">
			<label for="harness-icon" class="text-sm font-light">Icon URL</label>
			<div class="flex items-center gap-3">
				{#if harnessForm.icon}
					<img src={harnessForm.icon} alt="" class="size-10 shrink-0 rounded-md object-contain" />
				{/if}
				<input
					type="text"
					id="harness-icon"
					bind:value={harnessForm.icon}
					class="text-input-filled grow"
					inputmode="url"
					autocomplete="off"
				/>
			</div>
		</div>
		<div class="flex flex-col gap-2">
			<label for="harness-icon-dark" class="text-sm font-light">Icon URL (Dark)</label>
			<div class="flex items-center gap-3">
				{#if harnessForm.iconDark}
					<img
						src={harnessForm.iconDark}
						alt=""
						class="bg-base-300 size-10 shrink-0 rounded-md object-contain"
					/>
				{/if}
				<input
					type="text"
					id="harness-icon-dark"
					bind:value={harnessForm.iconDark}
					class="text-input-filled grow"
					inputmode="url"
					autocomplete="off"
				/>
			</div>
		</div>
	</div>
	<div class="flex justify-end gap-2 pt-4">
		<button class="btn btn-secondary text-sm" onclick={() => harnessDialog?.close()}>Cancel</button>
		<button
			class="btn btn-primary text-sm"
			disabled={!canSaveHarness || savingHarness}
			onclick={saveHarness}
		>
			{#if savingHarness}
				<Loading class="size-4" />
			{:else}
				{editingHarness ? 'Update' : 'Add'}
			{/if}
		</button>
	</div>
</ResponsiveDialog>

<Confirm
	msg={`Delete ${harnessToDelete?.name || 'this harness'}?`}
	note="A harness that agents still run on cannot be deleted."
	show={Boolean(harnessToDelete)}
	onsuccess={async () => {
		if (!harnessToDelete) return;
		try {
			await AdminService.deleteHarness(harnessToDelete.id);
			harnesses = await AdminService.listHarnesses();
		} catch (err) {
			errors.append(`Failed to delete harness: ${err}`);
		}
		harnessToDelete = undefined;
	}}
	oncancel={() => (harnessToDelete = undefined)}
/>
