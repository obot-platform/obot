<script lang="ts">
	import { type Task } from '$lib/services';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';
	import { version } from '$lib/stores';
	import Trigger from './Trigger.svelte';
	import type { Project, ProjectCredential } from '$lib/services';
	import { ChatService } from '$lib/services';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import type { AssistantTool } from '$lib/services';

	interface Props {
		task?: Task;
		readOnly?: boolean;
		project: Project;
	}

	let credentials = $state<ProjectCredential[]>([]);
	let toolSelection = $state<Record<string, AssistantTool>>({});
	const projectTools = getProjectTools();
	toolSelection = getSelectionMap();

	function getSelectionMap() {
		return projectTools.tools
			.filter((t) => !t.builtin)
			.reduce<Record<string, AssistantTool>>((acc, tool) => {
				acc[tool.id] = { ...tool };
				return acc;
			}, {});
	}

	$effect(() => {
		ChatService.listProjectCredentials(project.assistantID, project.id).then((creds) => {
			credentials = creds.items;
		});
	});

	let { task = $bindable(), readOnly, project }: Props = $props();

	let options = $derived.by(() => {
		const options: Record<string, string> = {
			onDemand: 'on demand',
			schedule: 'on interval',
			webhook: 'on webhook'
		};
		console.log(project);
		if (version.current.emailDomain) {
			options['email'] = 'on email';
		}
		if (project.capabilities?.onSlackMessage) {
			options['slack'] = 'on slack';
		}
		if (
			credentials.find((c) => c.toolID === 'discord-bundle')?.exists &&
			toolSelection['discord-bundle']?.enabled
		) {
			options['discord'] = 'on discord';
		}
		// assigned later so it's rendered last
		options['onDemand'] = 'on demand';
		return options;
	});

	function selectedTrigger(): string {
		if (task?.schedule) {
			return 'schedule';
		}
		if (task?.webhook) {
			return 'webhook';
		}
		if (task?.email) {
			return 'email';
		}
		if (task?.onSlackMessage) {
			return 'slack';
		}
		if (task?.onDiscordMessage) {
			return 'discord';
		}
		return 'onDemand';
	}

	async function selected(value: string) {
		if (!task) {
			return;
		}
		if (value === 'schedule') {
			task.schedule = {
				interval: 'daily',
				hour: 0,
				minute: 0,
				day: 0,
				weekday: 0,
				timezone: Intl.DateTimeFormat().resolvedOptions().timeZone
			};
			task.webhook = undefined;
			task.email = undefined;
			task.onDemand = undefined;
			task.onSlackMessage = undefined;
			task.onDiscordMessage = undefined;
		}
		if (value === 'webhook') {
			task.schedule = undefined;
			task.webhook = {};
			task.email = undefined;
			task.onDemand = undefined;
			task.onSlackMessage = undefined;
			task.onDiscordMessage = undefined;
		}
		if (value === 'email') {
			task.schedule = undefined;
			task.webhook = undefined;
			task.onDemand = undefined;
			task.email = {};
			task.onSlackMessage = undefined;
			task.onDiscordMessage = undefined;
		}
		if (value === 'onDemand') {
			task.schedule = undefined;
			task.webhook = undefined;
			task.email = undefined;
			task.onDemand = undefined;
			task.onSlackMessage = undefined;
			task.onDiscordMessage = undefined;
		}
		if (value === 'slack') {
			task.schedule = undefined;
			task.webhook = undefined;
			task.email = undefined;
			task.onDemand = undefined;
			task.onSlackMessage = {};
			task.onDiscordMessage = undefined;
		}
		if (value === 'discord') {
			task.schedule = undefined;
			task.webhook = undefined;
			task.email = undefined;
			task.onDemand = undefined;
			task.onSlackMessage = undefined;
			task.onDiscordMessage = {};
		}
	}
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex grow flex-col overflow-visible rounded-2xl bg-white p-5 shadow-sm dark:border"
>
	<div class="border-surface3 mb-4 flex items-center justify-between gap-4 border-b pb-4">
		<h3 class="text-lg font-semibold">Trigger Type</h3>
		<Dropdown
			class="bg-surface2 md:min-w-sm"
			selected={selectedTrigger()}
			values={options}
			onSelected={selected}
			disabled={readOnly}
		/>
	</div>

	{#if task}
		<Trigger bind:task {readOnly} />
	{/if}
</div>
