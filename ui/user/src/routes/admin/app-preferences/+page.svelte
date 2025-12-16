<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { LoaderCircle, Pencil } from 'lucide-svelte';
	import { fade } from 'svelte/transition';
	import { darkMode, profile } from '$lib/stores/index.js';
	import { AdminService, type AppPreferences } from '$lib/services';
	import appPreferences, { compileAppPreferences } from '$lib/stores/appPreferences.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { twMerge } from 'tailwind-merge';
	import { onDestroy, untrack } from 'svelte';
	import { browser } from '$app/environment';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { invalidateAll } from '$app/navigation';
	import UploadImage from '$lib/components/UploadImage.svelte';

	const duration = PAGE_TRANSITION_DURATION;
	let { data } = $props();
	let form = $state<AppPreferences>(untrack(() => data.appPreferences));
	let prevAppPreferences = $state<AppPreferences>(untrack(() => data.appPreferences));
	let saving = $state(false);
	let showSaved = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();
	let displayPreviewMode = $state(false);
	let selectedColorTab = $state<'light' | 'dark'>('light');
	let selectedIconTab = $state<'light' | 'dark'>('light');

	let editUrlDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let uploadImage = $state<ReturnType<typeof UploadImage>>();
	let selectedImageField = $state<keyof AppPreferences['logos']>();
	let editImageUrl = $state<string>('');

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	onDestroy(() => {
		if (browser) {
			appPreferences.setThemeColors(appPreferences.current.theme);
		}
	});

	async function handleSave() {
		if (timeout) {
			clearTimeout(timeout);
		}
		saving = true;
		try {
			appPreferences.current = form;
			appPreferences.setThemeColors(form.theme);
			await AdminService.updateAppPreferences(form);
			await invalidateAll();
			prevAppPreferences = form;
			showSaved = true;
			timeout = setTimeout(() => {
				showSaved = false;
			}, 3000);
		} catch (err) {
			console.error(err);
			// default behavior will show snackbar error
		} finally {
			saving = false;
		}
	}

	const standardIconFields: { id: keyof AppPreferences['logos']; label: string }[] = [
		{
			id: 'logoIcon',
			label: 'Default Icon'
		},
		{
			id: 'logoIconError',
			label: 'Error Icon'
		},
		{
			id: 'logoIconWarning',
			label: 'Warning Icon'
		}
	];

	const themeLightLogoFields: { id: keyof AppPreferences['logos']; label: string }[] = [
		{
			id: 'logoDefault',
			label: 'Full Logo'
		},
		{
			id: 'logoEnterprise',
			label: 'Full Enterprise Logo'
		},
		{
			id: 'logoChat',
			label: 'Full Chat Logo'
		}
	];

	const themeDarkLogoFields: { id: keyof AppPreferences['logos']; label: string }[] = [
		{
			id: 'darkLogoDefault',
			label: 'Full Logo'
		},
		{
			id: 'darkLogoEnterprise',
			label: 'Full Enterprise Logo'
		},
		{
			id: 'darkLogoChat',
			label: 'Full Chat Logo'
		}
	];

	const themeLightColorFields: { id: keyof AppPreferences['theme']; label: string }[][] = [
		[
			{
				id: 'primaryColor',
				label: 'Primary'
			}
		],
		[
			{
				id: 'backgroundColor',
				label: 'Background'
			},
			{
				id: 'onBackgroundColor',
				label: 'Primary Text'
			},
			{
				id: 'onSurfaceColor',
				label: 'Secondary Text'
			}
		],
		[
			{
				id: 'surface1Color',
				label: 'Surface 1'
			},
			{
				id: 'surface2Color',
				label: 'Surface 2'
			},
			{
				id: 'surface3Color',
				label: 'Surface 3'
			}
		]
	];

	const themeDarkColorFields: { id: keyof AppPreferences['theme']; label: string }[][] = [
		[
			{
				id: 'darkPrimaryColor',
				label: 'Primary'
			}
		],
		[
			{
				id: 'darkBackgroundColor',
				label: 'Background'
			},
			{
				id: 'darkOnBackgroundColor',
				label: 'Primary Text'
			},
			{
				id: 'darkOnSurfaceColor',
				label: 'Secondary Text'
			}
		],
		[
			{
				id: 'darkSurface1Color',
				label: 'Surface 1'
			},
			{
				id: 'darkSurface2Color',
				label: 'Surface 2'
			},
			{
				id: 'darkSurface3Color',
				label: 'Surface 3'
			}
		]
	];
</script>

