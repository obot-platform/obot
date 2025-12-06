<script lang="ts">
	import MarkdownTextEditor from '$lib/components/admin/MarkdownTextEditor.svelte';

	interface Props {
		task: {
			id: string;
			name: string;
			description: string;
			content: string;
		};
	}

	let { task = $bindable() }: Props = $props();
</script>

<div class="flex flex-col gap-1">
	<input
		class="ghost-input relative z-20 text-lg font-semibold"
		bind:value={task.name}
		placeholder="Task title"
	/>
	<div class="my-2">
		<MarkdownTextEditor placeholder="Add description..." bind:value={task.description} />
	</div>
	<div class="my-2">
		<MarkdownTextEditor
			placeholder="Add content..."
			bind:value={task.content}
			onUpdate={(content) => {
				console.log(content);
			}}
		/>
	</div>
</div>
