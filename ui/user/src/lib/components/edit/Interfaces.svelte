<script lang="ts">
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { ChevronRight, Smartphone, Webhook, Mail } from 'lucide-svelte';
	import { getLayout, openSidebarConfig, type Layout } from '$lib/context/layout.svelte';
	import type { SvelteComponent } from 'svelte';
	import type { IconProps } from 'lucide-svelte';

	const layout = getLayout();

	type Option = {
		id: string;
		label: string;
		icon: string | typeof SvelteComponent<IconProps>;
		type: 'image' | 'lucide';
	};

	const options: Option[] = [
		{
			id: 'slack',
			label: 'Slack',
			icon: '/admin/assets/slack_icon_small.svg',
			type: 'image'
		},
		{
			id: 'discord',
			label: 'Discord',
			icon: 'https://cdn.jsdelivr.net/npm/simple-icons@v13/icons/discord.svg',
			type: 'image'
		},
		{
			id: 'sms',
			label: 'SMS',
			icon: Smartphone,
			type: 'lucide'
		},
		{
			id: 'webhook',
			label: 'Webhook',
			icon: Webhook,
			type: 'lucide'
		},
		{
			id: 'email',
			label: 'Email',
			icon: Mail,
			type: 'lucide'
		}
	];
</script>

<CollapsePane classes={{ header: 'pl-3 py-2 text-md', content: 'p-0' }} iconSize={5}>
	{#snippet header()}
		<span class="flex grow items-center gap-2 text-start text-sm font-extralight">
			External Interfaces
		</span>
	{/snippet}

	<div class="flex flex-col p-2">
		{#each options as option}
			<button
				class="hover:bg-surface3 flex min-h-9 items-center justify-between rounded-md bg-transparent p-2 pr-3 text-xs transition-colors duration-200"
				onclick={() => {
					openSidebarConfig(layout, option.id as Layout['sidebarConfig']);
				}}
			>
				<span class="flex items-center gap-2">
					{#if option.type === 'image' && typeof option.icon === 'string'}
						<div class="bg-surface1 flex-shrink-0 rounded-sm p-1 dark:bg-gray-600">
							<img src={option.icon} class="size-4" alt={option.label} />
						</div>
					{:else if option.type === 'lucide' && typeof option.icon !== 'string'}
						<div class="bg-surface1 flex-shrink-0 rounded-sm p-1 dark:bg-gray-600">
							<svelte:component this={option.icon} class="size-4" />
						</div>
					{/if}

					{option.label}
				</span>
				<ChevronRight class="size-4" />
			</button>
		{/each}
	</div>
</CollapsePane>
