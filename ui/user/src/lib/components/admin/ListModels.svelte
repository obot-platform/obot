<script lang="ts">
	import { AdminService, type ModelProvider } from '$lib/services';
	import { darkMode, profile } from '$lib/stores';
	import { PictureInPicture2 } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Table from '../table/Table.svelte';
	import { ModelUsage, ModelUsageLabels, type Model } from '$lib/services';
	import Toggle from '../Toggle.svelte';
	import Select from '../Select.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { getAdminModels } from '$lib/context/admin/models.svelte';

	interface Props {
		provider: ModelProvider;
		readonly?: boolean;
	}

	function filterOutModelsByProvider(models: Model[], providerID: string) {
		return models
			.filter((model) => model.modelProvider === providerID)
			.sort((a, b) => a.name.localeCompare(b.name));
	}

	const adminModels = getAdminModels();
	const { provider, readonly }: Props = $props();
	let modelsDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let modelsByProvider = $derived(filterOutModelsByProvider(adminModels.items ?? [], provider.id));

	const usageOptions = Object.entries(ModelUsage).map(([_key, value]) => ({
		label: ModelUsageLabels[value],
		id: value
	}));
</script>

<button
	class="icon-button"
	onclick={() => {
		modelsDialog?.open();
	}}
>
	<PictureInPicture2 class="size-5" />
</button>

<ResponsiveDialog
	bind:this={modelsDialog}
	class="max-w-4xl bg-gray-50 p-0 pb-4 dark:bg-black"
	classes={{ header: 'p-4 pb-0' }}
>
	{#snippet titleContent()}
		{#if darkMode.isDark}
			{@const url = provider.iconDark ?? provider.icon}
			<img
				src={url}
				alt={provider.name}
				class={twMerge('size-9 rounded-md p-1', !provider.iconDark && 'bg-gray-600')}
			/>
		{:else}
			<img src={provider.icon} alt={provider.name} class="bg-surface1 size-9 rounded-md p-1" />
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
			<div class="default-scrollbar-thin h-[500px] overflow-y-auto px-4">
				<Table
					data={modelsByProvider}
					fields={['name', 'usage', 'active']}
					classes={{ root: 'dark:bg-surface1' }}
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
							<Select
								classes={{ root: 'w-full' }}
								class="bg-surface1 dark:bg-surface2 dark:border-surface3 border border-transparent shadow-inner"
								options={usageOptions}
								selected={columnData.usage}
								onSelect={(option) => {
									AdminService.updateModel(columnData.id, {
										...columnData,
										usage: option.id as ModelUsage
									});
									const index = modelsByProvider.findIndex((m) => m.id === columnData.id);
									if (index !== -1) {
										modelsByProvider[index].usage = option.id as ModelUsage;
									}
								}}
								disabled={readonly}
							/>
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
