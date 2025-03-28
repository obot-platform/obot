<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea';
	import type { OnDemand } from '$lib/services';
	import { Plus, Trash2 } from 'lucide-svelte';

	interface Props {
		onDemand?: OnDemand;
		readOnly?: boolean;
	}

	let { onDemand = $bindable(), readOnly }: Props = $props();
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
	<h4 class="text-base font-medium">Arguments</h4>
	<p class="text-gray text-sm">
		Reference these values in your steps using <span class="font-mono text-black dark:text-white"
			>$VAR</span
		> syntax
	</p>

	{#if onDemand?.params}
		<table class="w-full text-left">
			<thead class="text-sm">
				<tr>
					<th class="font-light">Name</th>
					<th class="font-light">Description</th>
				</tr>
			</thead>
			<tbody>
				{#each order as key, i (i)}
					<tr class="group">
						<td class="flex w-full justify-self-start pr-8">
							<input
								value={key}
								placeholder="Enter Name"
								class="ghost-input border-surface2 w-full"
								oninput={(e) => {
									if (e.target instanceof HTMLInputElement && onDemand?.params) {
										const oldKey = order[i];
										const newKey = e.target.value;
										order[i] = newKey;
										onDemand.params[newKey] = onDemand.params[oldKey] ?? '';
										delete onDemand.params[oldKey];
									}
								}}
								disabled={readOnly}
							/>
						</td>
						<td>
							<textarea
								use:autoHeight
								bind:value={onDemand.params[order[i]]}
								class="ghost-input border-surface2 w-full resize-none py-2.5"
								disabled={readOnly}
								placeholder="Add a good description"
								rows="1"
							></textarea>
						</td>
						{#if !readOnly}
							<td class="flex justify-end">
								<button
									class="icon-button"
									onclick={() => {
										const key = order[i];
										order = order.filter((k) => k !== key);
										if (onDemand?.params) {
											delete onDemand.params[key];
										}
									}}
								>
									<Trash2 class="size-5" />
								</button>
							</td>
						{/if}
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
	{#if !readOnly}
		<div class="self-end">
			<button
				class="button-small"
				onclick={() => {
					if (!onDemand?.params) {
						onDemand = {
							params: { '': '' }
						};
					}

					order.push('');
				}}
			>
				<Plus class="size-4" /> Argument
			</button>
		</div>
	{/if}
</div>
