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
				<div class="flex w-full flex-col gap-1">
					<label for="param-{key}" class="flex-1 text-sm font-light capitalize">{key}</label>
					<input
						id="param-{key}"
						bind:value={params[key]}
						class="dark:bg-surface3 bg-surface2 flex grow rounded-md p-2 shadow-inner outline-hidden"
						placeholder={task.onDemand.params[key]}
					/>
				</div>
			{/each}
		</div>
	{:else if task?.email}
		<h4 class="text-base font-semibold">Sample Email Details</h4>
		<div class="mt-4 flex flex-col gap-4">
			<div class="flex flex-col gap-1">
				<label for="from" class="w-[70px] text-sm font-light">From</label>
				<input
					id="from"
					bind:value={emailInput.from}
					class="dark:bg-surface3 bg-surface2 flex grow rounded-md p-2 shadow-inner outline-hidden"
					placeholder=""
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label for="from" class="w-[70px] text-sm font-light">To</label>
				<input
					id="from"
					bind:value={emailInput.to}
					class="dark:bg-surface3 bg-surface2 flex grow rounded-md p-2 shadow-inner outline-hidden"
					placeholder=""
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label for="from" class="w-[70px] text-sm font-light">Subject</label>
				<input
					id="from"
					bind:value={emailInput.subject}
					class="dark:bg-surface3 bg-surface2 flex grow rounded-md p-2 shadow-inner outline-hidden"
					placeholder=""
				/>
			</div>
			<div class="flex">
				<textarea
					id="body"
					bind:value={emailInput.body}
					use:autoHeight
					rows="1"
					class="dark:bg-surface3 bg-surface2 mt-2 w-full resize-none rounded-md p-5 shadow-inner outline-hidden"
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
			class="dark:bg-surface3 bg-surface2 mt-2 w-full resize-none rounded-md p-5 shadow-inner outline-hidden"
			placeholder="Enter payload..."
		></textarea>
	{/if}
</div>
