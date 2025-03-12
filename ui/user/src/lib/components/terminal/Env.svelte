<script lang="ts">
	import Env from '$lib/components/tool/Env.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { masked } from '$lib/components/tool/Env.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let dialog: HTMLDialogElement;
	let envs: { key: string; value: string; editing: string }[] = $state([]);
	let error = $state('');

	export async function show() {
		const newEnvs = await ChatService.getAssistantEnv(project.assistantID, project.id);
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
			await ChatService.saveAssistantEnv(project.assistantID, project.id, newEnv);
		} catch (e) {
			if (e instanceof Error) {
				error = e.message;
			} else {
				error = String(e);
			}
			return;
		}
		dialog.close();
	}
</script>

<dialog
	bind:this={dialog}
	class="w-full max-w-3xl"
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
