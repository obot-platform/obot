<script lang="ts">
	import { browser } from '$app/environment';
	import Logo from '$lib/components/Logo.svelte';
	import { type PageProps } from './$types';

	let { data }: PageProps = $props();
	let { authProviders, loggedIn } = $derived(data);
	let overrideRedirect = $state<string | null>(null);

	let rd = $derived.by(() => {
		if (browser) {
			const rd = new URL(window.location.href).searchParams.get('rd');
			if (rd) {
				return rd;
			}
		}
		if (overrideRedirect !== null) {
			return overrideRedirect;
		}
		return '/';
	});
</script>

<svelte:head>
	<title>Obot - Build AI agents with MCP</title>
</svelte:head>

{#if !loggedIn}
	{@render unauthorizedContent()}
{:else}
	<div class="flex h-svh w-svw flex-col items-center justify-center">
		<div class="flex items-center justify-center gap-2">
			<div class="animate-bounce">
				<Logo />
			</div>
			<p class="text-base font-semibold">Logging in...</p>
		</div>
	</div>
{/if}

{#snippet unauthorizedContent()}
	<div class="text-on-background relative flex h-dvh w-full flex-col">
		<main
			class="dark:from-surface2 to-surface1 mx-auto flex h-full w-full flex-col items-center justify-center gap-18 bg-radial-[at_50%_50%] from-gray-50 pb-6 md:gap-24 md:pb-12 dark:to-black"
		>
			<div
				class="absolute top-1/2 left-1/2 flex w-md -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-4"
			>
				<Logo class="h-16" />
				<h1 class="text-2xl font-semibold">Welcome to Obot</h1>
				<p class="text-md text-on-surface1 mb-1 text-center font-light">
					Log in or create your account to continue
				</p>

				<div
					class="dark:border-surface3 dark:bg-gray-930 bg-background flex w-sm flex-col gap-4 rounded-xl border border-transparent p-4 shadow-sm"
				>
					{#each authProviders as provider (provider.id)}
						<button
							class="group bg-surface2 hover:bg-surface3 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold transition-colors duration-200"
							onclick={() => {
								localStorage.setItem('preAuthRedirect', window.location.href);
								window.location.href = `/oauth2/start?rd=${encodeURIComponent(
									overrideRedirect !== null ? overrideRedirect : rd
								)}&obot-auth-provider=${provider.namespace}/${provider.id}`;
							}}
						>
							{#if provider.icon}
								<img
									class="h-6 w-6 rounded-full bg-transparent p-1 dark:bg-gray-600"
									src={provider.icon}
									alt={provider.name}
								/>
								<span class="text-center text-sm font-light">Continue with {provider.name}</span>
							{/if}
						</button>
					{/each}
					{#if authProviders.length === 0}
						<p>
							No auth providers configured. Please configure at least one auth provider in the admin
							panel.
						</p>
					{/if}
				</div>
			</div>
		</main>
	</div>
{/snippet}

<style lang="postcss">
	:global {
		.well {
			padding-left: 1rem;
			padding-right: 1rem;
			@media (min-width: 1024px) {
				padding-left: 4rem;
				padding-right: 4rem;
			}
			@media (min-width: 768px) {
				padding-left: 2rem;
				padding-right: 2rem;
			}
		}
	}
</style>