<Layout classes={{ container: 'pb-0' }}>
	<div class="relative h-full w-full" transition:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<div class="flex items-center gap-4">
				<h1 class="text-2xl font-semibold">App Preferences</h1>
				<button
					class="button text-xs"
					onclick={() => {
						form = compileAppPreferences();
						if (displayPreviewMode) {
							appPreferences.setThemeColors(form.theme);
						}
					}}
				>
					Restore Default
				</button>
			</div>

			<div class="flex flex-col gap-1">
				<div class="flex justify-between gap-4">
					<h2 class="text-lg font-semibold">Theme Colors</h2>
					<Toggle
						label="Live Preview Mode"
						labelInline
						checked={displayPreviewMode}
						onChange={(checked) => {
							displayPreviewMode = checked;
							appPreferences.setThemeColors(checked ? form.theme : appPreferences.current.theme);
						}}
					/>
				</div>
				<div class="paper p-0">
					<div class="flex">
						<button
							class={twMerge(
								'page-tab max-w-full flex-1',
								selectedColorTab === 'light' && 'page-tab-active'
							)}
							onclick={() => (selectedColorTab = 'light')}
						>
							Light Scheme
						</button>
						<button
							class={twMerge(
								'page-tab max-w-full flex-1',
								selectedColorTab === 'dark' && 'page-tab-active'
							)}
							onclick={() => (selectedColorTab = 'dark')}
						>
							Dark Scheme
						</button>
					</div>
					<div class="flex flex-col gap-8 pt-4 pb-8">
						{#if selectedColorTab === 'light'}
							{@render colorFields('light')}
						{:else}
							{@render colorFields('dark')}
						{/if}
					</div>
				</div>
			</div>
			<div class="flex flex-col gap-1">
				<h2 class="text-lg font-semibold">Icons & Logos</h2>
				{@render iconFields('standard')}
			</div>
			<div class="flex flex-col gap-1">
				<div
					class={twMerge(
						'paper p-0',
						!darkMode.isDark &&
							selectedIconTab === 'dark' &&
							'bg-[var(--theme-background-dark)] text-[var(--theme-on-background-dark)]',
						darkMode.isDark &&
							selectedIconTab === 'light' &&
							'bg-[var(--theme-background-light)] text-[var(--theme-on-background-light)]'
					)}
				>
					<div class="flex">
						<button
							class={twMerge(
								'page-tab max-w-full flex-1',
								selectedIconTab === 'light' &&
									'page-tab-active hover:bg-[var(--theme-surface1-light)]',
								selectedIconTab === 'dark' && 'hover:bg-[var(--theme-surface2-dark)]'
							)}
							onclick={() => (selectedIconTab = 'light')}
						>
							Light Scheme
						</button>
						<button
							class={twMerge(
								'page-tab max-w-full flex-1',
								selectedIconTab === 'dark' &&
									'page-tab-active hover:bg-[var(--theme-surface2-dark)]',
								selectedIconTab === 'light' &&
									'bg-[var(--theme-background-light)] hover:bg-[var(--theme-surface1-light)]'
							)}
							onclick={() => (selectedIconTab = 'dark')}
						>
							Dark Scheme
						</button>
					</div>
					<div class="flex flex-col gap-8 pt-4 pb-8">
						{#if selectedIconTab === 'light'}
							{@render iconFields('light')}
						{:else}
							{@render iconFields('dark')}
						{/if}
					</div>
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<h2 class="text-lg font-semibold">Footer Branding</h2>
				<div class="paper flex flex-col gap-4">
					<label class="flex flex-col gap-1">
						<span class="text-on-surface1 text-sm font-semibold">Product Name</span>
						<input
							type="text"
							class="ghost-input"
							bind:value={form.branding.productName}
							placeholder="Obot"
							disabled={isAdminReadonly}
						/>
						<span class="text-on-surface1 text-xs">
							The product name displayed in the footer message
						</span>
					</label>

					<label class="flex flex-col gap-1">
						<span class="text-on-surface1 text-sm font-semibold">Issue Report URL</span>
						<input
							type="url"
							class="ghost-input"
							bind:value={form.branding.issueReportUrl}
							placeholder="https://github.com/..."
							disabled={isAdminReadonly}
						/>
						<span class="text-on-surface1 text-xs">
							URL for the "Report issues here" link in the footer
						</span>
					</label>

					<label class="flex flex-col gap-1">
						<span class="text-on-surface1 text-sm font-semibold">Footer Message</span>
						<input
							type="text"
							class="ghost-input"
							bind:value={form.branding.footerMessage}
							placeholder={"{productName} isn't perfect. Double check its work."}
							disabled={isAdminReadonly}
						/>
						<span class="text-on-surface1 text-xs">
							Use <code class="bg-surface2 rounded px-1">{'{productName}'}</code> as a placeholder for
							the product name
						</span>
					</label>

					<Toggle
						label="Show Footer"
						labelInline
						checked={form.branding.showFooter}
						onChange={(checked) => {
							form = { ...form, branding: { ...form.branding, showFooter: checked } };
						}}
						disabled={isAdminReadonly}
					/>
				</div>
			</div>

			{#if !isAdminReadonly}
				<div
					class="bg-surface1 dark:bg-background sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8"
				>
					{#if showSaved}
						<span
							in:fade={{ duration: 200 }}
							class="text-on-surface1 flex min-h-10 items-center px-4 text-sm font-extralight"
						>
							Your changes have been saved.
						</span>
					{/if}

					<button
						class="button hover:bg-surface3 flex items-center gap-1 bg-transparent"
						onclick={() => {
							form = prevAppPreferences;
							appPreferences.setThemeColors(prevAppPreferences.theme);
						}}
					>
						Cancel
					</button>
					<button
						class="button-primary flex items-center gap-1"
						disabled={saving}
						onclick={handleSave}
					>
						{#if saving}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							Save
						{/if}
					</button>
				</div>
			{:else}
				<div class="h-4"></div>
			{/if}
		</div>
	</div>
</Layout>

{#snippet colorFields(scheme: 'light' | 'dark')}
	{@const fieldsToUse = scheme === 'light' ? themeLightColorFields : themeDarkColorFields}
	{#each fieldsToUse as fieldset, i (i)}
		<div class="grid grid-cols-3 gap-4">
			{#each fieldset as field (field.id)}
				<div class="flex flex-col items-center justify-center gap-1">
					<label class="text-on-surface1 text-xs" for={field.id}>{field.label}</label>
					<div class="relative">
						<div
							class="size-8 rounded-full border dark:border-white"
							style="background-color: {form.theme[field.id]}"
						></div>
						<input
							class="absolute top-0 left-0 size-8 cursor-pointer opacity-0"
							type="color"
							id={field.id}
							value={form.theme[field.id].startsWith('#') ? form.theme[field.id] : '#ffffff'}
							oninput={(e) => {
								if (!e.currentTarget.value.startsWith('#')) {
									return;
								}
								const newForm = {
									...form,
									theme: { ...form.theme, [field.id]: e.currentTarget.value }
								};
								if (displayPreviewMode) {
									appPreferences.setThemeColors(newForm.theme);
								}
								form = newForm;
							}}
						/>
					</div>
				</div>
			{/each}
		</div>
	{/each}
{/snippet}

{#snippet iconFields(type: 'standard' | 'light' | 'dark')}
	{@const fieldsToUse =
		type === 'standard'
			? standardIconFields
			: type === 'light'
				? themeLightLogoFields
				: themeDarkLogoFields}
	<div
		class={twMerge(
			type === 'standard' ? 'grid grid-cols-2 gap-8 md:grid-cols-4' : 'flex flex-col gap-8'
		)}
	>
		{#each fieldsToUse as field (field.id)}
			<button
				class={twMerge(
					'group active:bg-surface1 dark:active:bg-surface3 relative flex flex-col items-center justify-center gap-2',
					type === 'standard' ? 'paper gap-4' : 'w-full self-center rounded-sm p-2 md:w-xl'
				)}
				onclick={() => {
					editImageUrl = form.logos[field.id].startsWith('/user/images/')
						? ''
						: form.logos[field.id];
					selectedImageField = field.id;
					editUrlDialog?.open();
				}}
			>
				<p class="text-on-surface1 text-sm">{field.label}</p>
				<img
					src={form.logos[field.id]}
					alt={field.label}
					class={twMerge(
						'flex-shrink-0 object-contain',
						type === 'standard' ? 'h-24' : 'max-h-32 max-w-full'
					)}
				/>
				<Pencil
					class="text-on-surface1 absolute top-2 right-2 size-6 opacity-0 transition-opacity group-hover:opacity-100"
				/>
			</button>
		{/each}
	</div>
{/snippet}

<ResponsiveDialog
	bind:this={editUrlDialog}
	title={editImageUrl ? 'Edit Image URL' : 'Add Image URL'}
>
	<UploadImage
		label="Upload Image"
		onUpload={(imageUrl: string) => {
			editImageUrl = imageUrl;
		}}
		variant="preview"
		bind:this={uploadImage}
	/>
	<div class="flex grow"></div>
	<div class="flex justify-end gap-2">
		<button
			class="button-primary mt-4 w-full md:w-fit"
			onclick={() => {
				if (!selectedImageField) return;
				const newForm = {
					...form,
					logos: { ...form.logos, [selectedImageField]: editImageUrl }
				};
				form = newForm;
				editImageUrl = '';
				selectedImageField = undefined;
				editUrlDialog?.close();
				uploadImage?.clearPreview();
			}}>Apply</button
		>
	</div>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | App Preferences</title>
</svelte:head>
