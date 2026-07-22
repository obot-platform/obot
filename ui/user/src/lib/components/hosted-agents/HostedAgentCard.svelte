<script lang="ts">
	import type { HostedAgent } from '$lib/services/admin/types';
	import { goto } from '$lib/url';
	import { Bot } from '@lucide/svelte';

	interface Props {
		agent: HostedAgent;
	}

	let { agent }: Props = $props();
</script>

<div
	class="dark:bg-base-400 dark:border-base-400 bg-base-100 flex flex-col gap-3 rounded-lg border border-transparent p-4"
>
	<div class="flex items-center gap-3">
		{#if agent.icon}
			<img
				src={agent.icon}
				alt=""
				class="size-10 rounded-md object-contain {agent.iconDark ? 'dark:hidden' : ''}"
			/>
		{/if}
		{#if agent.iconDark}
			<img
				src={agent.iconDark}
				alt=""
				class="hidden size-10 rounded-md object-contain dark:block"
			/>
		{/if}
		{#if !agent.icon && !agent.iconDark}
			<div class="bg-base-300 dark:bg-base-200 flex size-10 items-center justify-center rounded-md">
				<Bot class="text-muted-content size-5" />
			</div>
		{/if}
		<div class="flex min-w-0 flex-col">
			<h3 class="truncate font-semibold">{agent.name}</h3>
		</div>
	</div>

	{#if agent.description}
		<p class="text-muted-content line-clamp-2 text-sm font-light">{agent.description}</p>
	{/if}

	<div class="mt-auto flex items-center justify-between gap-2 pt-2">
		<button
			class="btn btn-primary w-full text-sm"
			onclick={() => goto(`/hosted-agents/${agent.id}`)}
		>
			Manage Instances
		</button>
	</div>
</div>
