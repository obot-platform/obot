<script lang="ts">
	import { getAdminModels } from '$lib/context/admin/models.svelte';
	import { AdminService, ModelUsageLabels, type ModelProvider } from '$lib/services';
	import { ModelUsage, type Model } from '$lib/services';
	import { darkMode, profile } from '$lib/stores';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Toggle from '../Toggle.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import { PictureInPicture2 } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		provider: ModelProvider;
		readonly?: boolean;
	}

	function filterOutModelsByProvider(models: Model[], providerID: string) {
		return models
			.filter((model) => model.modelProvider === providerID)
			.sort((a, b) => {
				const aIsLlm = a.usage === ModelUsage.LLM;
				const bIsLlm = b.usage === ModelUsage.LLM;
				if (aIsLlm !== bIsLlm) {
					return aIsLlm ? -1 : 1;
				}
				return a.name.localeCompare(b.name);
			});
	}

	const adminModels = getAdminModels();
	const { provider, readonly }: Props = $props();
	let modelsDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let modelsByProvider = $derived(filterOutModelsByProvider(adminModels.items ?? [], provider.id));
</script>

<IconButton
	onclick={() => {
		modelsDialog?.open();
	}}
>
	<PictureInPicture2 class="size-5" />
</IconButton>

<ResponsiveDialog
	bind:this={modelsDialog}
	class="bg-base-200 dark:bg-base-100 max-w-4xl"
	classes={{ header: 'p-4 pb-0', content: 'p-0 pb-4' }}
>
	{#snippet titleContent()}
		{#if darkMode.isDark}
			{@const url = provider.iconDark ?? provider.icon}
			<img
				src={url}
				alt={provider.name}
				class={twMerge('size-9 rounded-md p-1', !provider.iconDark && 'bg-base-300')}
			/>
		{:else}
			<img src={provider.icon} alt={provider.name} class="bg-base-200 size-9 rounded-md p-1" />
		{/if}
		{provider.name} Models
	{/snippet}
	{#if provider}
		<form class="flex flex-col gap-4" onsubmit={(e) => e.preventDefault()}>
			<input
				type="text"
				autocomplete="email"
				name="email"
				value={profile.current.email}
				class="hidden"
				disabled={readonly}
			/>
			<div class="default-scrollbar-thin h-125 overflow-y-auto px-4">
				<Table
					data={modelsByProvider}
					fields={['name', 'usage', 'active']}
					classes={{ root: 'dark:bg-base-200' }}
					setRowClasses={(row) => {
						return row.usage !== ModelUsage.LLM ? 'text-muted-content' : '';
					}}
				>
					{#snippet onRenderColumn(field, columnData)}
						{#if field === 'active'}
							<Toggle
								checked={columnData.active}
								onChange={(value) => {
									AdminService.updateModel(columnData.id, {
										...columnData,
										active: value
									});
									const index = modelsByProvider.findIndex((m) => m.id === columnData.id);
									if (index !== -1) {
										modelsByProvider[index].active = value;
									}
								}}
								label="Toggle Active Model"
								disabled={readonly}
							/>
						{:else if field === 'usage'}
							<span class="badge badge-xs badge-secondary">
								{ModelUsageLabels[columnData.usage as ModelUsage]}
							</span>
						{:else if field === 'name'}
							{columnData.displayName ? columnData.displayName : columnData.name}
						{:else}
							{columnData[field as keyof typeof columnData]}
						{/if}
					{/snippet}
				</Table>
			</div>
		</form>
	{/if}
</ResponsiveDialog>
