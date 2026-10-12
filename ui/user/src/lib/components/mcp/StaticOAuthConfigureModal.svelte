<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import type { MCPServerOAuthCredentialStatus } from '$lib/services/admin/types';
	import Confirm from '../Confirm.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';
	import McpDeprecatedNotice from './McpDeprecatedNotice.svelte';
	import { CircleAlert, Trash2 } from '@lucide/svelte';

	interface Props {
		oauthStatus?: MCPServerOAuthCredentialStatus;
		onSave: (credentials: { clientID: string; clientSecret: string }) => Promise<void>;
		onDelete?: () => Promise<void>;
		onSkip?: () => void;
		onCancel?: () => void;
		showSkip?: boolean;
		deprecated?: boolean;
	}

	let {
		oauthStatus,
		onSave,
		onDelete,
		onSkip,
		onCancel,
		showSkip = false,
		deprecated
	}: Props = $props();

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
			error = m.mcps_oauth_static_oauth_already_configured();
			return;
		}

		// Client ID is required; public OAuth clients do not have a client secret.
		if (!form.clientID.trim()) {
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
			error = err instanceof Error ? err.message : m.mcps_oauth_static_oauth_save_failed();
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
			error = err instanceof Error ? err.message : m.mcps_oauth_static_oauth_delete_failed();
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
	title={m.mcps_oauth_static_oauth_title()}
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
				<CircleAlert class="size-6 text-error" />
				<p class="text-sm font-light">{error}</p>
			</div>
		{/if}

		<McpDeprecatedNotice {deprecated} variant="notification" />

		{#if oauthStatus?.configured}
			<p class="text-muted-content text-sm font-light">
				{m.mcps_oauth_static_oauth_configured_description()}
			</p>
		{:else}
			<p class="text-muted-content text-sm font-light">
				{m.mcps_oauth_static_oauth_required_description()}
			</p>
		{/if}

		<div class="flex flex-col gap-4">
			<div class="flex flex-col gap-1">
				<label for="clientID" class:text-error={showRequired && !form.clientID}>
					{m.mcps_oauth_static_oauth_client_id()}
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
					autocomplete="off"
				/>
			</div>

			<div class="flex flex-col gap-1">
				<label for="clientSecret">{m.mcps_oauth_static_oauth_client_secret_optional()}</label>
				<SensitiveInput
					name="clientSecret"
					bind:value={form.clientSecret}
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
				class="btn btn-error flex items-center gap-1"
				onclick={() => {
					dialog?.close();
					showDeleteConfirm = true;
				}}
				disabled={loading}
			>
				<Trash2 class="size-4" />
				{m.mcps_oauth_static_oauth_clear_credentials()}
			</button>
		{:else}
			<div></div>
		{/if}

		{#if !oauthStatus?.configured}
			<div class="flex gap-2">
				{#if showSkip}
					<button type="button" class="btn btn-secondary" onclick={handleSkip} disabled={loading}>
						{m.mcps_skip()}
					</button>
				{/if}
				<button type="button" class="btn btn-secondary" onclick={handleCancel} disabled={loading}>
					{m.common_cancel()}
				</button>
				<button type="button" class="btn btn-primary" onclick={handleSave} disabled={loading}>
					{#if loading}
						<Loading class="size-4" />
					{:else}
						{m.core_save()}
					{/if}
				</button>
			</div>
		{/if}
	</div>
</ResponsiveDialog>

<Confirm
	show={showDeleteConfirm}
	msg={m.mcps_oauth_static_oauth_clear_confirm()}
	onsuccess={handleDelete}
	oncancel={() => {
		showDeleteConfirm = false;
		dialog?.open();
	}}
	{loading}
/>
