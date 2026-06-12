<script lang="ts">
	import AppNotificationBanner from '$lib/components/AppNotificationBanner.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { profile, responsive } from '$lib/stores';
	import { PanelRightOpen, PanelRightClose } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let appNotifications = $state(untrack(() => data.appNotifications));

	const duration = PAGE_TRANSITION_DURATION;
	let saving = $state(false);
	let showConfigurationSidebar = $state(untrack(() => (responsive.isMobile ? false : true)));
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	async function handleSave() {}
</script>

<Layout title="App Notifications">
	{#if responsive.isMobile && !showConfigurationSidebar}
		<div class="fixed top-20 right-4 z-40">
			<IconButton
				onclick={() => (showConfigurationSidebar = !showConfigurationSidebar)}
				tooltip={{ text: 'Open Branding Sidebar' }}
			>
				<PanelRightOpen class="size-6 text-muted-content" />
			</IconButton>
		</div>
	{/if}

	{#snippet rightSidebar()}
		<div
			class={twMerge(
				'bg-base-100 dark:bg-base-200 border-base-300 overflow-y-auto border-l flex flex-col transition-transform',
				responsive.isMobile
					? 'fixed z-40 h-[calc(100dvh-4rem)] w-dvw top-16 translate-x-full'
					: 'static w-sm min-w-sm h-dvh',
				responsive.isMobile && showConfigurationSidebar ? 'translate-x-0' : ''
			)}
		>
			<div class="flex flex-col divide-y divide-base-300">
				{#if responsive.isMobile}
					<div
						class="flex justify-between items-center p-4 sticky bg-base-100 dark:bg-base-200 top-0 left-0"
					>
						<h3 class="text-base font-semibold">Configuration</h3>
						<IconButton onclick={() => (showConfigurationSidebar = !showConfigurationSidebar)}>
							<PanelRightClose class="size-6 text-muted-content" />
						</IconButton>
					</div>
				{/if}
				<div class="flex items-center justify-between px-4 py-2 h-16">
					{#if !responsive.isMobile}
						<h3 class="text-base font-semibold">Configuration</h3>
					{/if}
				</div>
				<div class="flex flex-col gap-2 p-4">
					<div class="flex justify-between items-center gap-4 pb-1">
						<p class="text-sm font-medium">Enable Banner</p>
						<input
							type="checkbox"
							class="toggle toggle-sm"
							bind:checked={appNotifications.banner.enabled}
						/>
					</div>

					<p class="text-xs font-light text-muted-content">
						Once banner is enabled, it will be displayed at the top of the page across all pages.
						Additional properties for the banner can be modified below.
					</p>
				</div>
				<div class="flex flex-col gap-2 p-4">
					<p class="text-sm font-medium mb-2">Banner Properties</p>
					<div class="flex items-center justify-between">
						<p class="text-sm font-light">Type</p>
						<select
							class="select select-sm max-w-46 min-w-0"
							bind:value={appNotifications.banner.type}
						>
							<option value="info">Info</option>
							<option value="warning">Warning</option>
						</select>
					</div>
					<div class="flex flex-col gap-2">
						<p class="text-sm font-light">Text</p>
						<textarea
							class="input-text-filled min-h-[120px] resize-y"
							bind:value={appNotifications.banner.text}
						></textarea>
					</div>
					<div class="flex items-center justify-between">
						<p class="text-sm font-light">Dismissable</p>
						<input
							type="checkbox"
							class="toggle toggle-sm"
							bind:checked={appNotifications.banner.dismissable}
						/>
					</div>
				</div>
			</div>
			<div class="flex grow"></div>
			{#if !isAdminReadonly}
				<div
					class="sticky bottom-0 left-0 w-full bg-base-100 dark:bg-base-200 px-4 py-2 border-t border-base-300"
				>
					<div class="flex justify-end items-center gap-2">
						<div class="flex items-center gap-2">
							<button class="btn btn-primary" onclick={handleSave}>
								{#if saving}
									<Loading class="size-4" />
								{:else}
									Save
								{/if}
							</button>
							<button
								class="btn btn-secondary"
								onclick={() => {
									//TODO:
								}}>Cancel</button
							>
						</div>
					</div>
				</div>
			{/if}
		</div>
	{/snippet}
	<div class="relative h-full w-full @container mb-8" transition:fade={{ duration }}>
		<div
			class="mockup-window border border-base-300 w-full bg-base-100 dark:bg-base-200 h-[50vh] min-h-96px relative"
		>
			<div class="w-full">
				{#if appNotifications.banner.enabled}
					<AppNotificationBanner
						data={appNotifications.banner}
						placeholder="[banner placeholder text]"
					/>
				{/if}
			</div>
			<div class="w-full h-full flex justify-center items-center absolute top-0 left-0">
				<p class="text-muted-content font-light text-sm">
					<b class="font-semibold">Enable Banner</b> to see an example of the banner in action.
				</p>
			</div>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | App Notifications</title>
</svelte:head>
