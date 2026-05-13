<script lang="ts">
	import { page } from '$app/state';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { twMerge } from 'tailwind-merge';

	let error = $derived(page.url.searchParams.get('error') ?? '');
	let code = $derived(page.url.searchParams.get('code') ?? '');
	let state = $derived(page.url.searchParams.get('state') ?? '');
	let errorDescription = $derived(page.url.searchParams.get('error_description') ?? '');
	let isError = $derived(error !== '');
	let hasPayload = $derived(isError || code !== '' || state !== '');
</script>

<main
	id="main-content"
	class="text-on-background bg-surface1 flex min-h-dvh items-center justify-center px-4 py-16"
>
	<div
		class={twMerge(
			'min-w-md paper flex flex-col items-center justify-center gap-4 p-0 pt-8',
			isError ? 'md:max-w-4xl' : 'md:max-w-md'
		)}
	>
		<BetaLogo />
		<h1 class=" text-lg font-semibold">OAuth Debugger Authorization</h1>

		<div class="flex flex-col items-center justify-center gap-4 p-4 pt-0">
			{#if !hasPayload}
				<p class="text-on-surface1 text-sm">
					This page is opened after the MCP OAuth debugger redirect. No code or error was provided.
				</p>
			{:else if isError}
				<dl class="border-red-500/30 bg-red-500/10 space-y-4 rounded-md border p-4">
					<div class="flex items-start justify-between">
						<div>
							<dt class="text-red-500 text-sm font-medium">Error</dt>
							<dd class="text-on-background mt-1 font-mono text-sm break-all">{error}</dd>
						</div>
						<CopyButton
							showTextLeft
							classes={{ button: 'text-xs shrink-0 flex items-center gap-1' }}
							text={JSON.stringify({
								error,
								errorDescription
							})}
						/>
					</div>
					{#if errorDescription}
						<div>
							<dt class="text-red-500 text-sm font-medium">Description</dt>
							<dd class="text-on-background mt-1 text-sm wrap-break-word">{errorDescription}</dd>
						</div>
					{/if}
				</dl>
			{:else}
				<p class="text-on-surface1 text-sm font-light">
					Copy the authorization code below into the "<b>Request & Acquire Authorization Code</b>"
					step.
				</p>

				<div class="relative bg-surface2 rounded-md px-4 py-2">
					<div class="flex items-center justify-between mb-2">
						<p class="text-sm font-semibold">Authorization Code</p>
						<CopyButton
							showTextLeft
							classes={{ button: 'text-xs shrink-0 flex items-center gap-1' }}
							text={code}
						/>
					</div>
					<pre
						class="bg-transparent my-0 max-w-full min-w-0 flex-1 overflow-x-auto font-mono text-sm break-all text-on-background">{code}</pre>
				</div>
			{/if}
		</div>

		<div class="border-t border-surface2 py-2 px-4 w-full">
			<p class="text-[9px] text-on-surface1 truncate">
				<span class="font-medium">State:</span>
				{state}
			</p>
		</div>
	</div>
</main>

<svelte:head>
	<title>Obot | OAuth Debugger Authorization</title>
</svelte:head>
