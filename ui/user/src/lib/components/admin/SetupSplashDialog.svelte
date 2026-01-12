<script lang="ts">
	import { page } from '$app/state';
	import { AdminService, Group } from '$lib/services';
	import { profile, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { goto } from '$lib/url';
	import Logo from '../Logo.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { CircleCheckBig, LoaderCircle } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let loading = $state(false);
	const storeData = $derived($adminConfigStore);
	const isAuthProviderConfigured = $derived(
		version.current.authEnabled ? storeData.authProviderConfigured : true
	);
	const isOnAuthProvidersPage = $derived(page.url.pathname === '/admin/auth-providers');
	const isBootstrapUser = $derived(profile.current.isBootstrapUser?.() ?? false);

	$effect(() => {
		if (profile.current.loaded && !profile.current.unauthorized && storeData.lastFetched) {
			const created = profile.current.created ? new Date(profile.current.created) : null;
			let firstTimeViewed = localStorage.getItem('seenSplashDialog')
				? new Date(localStorage.getItem('seenSplashDialog')!)
				: null;

			// the user is newer than the seenSplashDialog set, likely case of fresh install & revisiting with browser
			if (created && firstTimeViewed && created > firstTimeViewed) {
				localStorage.removeItem('seenSplashDialog');
				firstTimeViewed = null;
			}

			const isOwner = profile.current.groups.includes(Group.OWNER);
			if (
				!firstTimeViewed &&
				(isBootstrapUser || isOwner) &&
				(!isAuthProviderConfigured || !storeData.modelProviderConfigured || !storeData.eulaAccepted)
			) {
				dialog?.open();
			}
		}
	});

	async function handleAcceptEula() {
		if (storeData.eulaAccepted) return;
		loading = true;
		const response = await AdminService.acceptEula();
		adminConfigStore.updateEula(response.accepted);

		localStorage.setItem('seenSplashDialog', new Date().toISOString());
		loading = false;
	}
</script>

<ResponsiveDialog bind:this={dialog} hideClose disableClickOutside class="text-md w-sm">
	<div class="flex w-full items-center justify-center">
		<Logo class="size-18" />
	</div>
	<h2 class="mb-8 text-center text-2xl font-semibold">Welcome to Obot!</h2>

	<div class="w-fit self-center">
		{#if !isAuthProviderConfigured || !storeData.modelProviderConfigured}
			{#if isBootstrapUser}
				<p>Before using Obot, you'll need to:</p>
			{:else}
				<p class="text-center">
					You're halfway there! You just need to configure your model provider.
				</p>
			{/if}

			<ul class="checklist">
				{#if version.current.authEnabled}
					{@render renderChecklistItem(
						'Setup an Authentication Provider',
						isAuthProviderConfigured
					)}
				{/if}
				{@render renderChecklistItem('Setup a Model Provider', storeData.modelProviderConfigured)}
			</ul>
		{/if}

		<p class="pt-4">
			By continuing, you agree to Obot's <a
				href="https://obot.ai/eul"
				rel="external"
				target="_blank"
				class="text-link">EULA</a
			>
		</p>
	</div>

	{#if isBootstrapUser}
		<button
			class="button-primary mt-8 flex justify-center text-center"
			disabled={loading}
			onclick={async () => {
				handleAcceptEula();
				localStorage.setItem('seenSplashDialog', new Date().toISOString());

				if (isOnAuthProvidersPage) {
					dialog?.close();
					return;
				}

				if (!isAuthProviderConfigured) {
					goto('/admin/auth-providers');
				} else {
					goto('/admin/model-providers');
				}
				dialog?.close();
			}}
		>
			{#if loading}
				<LoaderCircle class="size-4 animate-spin" />
			{:else}
				Get Started
			{/if}
		</button>
	{:else}
		<button
			class="button-primary mt-8 flex justify-center text-center"
			onclick={() => {
				handleAcceptEula();
				localStorage.setItem('seenSplashDialog', new Date().toISOString());
				if (storeData.modelProviderConfigured) {
					dialog?.close();
				} else {
					goto('/admin/model-providers');
				}
			}}
		>
			Continue
		</button>
	{/if}
</ResponsiveDialog>

{#snippet renderChecklistItem(label: string, isChecked: boolean)}
	<li>
		<span
			class={twMerge('flex items-center gap-1', isChecked ? 'text-on-surface1 line-through' : '')}
		>
			{label}
			{#if isChecked}
				<CircleCheckBig class="size-5 text-green-500" />
			{/if}
		</span>
	</li>
{/snippet}

<style lang="postcss">
	.checklist {
		padding-left: 1rem;
		margin-top: 0.5rem;
		list-style-type: disc;
		li {
			margin-bottom: 0.5rem;
			gap: 0.5rem;
		}
	}
</style>
