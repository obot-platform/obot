<script lang="ts">
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { profile, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import Logo from '../navbar/Logo.svelte';
	import { Group } from '$lib/services';
	import { CircleCheckBig } from 'lucide-svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { twMerge } from 'tailwind-merge';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	const storeData = $derived($adminConfigStore);
	const isAuthProviderConfigured = $derived(
		version.current.authEnabled ? storeData.authProviderConfigured : true
	);
	const isOnAuthProvidersPage = $derived(page.url.pathname === '/admin/auth-providers');
	const isBootstrapUser = $derived(profile.current.isBootstrapUser?.() ?? false);

	$effect(() => {
		if (profile.current.loaded && !profile.current.unauthorized && storeData.lastFetched) {
			const firstTimeViewed = localStorage.getItem('seenSplashDialog');
			const isOwner = profile.current.groups.includes(Group.OWNER);
			if (
				!firstTimeViewed &&
				(isBootstrapUser || isOwner) &&
				(!isAuthProviderConfigured || !storeData.modelProviderConfigured)
			) {
				dialog?.open();
			}
		}
	});
</script>

<ResponsiveDialog bind:this={dialog} class="text-md w-sm">
	<div class="flex w-full items-center justify-center">
		<Logo class="size-18" />
	</div>
	<h2 class="mb-8 text-center text-2xl font-semibold">Welcome to Obot!</h2>

	<div class="w-fit self-center">
		{#if isBootstrapUser}
			<p>Before using Obot, you'll need to:</p>
		{:else}
			<p class="text-center">
				You're halfway there! You just need to configure your model provider.
			</p>
		{/if}

		<ul class="checklist">
			{#if version.current.authEnabled}
				{@render renderChecklistItem('Setup an Authentication Provider', isAuthProviderConfigured)}
			{/if}
			{@render renderChecklistItem('Setup a Model Provider', storeData.modelProviderConfigured)}
		</ul>
	</div>

	{#if isBootstrapUser}
		<button
			class="button-primary mt-8 text-center"
			onclick={() => {
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
			Get Started
		</button>
	{:else}
		<button
			class="button-primary mt-8 text-center"
			onclick={() => {
				localStorage.setItem('seenSplashDialog', new Date().toISOString());
				goto('/admin/model-providers');
			}}
		>
			Continue
		</button>
	{/if}
</ResponsiveDialog>

{#snippet renderChecklistItem(label: string, isChecked: boolean)}
	<li>
		<span class={twMerge('flex items-center gap-1', isChecked ? 'text-gray-500 line-through' : '')}>
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
