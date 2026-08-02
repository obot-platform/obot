<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import type {
		MCPServerOAuthCredentialStatus,
		MCPServerOAuthCredentialTestRequest,
		MCPServerOAuthCredentialTestResult,
		MCPServerOAuthCredentialTestStart
	} from '$lib/services/admin/types';
	import { poll } from '$lib/utils';
	import Confirm from '../Confirm.svelte';
	import CopyField from '../CopyField.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';
	import McpDeprecatedNotice from './McpDeprecatedNotice.svelte';
	import {
		beginStaticOAuthCredentialTest,
		canSaveStaticOAuthCredentials,
		failStaticOAuthCredentialTest,
		idleStaticOAuthCredentialTest,
		invalidateStaticOAuthCredentialTest,
		safeStaticOAuthAuthorizationURL,
		scheduleStaticOAuthCredentialTestExpiry,
		succeedStaticOAuthCredentialTest
	} from './staticOAuthCredentialTestState';
	import { CircleAlert, CircleCheckBig, ExternalLink, Trash2 } from '@lucide/svelte';
	import { onDestroy } from 'svelte';

	interface Props {
		oauthStatus?: MCPServerOAuthCredentialStatus;
		onStartTest: (
			credentials: MCPServerOAuthCredentialTestRequest
		) => Promise<MCPServerOAuthCredentialTestStart>;
		onGetTest: (testState: string) => Promise<MCPServerOAuthCredentialTestResult>;
		onSave: (credentials: {
			clientID: string;
			clientSecret: string;
			proof: string;
		}) => Promise<void>;
		onDelete?: (expectedGeneration: string) => Promise<void>;
		onSkip?: () => void;
		onCancel?: () => void;
		showSkip?: boolean;
		deprecated?: boolean;
	}

	let {
		oauthStatus,
		onStartTest,
		onGetTest,
		onSave,
		onDelete,
		onSkip,
		onCancel,
		showSkip = false,
		deprecated
	}: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let loading = $state(false);
	let testing = $state(false);
	let error = $state<string>();
	let showDeleteConfirm = $state(false);
	let showRequired = $state(false);
	let credentialTest = $state(idleStaticOAuthCredentialTest());
	let testGeneration = 0;
	let testPopup: Window | null = null;
	let cancelTestExpiry: (() => void) | undefined;

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
		credentialTest = idleStaticOAuthCredentialTest();
	}

	function onClose() {
		resetCredentialTest();
		form = { clientID: '', clientSecret: '' };
		showRequired = false;
		error = undefined;
	}

	function closeTestPopup() {
		if (testPopup && !testPopup.closed) {
			testPopup.close();
		}
		testPopup = null;
	}

	function resetCredentialTest() {
		testGeneration += 1;
		testing = false;
		cancelTestExpiry?.();
		cancelTestExpiry = undefined;
		credentialTest = invalidateStaticOAuthCredentialTest(credentialTest);
		closeTestPopup();
	}

	function handleCredentialInput() {
		showRequired = false;
		error = undefined;
		resetCredentialTest();
	}

	function credentialTestFailureMessage(category?: string): string {
		switch (category) {
			case 'authorization_denied':
				return 'Authorization was denied. Test the credentials again to continue.';
			case 'invalid_callback':
				return 'The OAuth callback was invalid. Check the provider callback URL and test again.';
			case 'token_exchange_failed':
				return 'The provider rejected the client credentials. Check them and test again.';
			case 'expired':
				return 'The credential test expired. Test the credentials again to continue.';
			default:
				return 'The OAuth credential test failed. Test the credentials again to continue.';
		}
	}

	async function handleTest() {
		showRequired = false;
		error = undefined;
		if (!form.clientID.trim() || !form.clientSecret.trim()) {
			showRequired = true;
			return;
		}

		const popup = window.open('', '_blank');
		if (!popup) {
			error = 'Allow pop-ups for Obot to test the OAuth credentials.';
			return;
		}
		popup.opener = null;
		testPopup = popup;

		const generation = ++testGeneration;
		testing = true;
		credentialTest = beginStaticOAuthCredentialTest(form.clientID, form.clientSecret);
		try {
			const started = await onStartTest({
				clientID: form.clientID,
				clientSecret: form.clientSecret
			});
			if (generation !== testGeneration) return;
			const oauthURL = safeStaticOAuthAuthorizationURL(started.oauthURL);
			if (!oauthURL) throw new Error('The OAuth provider returned an unsafe authorization URL.');
			popup.location.href = oauthURL;

			await poll(
				async () => {
					if (generation !== testGeneration) return true;
					const result = await onGetTest(started.testState);
					if (result.status === 'pending') {
						if (!popup.closed) return false;
						credentialTest = failStaticOAuthCredentialTest(credentialTest, 'popup_closed');
						error = 'The authorization window was closed before the test finished.';
						return true;
					}
					if (result.status === 'succeeded') {
						if (!result.proof) {
							credentialTest = failStaticOAuthCredentialTest(credentialTest, 'invalid_test_result');
							error = credentialTestFailureMessage('invalid_test_result');
							return true;
						}
						credentialTest = succeedStaticOAuthCredentialTest(
							credentialTest,
							result.proof,
							result.expiresAt
						);
						if (credentialTest.status !== 'succeeded') {
							error = credentialTestFailureMessage('invalid_test_result');
							return true;
						}
						cancelTestExpiry?.();
						cancelTestExpiry = scheduleStaticOAuthCredentialTestExpiry(
							credentialTest.expiresAt,
							() => {
								if (generation !== testGeneration) return;
								credentialTest = invalidateStaticOAuthCredentialTest(credentialTest);
								error = credentialTestFailureMessage('expired');
								cancelTestExpiry = undefined;
							}
						);
					} else {
						credentialTest = failStaticOAuthCredentialTest(
							credentialTest,
							result.failureCategory ?? 'invalid_test_result'
						);
						error = credentialTestFailureMessage(result.failureCategory);
					}
					return true;
				},
				{ interval: 1000, maxTimeout: 5 * 60 * 1000 }
			);
		} catch (err) {
			if (generation !== testGeneration) return;
			credentialTest = failStaticOAuthCredentialTest(credentialTest, 'test_failed');
			error = err instanceof Error ? err.message : credentialTestFailureMessage();
		} finally {
			if (generation === testGeneration) {
				testing = false;
				closeTestPopup();
			}
		}
	}

	onDestroy(resetCredentialTest);

	export function open() {
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}

	async function handleSave() {
		showRequired = false;
		error = undefined;

		if (!form.clientID.trim()) {
			showRequired = true;
			return;
		}
		if (!form.clientSecret.trim()) {
			showRequired = true;
			return;
		}
		if (!canSaveStaticOAuthCredentials(credentialTest, form.clientID, form.clientSecret)) {
			error = 'Test these OAuth credentials successfully before saving.';
			return;
		}

		const proof = credentialTest.proof;
		credentialTest = invalidateStaticOAuthCredentialTest(credentialTest);
		cancelTestExpiry?.();
		cancelTestExpiry = undefined;
		loading = true;
		try {
			await onSave({
				clientID: form.clientID,
				clientSecret: form.clientSecret,
				proof
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
		const expectedGeneration = oauthStatus?.generation;
		if (!expectedGeneration) {
			showDeleteConfirm = false;
			dialog?.open();
			error = 'Reload the OAuth application status before clearing credentials.';
			return;
		}
		loading = true;
		try {
			await onDelete(expectedGeneration);
			showDeleteConfirm = false;
			dialog?.close();
		} catch (err) {
			showDeleteConfirm = false;
			dialog?.open();
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
				<CircleAlert class="size-6 text-error" />
				<p class="text-sm font-light">{error}</p>
			</div>
		{/if}

		<McpDeprecatedNotice {deprecated} variant="notification" />

		{#if oauthStatus?.configured}
			<p class="text-muted-content text-sm font-light">
				Test replacement credentials before saving. The active app and user grants remain usable
				unless the tested replacement is saved successfully.
			</p>
		{:else}
			<p class="text-muted-content text-sm font-light">
				This remote MCP server requires OAuth configuration. Provide the client credentials from
				your OAuth provider, then test them before saving.
			</p>
		{/if}

		<div class="flex flex-col gap-4">
			<div class="flex flex-col gap-1">
				<label for="clientID" class:text-error={showRequired && !form.clientID}> Client ID </label>
				<input
					type="text"
					id="clientID"
					bind:value={form.clientID}
					class="text-input-filled"
					class:error={showRequired && !form.clientID}
					placeholder="your-client-id"
					disabled={loading}
					oninput={handleCredentialInput}
				/>
			</div>

			<div class="flex flex-col gap-1">
				<label for="clientSecret" class:text-error={showRequired && !form.clientSecret}>
					Client Secret
				</label>
				<SensitiveInput
					name="clientSecret"
					bind:value={form.clientSecret}
					error={showRequired && !form.clientSecret}
					placeholder="your-client-secret"
					disabled={loading}
					oninput={handleCredentialInput}
				/>
			</div>

			{#if oauthStatus?.callbackURL}
				<CopyField
					id="static-oauth-callback-url"
					label="OAuth callback URL"
					value={oauthStatus.callbackURL}
					variant="code"
				/>
			{/if}

			{#if credentialTest.status === 'succeeded'}
				<div class="bg-success/10 text-success flex items-center gap-2 rounded-md p-3 text-sm">
					<CircleCheckBig class="size-5 shrink-0" />
					Credentials tested successfully. You can save them.
				</div>
			{/if}
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
				Clear Credentials
			</button>
		{:else}
			<div></div>
		{/if}

		<div class="flex flex-wrap gap-2">
			{#if showSkip && !oauthStatus?.configured}
				<button type="button" class="btn btn-secondary" onclick={handleSkip} disabled={loading}>
					Skip
				</button>
			{/if}
			<button type="button" class="btn btn-secondary" onclick={handleCancel} disabled={loading}>
				Cancel
			</button>
			<button
				type="button"
				class="btn btn-secondary"
				onclick={handleTest}
				disabled={loading || testing}
			>
				{#if testing}
					<Loading class="size-4" />
				{:else}
					<ExternalLink class="size-4" />
					Test Credentials
				{/if}
			</button>
			<button
				type="button"
				class="btn btn-primary"
				onclick={handleSave}
				disabled={loading ||
					testing ||
					!canSaveStaticOAuthCredentials(credentialTest, form.clientID, form.clientSecret)}
			>
				{#if loading}
					<Loading class="size-4" />
				{:else}
					{oauthStatus?.configured ? 'Replace Credentials' : 'Save'}
				{/if}
			</button>
		</div>
	</div>
</ResponsiveDialog>

<Confirm
	show={showDeleteConfirm}
	msg="Are you sure you want to clear the OAuth credentials? All deployments remain, but all Users must reconnect after new credentials are configured."
	onsuccess={handleDelete}
	oncancel={() => {
		showDeleteConfirm = false;
		dialog?.open();
	}}
	{loading}
/>
