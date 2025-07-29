<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { type PageProps } from './$types';
	import { browser } from '$app/environment';
	import Logo from '$lib/components/navbar/Logo.svelte';

	let { data }: PageProps = $props();
	let { authProviders, loggedIn } = data;
	let overrideRedirect = $state<string | null>(null);
	let signUp = $state(true);

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
		<div class="flex items-center justify-center">
			<div class="animate-bounce">
				<Logo />
			</div>
			<p class="text-base font-semibold">Logging in...</p>
		</div>
	</div>
{/if}

{#snippet unauthorizedContent()}
	<div class="relative flex h-screen w-full flex-col text-black dark:text-white">
		<!-- Header with logo and navigation -->
		<div class="bg-surface1 sticky top-0 z-30 flex h-16 w-full items-center dark:bg-black">
			<div class="relative flex items-end p-5">
				{#if darkMode.isDark}
					<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
				{:else}
					<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
				{/if}
				<div class="ml-1.5 -translate-y-1">
					<span
						class="rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
					>
						BETA
					</span>
				</div>
			</div>
			<div class="grow"></div>
			<div class="flex items-center gap-4 px-5"></div>
		</div>

		<main
			class="dark:from-surface2 to-surface1 mx-auto flex h-full w-full flex-col items-center justify-center gap-18 bg-radial-[at_50%_50%] from-gray-50 pb-6 md:gap-24 md:pb-12 dark:to-black"
		>
			<div
				class="absolute top-1/2 left-1/2 flex w-md -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-4"
			>
				<img src="/user/images/obot-icon-blue.svg" class="h-16" alt="Obot logo" />
				<h1 class="text-2xl font-semibold">Welcome to Obot</h1>
				<p class="text-md mb-1 text-center font-light text-gray-400 dark:text-gray-600">
					Log in or create your account to continue
				</p>

				<div
					class="dark:border-surface3 dark:bg-gray-930 flex w-sm flex-col gap-4 rounded-xl border border-transparent bg-white p-4 shadow-sm"
				>
					{#each authProviders as provider (provider.id)}
						<a
							rel="external"
							href="/oauth2/start?rd={encodeURIComponent(
								overrideRedirect !== null ? overrideRedirect : rd
							)}&obot-auth-provider={provider.namespace}/{provider.id}"
							class="group bg-surface2 hover:bg-surface3 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold transition-colors duration-200"
							onclick={(e) => {
								console.log(`post-auth redirect ${e.target}`);
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
						</a>
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
