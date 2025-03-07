<script lang="ts">
	import type { OnDemand } from '$lib/services';
	import { Trash2 } from 'lucide-svelte';

	interface Props {
		onDemand?: OnDemand;
	}

	let { onDemand = $bindable() }: Props = $props();
	let order = $state<string[]>([]);

	$effect(() => {
		for (const key in onDemand?.params ?? {}) {
			if (!order.includes(key)) {
				order.push(key);
			}
		}
	});
</script>

<div class="flex flex-col gap-4">
	<h4 class="text-xl font-semibold">Arguments</h4>
	<p class="text-sm text-gray">
		Reference these values in your steps using <span class="font-mono text-black dark:text-white"
			>$VAR</span
		> syntax
	</p>

	<table class="w-full text-left">
		<thead class="font-semibold">
			<tr>
				<th>Name</th>
				<th>Description</th>
			</tr>
		</thead>
		<tbody>
			{#if onDemand?.params}
				{#each order as key, i (i)}
					<tr class="group">
						<td>
							<input
								value={key}
								placeholder="Enter Name"
								oninput={(e) => {
									if (e.target instanceof HTMLInputElement && onDemand?.params) {
										const oldKey = order[i];
										const newKey = e.target.value;
										order[i] = newKey;
										onDemand.params[newKey] = onDemand.params[oldKey] ?? '';
										delete onDemand.params[oldKey];
									}
								}}
							/>
						</td>
						<td>
							<input bind:value={onDemand.params[order[i]]} placeholder="Add a good description" />
						</td>
						<td>
							<button
								class="icon-button-colors rounded-lg p-2"
								onclick={() => {
									const key = order[i];
									order = order.filter((k) => k !== key);
									if (onDemand?.params) {
										delete onDemand.params[key];
									}
								}}
							>
								<Trash2 class="size-4" />
							</button>
						</td>
					</tr>
				{/each}
			{/if}
		</tbody>
	</table>
	<div class="self-end">
		<button
			class="button"
			onclick={() => {
				order.push('');
			}}
		>
			Add Argument
		</button>
	</div>
</div>
