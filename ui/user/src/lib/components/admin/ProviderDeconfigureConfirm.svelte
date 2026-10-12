<script lang="ts">
	import { m } from '$lib/i18n';
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
		title = m.models_providers_confirm_deconfiguration(),
		confirmButtonText = m.models_providers_deconfigure()
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
	let scimManaged = $derived(
		!!authProvider && 'scimState' in authProvider && !!authProvider.scimState
	);
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
		header: 'px-4 pt-4 md:pb-0',
		content: 'p-0'
	}}
	class={authProvider ? 'md:max-w-4xl' : 'md:max-w-md'}
>
	{#if providers}
		<div class="flex h-full">
			<div class="px-4 py-4 md:py-0 flex flex-col gap-4 h-full">
				<p>
					{#if providers.length === 1}
						{m.models_providers_deconfigure_single_prefix()}
						<b>{providers[0].name}</b>{m.models_providers_sentence_end()}
					{:else}
						{m.models_providers_deconfigure_multi_prefix()}
						<b>{listOfProviders}</b>{m.models_providers_sentence_end()}
					{/if}
					{m.models_providers_cannot_be_undone()}
				</p>
				{#if authProvider}
					<div class="p-4 bg-error/10 text-error rounded-md text-sm">
						<p class="mb-2">
							{m.models_providers_deconfiguring_prefix()}
							<b>{authProvider.name || m.models_providers_this_provider()}</b>
							{m.models_providers_deconfiguring_suffix()}
						</p>
						<ul class="px-4 list-disc space-y-2">
							<li>
								{m.models_providers_deconfigure_effect_users()}
							</li>
							<li>
								{m.models_providers_deconfigure_effect_powerusers()}
							</li>
							<li>
								{m.models_providers_deconfigure_effect_accounts()}
							</li>
							{#if scimManaged}
								<li>
									{m.identity_access_auth_providers_scim_deconfigure_effect({
										name: authProvider.name
									})}
								</li>
							{/if}
						</ul>
					</div>
				{/if}

				{#if authProvider}
					<div class="flex flex-col gap-1">
						<p>
							{m.models_providers_type_provider_id_prefix()}
							<code class="text-xs p-1 bg-base-200">{authProvider.id}</code>
							{m.models_providers_type_provider_id_suffix()}
						</p>

						<input type="text" class="input-text-filled w-full" bind:value={confirmationInput} />
					</div>
				{/if}
				<div class="md:hidden flex grow"></div>
				<div class="flex gap-4 w-full pt-4 md:py-4">
					<button class="btn btn-secondary flex-1" disabled={loading} onclick={onCancel}
						>{m.models_providers_nevermind()}</button
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
