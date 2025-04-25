<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { getLayout, openSidebarConfig, type Layout } from '$lib/context/layout.svelte';
	import type { Project } from '$lib/services';
	import { Settings } from 'lucide-svelte';
	import DotDotDot from '../DotDotDot.svelte';

	interface Props {
		project: Project;
		selected?: string[];
	}

	const mockSelected = ['chatbot', 'slack'];
	let { project, selected = mockSelected }: Props = $props();
	let layout = getLayout();

	const options = [
		{
			id: 'chatbot',
			label: 'Chatbot',
			icon: ''
		},
		{
			id: 'slack',
			label: 'Slack',
			icon: '/admin/assets/slack_icon_small.svg'
		},
		{
			id: 'discord',
			label: 'Discord',
			icon: 'https://cdn.jsdelivr.net/npm/simple-icons@v13/icons/discord.svg'
		},
		{
			id: 'sms',
			label: 'SMS',
			icon: ''
		},
		{
			id: 'email',
			label: 'Email',
			icon: ''
		},
		{
			id: 'webhook',
			label: 'Webhook',
			icon: ''
		}
	];
	const optionMap = new Map(options.map((option) => [option.id, option]));
</script>

<div class="flex flex-col gap-2">
	<div class="mb-1 flex items-center justify-between">
		<p class="grow text-sm font-semibold">Interfaces</p>
		<DotDotDot class="p-0">
			{#snippet icon()}
				<Settings class="icon-button size-5" />
			{/snippet}
			<div class="default-dialog flex w-32 flex-col p-2">
				{#each options as option}
					<button
						class="menu-button"
						onclick={() => openSidebarConfig(layout, option.id as Layout['sidebarConfig'])}
					>
						{option.label}
					</button>
				{/each}
			</div>
		</DotDotDot>
	</div>

	<div class="flex flex-col gap-2">
		{#each selected as item}
			{@const configedInterface = optionMap.get(item)}
			{#if configedInterface}
				<div
					class="group flex min-h-6 w-full items-center rounded-md transition-colors duration-300"
				>
					<button class="flex h-full grow items-center gap-2 pl-1.5" onclick={() => {}}>
						<div class="rounded-md bg-gray-50 p-1 dark:bg-gray-600">
							<img
								src={configedInterface.icon || '/user/images/obot-icon-blue.svg'}
								class="size-4"
								alt={configedInterface.label}
							/>
						</div>
						<p class="w-[calc(100%-24px)] truncate text-left text-xs font-light">
							{configedInterface.label}
						</p>
					</button>
					<button
						class="py-2 pr-3 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
						onclick={() =>
							openSidebarConfig(layout, configedInterface.id as Layout['sidebarConfig'])}
					>
						<Settings class="size-4" />
					</button>
				</div>
			{/if}
		{/each}
	</div>
</div>
