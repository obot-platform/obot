<script lang="ts">
	import type { MCPServerOAuthCredentialStatus } from '$lib/services/admin/types';
	import { LoaderCircle, AlertCircle, Trash2 } from 'lucide-svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';
	import Confirm from '../Confirm.svelte';

	interface Props {
		oauthStatus?: MCPServerOAuthCredentialStatus;
		onSave: (credentials: { clientID: string; clientSecret: string }) => Promise<void>;
		onDelete?: () => Promise<void>;
		onSkip?: () => void;
		onCancel?: () => void;
		showSkip?: boolean;
	}

	let { oauthStatus, onSave, onDelete, onSkip, onCancel, showSkip = false }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let loading = $state(false);
	let error = $state<string>();
	let showDeleteConfirm = $state(false);
	let showRequired = $state(false);

	let form = $state({
		clientID: '',
		clientSecret: ''
	});

	function onOpen() {
		form = {
			clientID: oauthStatus?.clientID ?? '',
			clientSecret: ''
		};
		showRequired = false;
		error = undefined;
	}

	function onClose() {
		form = { clientID: '', clientSecret: '' };
		showRequired = false;
		error = undefined;
	}

	export function open() {
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}

	async function handleSave() {
		showRequired = false;
		error = undefined;

		// Credentials cannot be updated once configured - must delete and recreate
		if (oauthStatus?.configured) {
			error = 'Credentials already configured. Clear credentials first to change them.';
			return;
		}

		// Initial setup: Validate all required fields
		if (!form.clientID.trim()) {
			showRequired = true;
			return;
		}
		if (!form.clientSecret.trim()) {
			showRequired = true;
			return;
		}

		loading = true;
		try {
			await onSave({
				clientID: form.clientID.trim(),
				clientSecret: form.clientSecret.trim()
			});
			dialog?.close();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to save OAuth credentials';
		} finally {
			loading = false;
		}
	}

	async function handleDelete() {
		if (!onDelete) return;
		loading = true;
		try {
			await onDelete();
			showDeleteConfirm = false;
			dialog?.close();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to delete OAuth credentials';
		} finally {
			loading = false;
		}
	}

	function handleSkip() {
		onSkip?.();
		dialog?.close();
	}

	function handleCancel() {
		onCancel?.();
		dialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	{onOpen}
	{onClose}
	title="Configure Static OAuth"
	classes={{ header: 'p-4 pb-0', content: 'p-0' }}
>
	<form
		class="default-scrollbar-thin flex max-h-[70vh] flex-col gap-4 overflow-y-auto p-4 pt-2"
		onsubmit={(e) => {
			e.preventDefault();
			handleSave();
		}}
	>
		{#if error}
			<div class="notification-error flex items-center gap-2">
				<AlertCircle class="size-6 text-red-500" />
				<p class="text-sm font-light">{error}</p>
			</div>
		{/if}

		{#if oauthStatus?.configured}
			<p class="text-on-surface1 text-sm font-light">
				OAuth credentials are configured. To change the client ID or secret, clear the credentials
				and re-enter all values.
			</p>
		{:else}
			<p class="text-on-surface1 text-sm font-light">
				This remote MCP server requires OAuth configuration. Provide the client credentials from
				your OAuth provider.
			</p>
		{/if}

		<div class="flex flex-col gap-4">
			<div class="flex flex-col gap-1">
				<label for="clientID" class:text-red-500={showRequired && !form.clientID}>
					Client ID
				</label>
				<input
					type="text"
					id="clientID"
					bind:value={form.clientID}
					class="text-input-filled"
					class:error={showRequired && !form.clientID}
					class:opacity-60={oauthStatus?.configured}
					placeholder="your-client-id"
					readonly={oauthStatus?.configured}
				/>
			</div>

			<div class="flex flex-col gap-1">
				<label for="clientSecret" class:text-red-500={showRequired && !form.clientSecret}>
					Client Secret
				</label>
				<SensitiveInput
					name="clientSecret"
					bind:value={form.clientSecret}
					error={showRequired && !form.clientSecret}
					placeholder={oauthStatus?.configured ? '••••••••' : 'your-client-secret'}
					readonly={oauthStatus?.configured}
					classes={{ input: oauthStatus?.configured ? 'opacity-60' : '' }}
				/>
			</div>
		</div>
	</form>

	<div class="flex flex-col gap-2 p-4 pt-0 md:flex-row md:justify-between">
		{#if oauthStatus?.configured && onDelete}
			<button
				type="button"
				class="button-destructive flex items-center gap-1"
				onclick={() => {
					dialog?.close();
					showDeleteConfirm = true;
				}}
				disabled={loading}
			>
				<Trash2 class="size-4" />
				Clear Credentials
			</button>
		{:else}
			<div></div>
		{/if}

		{#if !oauthStatus?.configured}
			<div class="flex gap-2">
				{#if showSkip}
					<button type="button" class="button" onclick={handleSkip} disabled={loading}>
						Skip
					</button>
				{/if}
				<button type="button" class="button" onclick={handleCancel} disabled={loading}>
					Cancel
				</button>
				<button type="button" class="button-primary" onclick={handleSave} disabled={loading}>
					{#if loading}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						Save
					{/if}
				</button>
			</div>
		{/if}
	</div>
</ResponsiveDialog>

<Confirm
	show={showDeleteConfirm}
	msg="Are you sure you want to clear the OAuth credentials? Users will not be able to connect to this server until new credentials are configured."
	onsuccess={handleDelete}
	oncancel={() => {
		showDeleteConfirm = false;
		dialog?.open();
	}}
	{loading}
/>
