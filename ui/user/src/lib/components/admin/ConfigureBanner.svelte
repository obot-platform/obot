<script lang="ts">
	import { version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';

	interface Props {
		modelProviderConfigured?: boolean;
		authProviderConfigured?: boolean;
	}

	let { modelProviderConfigured, authProviderConfigured }: Props = $props();

	// Use the store for reactive data
	const storeData = $derived($adminConfigStore);

	// Use props if provided, otherwise use store values
	const isModelProviderConfigured = $derived(
		typeof modelProviderConfigured === 'boolean'
			? modelProviderConfigured
			: storeData.modelProviderConfigured
	);

	const isAuthProviderConfigured = $derived(
		version.current.authEnabled
			? typeof authProviderConfigured === 'boolean'
				? authProviderConfigured
				: storeData.authProviderConfigured
			: true
	);

	const loading = $derived(storeData.loading);
</script>

{#if !loading && (!isModelProviderConfigured || !isAuthProviderConfigured)}
	<div
		class="dark:bg-surface2 flex min-h-44 justify-center overflow-hidden rounded-xl bg-white py-4"
	>
		<div
			class="relative flex min-h-36 w-[calc(100%-4rem)] max-w-screen-md flex-row items-center justify-between gap-4 rounded-sm"
		>
			<div class="absolute opacity-5 md:top-[-1.75rem] md:left-[-3.0rem] md:opacity-45">
				<img
					src="/user/images/obot-icon-surprised-yellow.svg"
					alt="obot alert"
					class="md:h-[17.5rem] md:w-[17.5rem]"
				/>
			</div>
			<div class="relative z-10 flex flex-col gap-2 md:ml-64">
				<h4 class="text-lg font-semibold">Wait! You've still got some setup to do!</h4>
				{#if !isModelProviderConfigured}
					<p class="text-sm font-light">
						<b class="font-semibold">Model Provider:</b> To use the Obot Chat feature, configure a Model
						Provider.
					</p>
				{/if}
				{#if !isAuthProviderConfigured}
					<p class="text-sm font-light">
						<b class="font-semibold">Auth Provider:</b> To support multiple users, configure an Auth
						Provider.
					</p>
				{/if}
				<div class="flex flex-row flex-wrap gap-2">
					{#if !isModelProviderConfigured}
						<a
							href="/admin/model-providers"
							class="button grow bg-yellow-500 text-center text-sm text-black hover:bg-yellow-500/70"
						>
							Configure Model Provider
						</a>
					{/if}
					{#if !isAuthProviderConfigured}
						<a
							href="/admin/auth-providers"
							class="button grow bg-yellow-500 text-center text-sm text-black hover:bg-yellow-500/70"
						>
							Configure Auth Provider
						</a>
					{/if}
				</div>
			</div>
		</div>
	</div>
{/if}
