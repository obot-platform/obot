<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import type { AuthProvider, ModelProvider } from '$lib/services';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';

	interface Props {
		onConfirm: () => void | Promise<void>;
		onCancel: () => void | Promise<void>;
		loading?: boolean;
		title?: string;
		providers?: (AuthProvider | ModelProvider)[];
		confirmButtonText?: string;
	}

	let {
		loading,
		onConfirm,
		onCancel,
		providers,
		title = 'Confirm Deconfiguration',
		confirmButtonText = 'Deconfigure'
	}: Props = $props();

	let providerDeconfigureConfirmDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let confirmationInput = $state('');

	export function open() {
		confirmationInput = '';
		providerDeconfigureConfirmDialog?.open();
	}

	export function close() {
		providerDeconfigureConfirmDialog?.close();
	}

	let authProvider = $derived(providers?.find((p) => p.type === 'authprovider'));
	let listOfProviders = $derived(
		providers
			?.map((p) => p.name)
			.join(', ')
			.replace(/,([^,]*)$/, ' and$1')
	);
</script>

<ResponsiveDialog
	bind:this={providerDeconfigureConfirmDialog}
	{title}
	classes={{
		header: 'border-t-4 border-error px-4 pt-4 md:pb-0',
		content: 'p-0'
	}}
	class={authProvider ? 'md:max-w-4xl' : 'md:max-w-md'}
>
	{#if providers}
		<div class="flex h-full">
			<div class="px-4 py-4 md:py-0 flex flex-col gap-4 h-full">
				<p>
					{#if providers.length === 1}
						This action will deconfigure the provider: <b>{providers[0].name}</b>.
					{:else}
						This action will deconfigure the following providers: <b>{listOfProviders}</b>.
					{/if}
					This action cannot be undone. Are you sure you wish to continue?
				</p>
				{#if authProvider}
					<div class="p-4 bg-error/10 text-error rounded-md text-sm">
						<p class="mb-2">
							Deconfiguring <b>{authProvider.name || 'this provider'}</b> will result in the following:
						</p>
						<ul class="px-4 list-disc space-y-2">
							<li>
								Existing users will need to sign in via a different accessible provider -- each user
								will log in with a new account & lose access to their previous account.
							</li>
							<li>
								Powerusers will lose access to any of their created MCP entries and registries. They
								will be available for connection but no longer editable by their creator.
							</li>
							<li>
								The accounts tied to this provider will continue to exist and will require manual
								cleanup by an administrator.
							</li>
						</ul>
					</div>
				{/if}

				{#if authProvider}
					<div class="flex flex-col gap-1">
						<p>
							Type the provider ID <code class="text-xs p-1 bg-base-200">{authProvider.id}</code> to confirm.
						</p>

						<input type="text" class="input-text-filled w-full" bind:value={confirmationInput} />
					</div>
				{/if}
				<div class="md:hidden flex grow"></div>
				<div class="flex gap-4 w-full pt-4 md:py-4">
					<button class="btn btn-secondary flex-1" disabled={loading} onclick={onCancel}
						>Nevermind</button
					>
					<button
						class="btn btn-error btn-soft flex-1"
						disabled={loading || (authProvider && confirmationInput !== authProvider.id)}
						onclick={onConfirm}
					>
						{#if loading}
							<Loading class="size-4" />
						{:else}
							{confirmButtonText}
						{/if}
					</button>
				</div>
			</div>
		</div>
	{/if}
</ResponsiveDialog>
