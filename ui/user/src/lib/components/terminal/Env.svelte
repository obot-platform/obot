<script lang="ts">
	import { onMount } from 'svelte';
	import Env from '$lib/components/tool/Env.svelte';
	import { ChatService } from '$lib/services';
	import { currentAssistant } from '$lib/stores';
	import { masked } from '$lib/components/tool/Env.svelte';

	let dialog: HTMLDialogElement;
	let envs: { key: string; value: string; editing: string }[] = $state([]);
	let error = $state('');

	export async function show() {
		const newEnvs = await ChatService.getAssistantEnv($currentAssistant.id);
		envs = Object.entries(newEnvs).map(([key, value]) => ({ key, value, editing: masked }));
		envs.push({ key: '', value: '', editing: '' });
		error = '';
		dialog.showModal();
	}

	async function save() {
		const newEnv = envs.reduce(
			(acc, { key, value }) => {
				if (key) {
					acc[key] = value;
				}
				return acc;
			},
			{} as Record<string, string>
		);
		try {
			await ChatService.saveAssistantEnv($currentAssistant.id, newEnv);
		} catch (e) {
			error = e.toString();
			return;
		}
		dialog.close();
	}

	onMount(() => {
		show();
	});
</script>

<dialog
	bind:this={dialog}
	class="w-full max-w-3xl rounded-3xl bg-gray-50 text-black dark:bg-gray-950 dark:text-gray-50"
>
	<Env {envs} />
	{#if error}
		<p class="p-5 text-sm text-red-500">{error}</p>
	{/if}
	<div class="flex w-full items-center justify-end gap-2 p-5">
		<button class="p-3 px-6" onclick={() => dialog.close()}> Cancel </button>
		<button class="rounded-3xl bg-blue p-3 px-6 text-white" onclick={() => save()}> Save </button>
	</div>
</dialog>

<style lang="postcss">
	dialog::backdrop {
		@apply bg-black bg-opacity-60;
	}
</style>
