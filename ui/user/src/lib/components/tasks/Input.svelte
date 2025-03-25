<script lang="ts">
	import { type Task } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea.js';

	interface Props {
		input?: string;
		displayRunID?: string;
		task?: Task;
	}

	let { input = $bindable(''), task }: Props = $props();

	let params: Record<string, string> = $state({});
	let payload: string = $state('');
	let emailInput = $state({
		type: 'email',
		from: '',
		to: '',
		subject: '',
		body: ''
	});

	$effect(() => {
		if (task?.onDemand?.params) {
			input = JSON.stringify(params);
		} else if (task?.email) {
			input = JSON.stringify(emailInput);
		} else if (task?.webhook) {
			input = JSON.stringify({
				type: 'webhook',
				payload
			});
		} else {
			input = '';
		}
	});
</script>

<div class="w-full">
	{#if task?.onDemand?.params}
		<h4 class="mb-3 text-base font-semibold">Arguments</h4>
		<div class="mt-4 flex flex-col items-baseline gap-4">
			{#each Object.keys(task.onDemand.params) as key}
				<div class="flex w-full flex-col">
					<label for="param-{key}" class="flex-1 text-sm font-light capitalize">{key}</label>
					<input
						id="param-{key}"
						bind:value={params[key]}
						class="ghost-input border-surface3 text-md w-full"
						placeholder={task.onDemand.params[key]}
					/>
				</div>
			{/each}
		</div>
	{:else if task?.email}
		<h4 class="text-base font-semibold">Sample Email</h4>
		<div class="mt-5 flex flex-col gap-5 rounded-xl bg-white p-5 dark:bg-black">
			<div class="flex items-baseline">
				<label for="from" class="w-[70px] text-sm font-semibold">From</label>
				<input
					id="from"
					bind:value={emailInput.from}
					class="rounded-md bg-gray-50 p-2 outline-hidden dark:bg-gray-950"
					placeholder=""
				/>
			</div>
			<div class="flex items-baseline">
				<label for="from" class="w-[70px] text-sm font-semibold">To</label>
				<input
					id="from"
					bind:value={emailInput.to}
					class="rounded-md bg-gray-50 p-2 outline-hidden dark:bg-gray-950"
					placeholder=""
				/>
			</div>
			<div class="flex items-baseline">
				<label for="from" class="w-[70px] text-sm font-semibold">Subject</label>
				<input
					id="from"
					bind:value={emailInput.subject}
					class="rounded-md bg-gray-50 p-2 outline-hidden dark:bg-gray-950"
					placeholder=""
				/>
			</div>
			<div class="flex">
				<textarea
					id="body"
					bind:value={emailInput.body}
					use:autoHeight
					rows="1"
					class="mt-2 w-full resize-none rounded-md bg-gray-50 p-5 outline-hidden dark:bg-gray-950"
					placeholder="Email content"
				></textarea>
			</div>
		</div>
	{:else if task?.webhook}
		<h4 class="text-base font-semibold">Sample Webhook Payload</h4>
		<textarea
			bind:value={payload}
			use:autoHeight
			rows="1"
			class="ghost-input border-surface3 w-full grow resize-none border-b"
			placeholder="Enter payload..."
		></textarea>
	{/if}
</div>
