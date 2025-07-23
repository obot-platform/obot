<script lang="ts">
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import { Server, X } from 'lucide-svelte';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';

	const projectMcps = getProjectMCPs();
	let currentIndex = $state(0);
	let mcpServersThatRequireOauth = $derived(
		projectMcps.items.filter((mcp) => !mcp.authenticated && mcp.oauthURL)
	);
	let dialogs = $state<HTMLDialogElement[]>([]);

	$effect(() => {
		console.log('hmm');
		if (mcpServersThatRequireOauth.length > 0) {
			console.log('a');
			currentIndex = 0;
			dialogs[currentIndex]?.showModal();
		}
	});

	function next() {
		dialogs[currentIndex]?.close();
		if (currentIndex !== mcpServersThatRequireOauth.length - 1) {
			currentIndex = currentIndex + 1;
			dialogs[currentIndex]?.showModal();
		}
	}
</script>

{#each mcpServersThatRequireOauth as mcpServer, i (mcpServer.id)}
	<dialog
		bind:this={dialogs[i]}
		class="flex w-full flex-col gap-4 p-4 md:w-sm"
		use:dialogAnimation={i !== 0 ? { type: 'slide' } : { type: 'fade' }}
	>
		<div class="absolute top-2 right-2">
			<button class="icon-button" onclick={next}>
				<X class="size-4" />
			</button>
		</div>
		<div class="flex items-center gap-2">
			<div class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600">
				{#if mcpServer.manifest.icon}
					<img src={mcpServer.manifest.icon} alt={mcpServer.manifest.name} class="size-6" />
				{:else}
					<Server class="size-6" />
				{/if}
			</div>
			<h3 class="text-lg leading-5.5 font-semibold">
				{mcpServer.manifest.name}
			</h3>
		</div>

		<p>
			In order to use {mcpServer.manifest.name}, authentication with the MCP server is required.
		</p>

		<p>Click the link below to authenticate.</p>

		<a
			href={mcpServer.oauthURL}
			target="_blank"
			class="button-primary text-center text-sm outline-none"
			onclick={next}
		>
			Authenticate
		</a>
	</dialog>
{/each}
