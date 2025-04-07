<script lang="ts">
	import { Plus, Minus } from 'lucide-svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';
	import { fade } from 'svelte/transition';

	interface Props {
		classes?: {
			header?: string;
		};
		params: { key: string; value: string }[];
		input?: { key: string; value: string }[];
		autofocus?: boolean;
	}

	let { params = $bindable([]), input = $bindable(), classes }: Props = $props();
</script>

<div
	class={twMerge(
		'bg-surface1 mt-5 flex flex-col gap-4 rounded-lg p-5 md:mt-0',
		input &&
			'border-surface2 dark:border-surface3 relative w-full rounded-lg border-2 bg-transparent'
	)}
>
	{#if input}
		<h4
			class={twMerge(
				'dark:bg-surface2 absolute top-0 left-3 w-fit -translate-y-3.5 bg-white px-2 text-base font-semibold',
				classes?.header
			)}
		>
			Arguments
		</h4>
	{:else}
		<div class="flex min-h-10 items-center">
			<h4 class="flex-1 text-lg font-semibold">Arguments</h4>
			{#if params.length === 0}
				<button
					transition:fade
					class="icon-button"
					onclick={() => params.push({ key: '', value: '' })}
					use:tooltip={{ text: 'Add Argument', disablePortal: true }}
				>
					<Plus class="size-5" />
				</button>
			{/if}
		</div>
	{/if}

	{#if params.length > 0}
		<table class="mb-2 w-full table-auto text-left" class:mt-4={!!input}>
			<thead class:hidden={input}>
				<tr>
					<th class="w-1/2 font-medium md:w-1/4">Name</th>
					{#if input}
						<th class="w-1/2 font-medium md:w-full">Value</th>
					{:else}
						<th class="w-1/2 font-medium md:w-full">Description</th>
					{/if}
				</tr>
			</thead>
			<tbody>
				{#each params as param, i}
					<tr>
						{#if input}
							{#if input[i] && input[i].key === param.key}
								<td class="w-full">
									<label for="param-{param.key}" class="flex-1 text-sm font-medium capitalize"
										>{param.key}</label
									>
									<input
										id="param-{param.key}"
										bind:value={input[i].value}
										class="text-input-filled mt-0.5"
										placeholder={param.value}
									/>
								</td>
							{/if}
						{:else}
							<td class="pr-4 align-bottom">
								<input bind:value={param.key} placeholder="Enter name" class="text-input w-full" />
							</td>
							<td class="flex items-center pr-4">
								<textarea
									use:autoHeight
									class="text-input min-h-[40px] w-full resize-none"
									rows="1"
									bind:value={param.value}
									placeholder="Add a good description"
								></textarea>
							</td>
						{/if}
						{#if !input}
							<td>
								<button
									class="icon-button translate-y-1"
									onclick={() => params.splice(i, 1)}
									use:tooltip={{ text: 'Remove Argument', disablePortal: true }}
								>
									<Minus class="h-5 w-5" />
								</button>
							</td>
						{/if}
					</tr>
				{/each}
			</tbody>
		</table>
		{#if !input}
			<div class="flex justify-end">
				<button class="button-small" onclick={() => params.push({ key: '', value: '' })}>
					<Plus class="size-4" /> Argument
				</button>
			</div>
		{/if}
	{:else if input}
		<div class="flex items-center justify-center">
			<p class="text-sm text-gray-500">No arguments.</p>
		</div>
	{/if}
</div>
