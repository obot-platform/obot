<script lang="ts">
	import { LoaderCircle, Server } from 'lucide-svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { ChatService, type MCPCatalogServer } from '$lib/services';
	import { errors } from '$lib/stores';

	interface Props {
		server?: MCPCatalogServer;
		onUpdateConfigure?: () => void;
	}

	let { server, onUpdateConfigure }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let newName = $state('');
	let originalName = $state('');
	let saving = $state(false);

	export function open() {
		const name = server?.alias || server?.manifest?.name || '';
		newName = name;
		originalName = name;
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}

	async function handleSave() {
		const trimmedName = newName.trim();
		if (!server?.id || !trimmedName || trimmedName === originalName) return;

		try {
			saving = true;
			await ChatService.updateSingleOrRemoteMcpServerAlias(server.id, trimmedName);
			dialog?.close();
			onUpdateConfigure?.();
		} catch (err) {
			errors.append(`Failed to update server alias: ${err}`);
		} finally {
			saving = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	animate="slide"
	onClose={() => {
		newName = originalName;
		saving = false;
	}}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2">
			<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
				{#if server?.manifest?.icon}
					<img
						src={server.manifest.icon}
						alt={newName || server?.alias || server?.manifest?.name}
						class="size-8"
					/>
				{:else}
					<Server class="size-8" />
				{/if}
			</div>
			{newName || server?.alias || server?.manifest?.name || 'Server'}
		</div>
	{/snippet}

	<form
		onsubmit={(e) => {
			e.preventDefault();
			handleSave();
		}}
	>
		<div class="my-4 flex flex-col gap-4">
			<div class="flex flex-col gap-1">
				<label for="serverName" class="text-sm font-medium">Server Alias</label>
				<input
					type="text"
					id="serverName"
					bind:value={newName}
					class="text-input-filled"
					placeholder="Enter server alias..."
				/>
			</div>
		</div>
	</form>

	<div class="flex justify-end gap-2">
		<button
			class="button-primary"
			onclick={handleSave}
			disabled={saving || !newName.trim() || newName.trim() === originalName}
		>
			{#if saving}
				<LoaderCircle class="size-4 animate-spin" />
			{:else}
				Update
			{/if}
		</button>
	</div>
</ResponsiveDialog>
