<script lang="ts">
	import { browser } from '$app/environment';
	import { invalidateAll } from '$app/navigation';
	import Layout from '$lib/components/Layout.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import UploadImage from '$lib/components/UploadImage.svelte';
	import CustomConfigurationForm from '$lib/components/mcp/CustomConfigurationForm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, type AppPreferences } from '$lib/services';
	import { darkMode, profile } from '$lib/stores';
	import appPreferences, { compileAppPreferences } from '$lib/stores/appPreferences.svelte';
	import { formatTimeAgo } from '$lib/time';
	import 'devicon/devicon.min.css';
	import { CircleAlert, HouseIcon, Info, LoaderCircle, Pencil, X } from 'lucide-svelte';
	import { onDestroy, untrack } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const duration = PAGE_TRANSITION_DURATION;
	let { data } = $props();
	let form = $state<AppPreferences>(untrack(() => data.appPreferences));
	let prevAppPreferences = $state<AppPreferences>(untrack(() => data.appPreferences));
	let saving = $state(false);
	let showSaved = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();
	let displayPreviewMode = $state(false);

	/** Preview rows for the MCP Servers-style table; `devicon` is a Devicon class name (e.g. `devicon-python-plain`). */
	type BrandingMockConnectorRow = {
		id: string;
		name: string;
		devicon: string;
		/** Optional manifest-style icon URL; UI can prefer this over `devicon` when set */
		icon?: string;
		type: 'single' | 'multi' | 'remote' | 'composite';
		status: string;
		created: string;
		registry: string;
		users: number;
	};

	const mockTableData: BrandingMockConnectorRow[] = [
		{
			id: 'mock-braintree',
			name: 'Braintree MCP',
			devicon: 'devicon-python-plain',
			type: 'single',
			status: 'Connected',
			created: new Date(Date.now() - 1000 * 60 * 45).toISOString(),
			registry: 'Global Registry',
			users: 1
		},
		{
			id: 'mock-acme-api',
			name: 'Acme Remote API',
			devicon: 'devicon-typescript-plain',
			type: 'remote',
			status: 'Requires OAuth Config',
			created: new Date(Date.now() - 1000 * 60 * 60 * 20).toISOString(),
			registry: 'Global Registry',
			users: 0
		},
		{
			id: 'mock-analytics',
			name: 'Analytics Warehouse',
			devicon: 'devicon-postgresql-plain',
			type: 'multi',
			status: 'Connected',
			created: new Date(Date.now() - 1000 * 60 * 60 * 24 * 3).toISOString(),
			registry: 'My Registry',
			users: 12
		},
		{
			id: 'mock-compose',
			name: 'Composite Toolkit',
			devicon: 'devicon-docker-plain',
			type: 'composite',
			status: '',
			created: new Date(Date.now() - 1000 * 60 * 60 * 24 * 14).toISOString(),
			registry: 'Global Registry',
			users: 4
		},
		{
			id: 'mock-slack',
			name: 'Slack Connector',
			devicon: 'devicon-slack-plain',
			type: 'remote',
			status: '',
			created: new Date(Date.now() - 1000 * 60 * 60 * 24 * 30).toISOString(),
			registry: "Partner's Registry",
			users: 0
		},
		{
			id: 'mock-react',
			name: 'UI Automation Server',
			devicon: 'devicon-react-original',
			type: 'single',
			status: 'Connected',
			created: new Date(Date.now() - 1000 * 60 * 8).toISOString(),
			registry: 'Global Registry',
			users: 2
		}
	];

	let editUrlDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let uploadImage = $state<ReturnType<typeof UploadImage>>();
	let selectedImageField = $state<keyof AppPreferences['logos']>();
	let editImageUrl = $state<string>('');

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	let selectedColorScheme = $state(untrack(() => (darkMode.isDark ? 'dark' : 'light')));
	let initialColorScheme = $state(untrack(() => (darkMode.isDark ? 'dark' : 'light')));
	let selectedSurfaceMode = $state<'solid' | 'tinted'>('solid');
	let selectedIndicatorMode = $state<'solid' | 'tinted'>('solid');
	let selectedConfigurationMode = $state<'theme' | 'logos'>('theme');

	onDestroy(() => {
		if (browser) {
			darkMode.setDark(initialColorScheme === 'dark');
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

	const themeLightSurfaceFields: { id: keyof AppPreferences['theme']; label: string }[] = [
		{
			id: 'backgroundColor',
			label: 'Background'
		},
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
	];

	const themeDarkSurfaceFields: { id: keyof AppPreferences['theme']; label: string }[] = [
		{
			id: 'darkBackgroundColor',
			label: 'Background'
		},
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
	];

	const themeLightIndicatorFields: { id: keyof AppPreferences['theme']; label: string }[] = [
		{
			id: 'secondaryColor',
			label: 'Secondary'
		},
		{
			id: 'successColor',
			label: 'Success'
		},
		{
			id: 'warningColor',
			label: 'Warning'
		},
		{
			id: 'errorColor',
			label: 'Error'
		}
	];

	const themeDarkIndicatorFields: { id: keyof AppPreferences['theme']; label: string }[] = [
		{
			id: 'darkSecondaryColor',
			label: 'Secondary'
		},
		{
			id: 'darkSuccessColor',
			label: 'Success'
		},
		{
			id: 'darkWarningColor',
			label: 'Warning'
		},
		{
			id: 'darkErrorColor',
			label: 'Error'
		}
	];

	const textLightFields: { id: keyof AppPreferences['theme']; label: string }[] = [
		{
			id: 'onBackgroundColor',
			label: 'Primary Text'
		},
		{
			id: 'onPrimaryColor',
			label: 'Primary Text'
		},
		{
			id: 'onSuccessColor',
			label: 'Success Text'
		},
		{
			id: 'onWarningColor',
			label: 'Warning Text'
		},
		{
			id: 'onErrorColor',
			label: 'Error Text'
		}
	];

	const textDarkFields: { id: keyof AppPreferences['theme']; label: string }[] = [
		{
			id: 'darkOnBackgroundColor',
			label: 'Primary Text'
		},
		{
			id: 'darkOnPrimaryColor',
			label: 'Primary Text'
		},
		{
			id: 'darkOnSuccessColor',
			label: 'Success Text'
		},
		{
			id: 'darkOnWarningColor',
			label: 'Warning Text'
		},
		{
			id: 'darkOnErrorColor',
			label: 'Error Text'
		}
	];
</script>

<Layout title="Branding" classes={{ container: 'pb-0' }}>
	{#snippet rightSidebar()}
		<div
			class="bg-base-100 dark:bg-base-200 border-base-300 h-dvh w-sm min-w-sm overflow-y-auto border-l flex flex-col"
		>
			<div class="flex flex-col divide-y divide-base-300">
				<div class="flex items-center justify-between px-4 py-2">
					<h3 class="text-base font-semibold">Configuration</h3>
					<div class="flex items-center gap-2 p-1.5 bg-base-200 rounded-md shadow-inner">
						<button
							class={twMerge(
								'btn btn-sm',
								selectedConfigurationMode === 'theme' ? 'btn-primary' : 'btn-secondary'
							)}
							onclick={() => {
								selectedConfigurationMode = 'theme';
							}}>Theme</button
						>
						<button
							class={twMerge(
								'btn btn-sm',
								selectedConfigurationMode === 'logos' ? 'btn-primary' : 'btn-secondary'
							)}
							onclick={() => {
								selectedConfigurationMode = 'logos';
							}}>Logos</button
						>
					</div>
				</div>
				<div class="flex items-center justify-between px-4 py-2">
					<p class="text-sm font-medium">Mode</p>
					<div class="flex items-center gap-2 p-1.5 bg-base-200 rounded-md shadow-inner">
						<button
							class={twMerge(
								'btn btn-sm',
								selectedColorScheme === 'light' ? 'btn-primary' : 'btn-secondary'
							)}
							onclick={() => {
								selectedColorScheme = 'light';
								darkMode.setDark(false);
								appPreferences.setThemeColors(form.theme);
							}}>Light</button
						>
						<button
							class={twMerge(
								'btn btn-sm',
								selectedColorScheme === 'dark' ? 'btn-primary' : 'btn-secondary'
							)}
							onclick={() => {
								selectedColorScheme = 'dark';
								darkMode.setDark(true);
								appPreferences.setThemeColors(form.theme);
							}}>Dark</button
						>
					</div>
				</div>

				{#if selectedConfigurationMode === 'theme'}
					{@render themeConfiguration()}
				{/if}
				{#if selectedConfigurationMode === 'logos'}
					{@render logosConfiguration()}
				{/if}
			</div>
			<div class="flex grow"></div>
			{#if !isAdminReadonly}
				<div
					class="sticky bottom-0 left-0 w-full bg-base-100 dark:bg-base-200 px-4 py-2 border-t border-base-300"
				>
					<div class="flex justify-between items-center gap-2">
						<button
							class="btn btn-sm dark:text-white"
							onclick={() => {
								form = compileAppPreferences();
								if (displayPreviewMode) {
									appPreferences.setThemeColors(form.theme);
								}
							}}
						>
							Restore Default
						</button>
						<div class="flex items-center gap-2">
							<button class="btn btn-primary" onclick={handleSave}>
								{#if saving}
									<LoaderCircle class="size-4 animate-spin" />
								{:else}
									Save
								{/if}
							</button>
							<button
								class="btn btn-secondary"
								onclick={() => {
									form = prevAppPreferences;
									appPreferences.setThemeColors(prevAppPreferences.theme);
								}}>Cancel</button
							>
						</div>
					</div>
				</div>
			{/if}
		</div>
	{/snippet}
	{#if showSaved}
		<div class="absolute bottom-0 right-0">
			<span
				in:fade={{ duration: 200 }}
				class="text-on-surface1 flex min-h-10 items-center px-4 text-sm font-extralight"
			>
				Your changes have been saved.
			</span>
		</div>
	{/if}
	<div class="relative h-full w-full @container mb-8" transition:fade={{ duration }}>
		<div>
			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6 shrink-0" />
					<div class="flex flex-col gap-1">
						<p class="font-semibold">Example Components</p>
						<p>
							Below are some example components used in the application for easy previewing. This
							itself is a commonly used notification that is displayed to provide information to the
							user in a detail view.
						</p>
					</div>
				</div>
			</div>
		</div>
		<div class="grid grid-cols-12 gap-4 mt-8">
			<div class="relative h-72 col-span-12 @min-[768px]:col-span-6">
				<div
					class="absolute top-1/2 left-1/2 flex w-md -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-4"
				>
					<Logo class="h-16" />
					<h1 class="text-2xl font-semibold">Welcome to Obot</h1>
					<p class="text-md text-on-surface1 mb-1 text-center font-light">
						Log in or create your account to continue
					</p>

					<div
						class="dark:border-surface3 dark:bg-gray-930 bg-background flex w-sm flex-col gap-4 rounded-xl border border-transparent p-4 shadow-sm"
					>
						<button
							class="group bg-surface2 hover:bg-surface3 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold transition-colors duration-200"
						>
							<img
								class="h-6 w-6 rounded-full bg-transparent p-1 dark:bg-gray-600"
								src="/user/images/github-mark/github-mark.svg"
								alt="Github"
							/>
							<span class="text-center text-sm font-light">Continue with Github</span>
						</button>
					</div>
				</div>
			</div>
			<div class="flex justify-center items-center col-span-12 @min-[768px]:col-span-6">
				<div class="dialog-container max-w-md">
					<div class="dialog-title p-4 pb-0">
						Confirm Action
						<button type="button">
							<X class="size-5" />
						</button>
					</div>
					<div class="flex flex-col items-center justify-center gap-2 p-4 pt-0">
						<div class="rounded-full p-2 bg-primary/10">
							<CircleAlert class="size-8 text-primary" />
						</div>
						<p class="text-center text-base font-medium">
							Are you sure you want to confirm this action?
						</p>

						<div class="mb-4 self-center text-center font-light">
							<p>
								This is an example of a confirmation dialog. It can be used to confirm any action
								that is irreversible or information that needs to be conveyed before submission.
							</p>
						</div>

						<div
							class="flex w-full flex-col items-center justify-center gap-2 @min-[768px]:flex-col @min-[768px]:justify-end"
						>
							<button type="button" class="flex w-full justify-center p-3 btn btn-primary">
								Confirm
							</button>
							<button type="button" class="btn btn-secondary w-full justify-center">Cancel</button>
						</div>
					</div>
				</div>
			</div>
		</div>
		<div class="flex gap-4 items-center flex-wrap mt-8">
			<div class="flex gap-4 grow flex-wrap">
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-4 flex gap-4">
					<button class="btn btn-circle btn-primary"><HouseIcon /></button>
					<button class="btn btn-primary">Confirm</button>
				</div>
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-4 flex gap-4">
					<button class="btn btn-circle btn-secondary"><HouseIcon /></button>
					<button class="btn btn-secondary">Confirm</button>
				</div>
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-4 flex gap-4">
					<button class="btn btn-circle btn-success"><HouseIcon /></button>
					<button class="btn btn-success">Confirm</button>
				</div>
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-4 flex gap-4">
					<button class="btn btn-circle btn-warning"><HouseIcon /></button>
					<button class="btn btn-warning">Confirm</button>
				</div>
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-4 flex gap-4">
					<button class="btn btn-circle btn-error"><HouseIcon /></button>
					<button class="btn btn-error">Confirm</button>
				</div>
			</div>
		</div>
		<div class="w-full mt-8">
			<div class="dark:bg-surface2 bg-background rounded-t-md shadow-sm">
				<div class="flex">
					<button class="page-tab w-1/2 max-w-1/2 page-tab-active"> Servers </button>
					<button class="page-tab w-1/2 max-w-1/2"> Users </button>
				</div>
				<Table data={mockTableData} fields={['name', 'status', 'created', 'registry', 'users']}>
					{#snippet onRenderColumn(field: string, row: BrandingMockConnectorRow)}
						{#if field === 'name'}
							<span class="flex items-center gap-2">
								<i class={twMerge('devicon', row.devicon)}></i>
								{row.name}
							</span>
						{/if}
						{#if field === 'status'}
							{#if row.status === 'Connected'}
								<div class="pill-primary bg-primary">{row.status}</div>
							{:else}
								<div class="text-xs font-light">{row.status}</div>
							{/if}
						{/if}
						{#if field === 'created'}
							{formatTimeAgo(row.created).relativeTime}
						{/if}
					{/snippet}
				</Table>
			</div>
		</div>

		<div class="w-full paper my-8">
			<h4 class="text-lg font-semibold">Custom Form</h4>
			<div class="flex flex-col gap-1">
				<label for="description" class="text-sm font-light">Description</label>
				<input class="text-input-filled" placeholder="Write a description..." />
			</div>
			<div class="flex gap-4 items-center justify-between">
				<p class="text-sm font-light">Example Selector</p>
				<div class="flex grow">
					<Select
						class="bg-surface1 dark:bg-background dark:border-surface3 border border-transparent shadow-inner"
						classes={{
							root: 'flex grow'
						}}
						selected="a"
						options={[
							{ label: 'Option 1', id: 'a' },
							{ label: 'Option 2', id: 'b' },
							{ label: 'Option 3', id: 'c' },
							{ label: 'Option 4', id: 'd' }
						]}
					/>
				</div>
			</div>
			<div class="flex justify-end">
				<label for="toggle" class="label text-sm">
					Toggle
					<input id="toggle" type="checkbox" checked={true} class="toggle" />
				</label>
			</div>
		</div>

		<CustomConfigurationForm
			config={[{ key: 'Example Key', value: 'Example Value', type: 'text' }]}
		/>
	</div>
</Layout>

{#snippet themeConfiguration()}
	<div class="flex justify-between items-center gap-4 px-4 py-2">
		<p class="text-sm font-medium">Accent Color</p>
		{@render colorSelector({ id: 'primaryColor', label: 'Primary' })}
	</div>

	<div class="flex flex-col gap-2 px-4 pt-2 pb-4">
		<div class="flex items-center justify-between">
			<p class="text-sm font-medium">Surfaces</p>

			<div class="flex items-center gap-2 p-1.5 bg-base-200 rounded-md shadow-inner">
				<button
					class={twMerge(
						'btn btn-sm',
						selectedSurfaceMode === 'solid' ? 'btn-primary' : 'btn-secondary'
					)}
					onclick={() => (selectedSurfaceMode = 'solid')}>Custom</button
				>
				<button
					class={twMerge(
						'btn btn-sm',
						selectedSurfaceMode === 'tinted' ? 'btn-primary' : 'btn-secondary'
					)}
					onclick={() => (selectedSurfaceMode = 'tinted')}>Tinted</button
				>
			</div>
		</div>

		{#if selectedSurfaceMode === 'solid'}
			<!-- solid custom -->
			{#each selectedColorScheme === 'light' ? themeLightSurfaceFields : themeDarkSurfaceFields as field (field.id)}
				<div class="flex items-center justify-between">
					<p class="text-sm font-light">{field.label}</p>
					{@render colorSelector({ id: field.id, label: field.label })}
				</div>
			{/each}
		{:else}
			<!-- tinted -->
		{/if}
	</div>

	<div class="flex flex-col gap-2 p-4">
		<p class="text-sm font-medium">Buttons & Indicators</p>

		{#if selectedIndicatorMode === 'solid'}
			<!-- solid custom -->
			{#each selectedColorScheme === 'light' ? themeLightIndicatorFields : themeDarkIndicatorFields as field (field.id)}
				<div class="flex items-center justify-between">
					<p class="text-sm font-light">{field.label}</p>
					{@render colorSelector({ id: field.id, label: field.label })}
				</div>
			{/each}
		{:else}
			<!-- tinted -->
		{/if}
	</div>

	<div class="flex flex-col gap-2 p-4">
		<p class="text-sm font-medium">Text</p>
		{#each selectedColorScheme === 'light' ? textLightFields : textDarkFields as field (field.id)}
			<div class="flex items-center justify-between">
				<p class="text-sm font-light">{field.label}</p>
				{@render colorSelector({ id: field.id, label: field.label })}
			</div>
		{/each}
		<div class="flex items-center justify-between">
			<p class="text-sm font-light shrink-0">Font Family</p>
			<select class="select select-sm max-w-46">
				<option value="Poppins">Poppins</option>
				<option value="Inter">Inter</option>
				<option value="Roboto">Roboto</option>
				<option value="Helvetica">Helvetica</option>
				<option value="system-ui">System Default</option>
			</select>
		</div>
	</div>
{/snippet}

{#snippet logosConfiguration()}
	{#each standardIconFields as field (field.id)}
		<div class="flex flex-col gap-2 p-4 relative">
			<p class="text-sm font-medium">{field.label}</p>
			{@render iconSelector({ id: field.id, label: field.label }, 'standard')}
		</div>
	{/each}

	{#each selectedColorScheme === 'light' ? themeLightLogoFields : themeDarkLogoFields as field (field.id)}
		<div class="flex flex-col gap-2 p-4 relative">
			<p class="text-sm font-medium">{field.label}</p>
			{@render iconSelector(
				{ id: field.id, label: field.label },
				selectedColorScheme as 'light' | 'dark'
			)}
		</div>
	{/each}
{/snippet}

{#snippet colorSelector(field: { id: keyof AppPreferences['theme']; label: string })}
	<div class="flex items-center gap-2">
		<div class="relative">
			<div
				class="size-7 rounded-full border dark:border-white"
				style="background-color: {form.theme[field.id]}"
			></div>
			<input
				class="absolute top-0 left-0 size-7 cursor-pointer opacity-0"
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
		<input type="text" class="input input-sm" value={form.theme[field.id]} />
	</div>
{/snippet}

{#snippet iconSelector(
	field: { id: keyof AppPreferences['logos']; label: string },
	type: 'standard' | 'light' | 'dark'
)}
	<button
		class={twMerge(
			'group active:bg-surface1 dark:active:bg-surface3 flex flex-col items-center justify-center gap-2'
		)}
		onclick={() => {
			editImageUrl = form.logos[field.id].startsWith('/user/images/') ? '' : form.logos[field.id];
			selectedImageField = field.id;
			editUrlDialog?.open();
		}}
	>
		<img
			src={form.logos[field.id]}
			alt={field.label}
			class={twMerge(
				'shrink-0 object-contain',
				type === 'standard' ? 'size-18' : 'max-h-18 max-w-full'
			)}
		/>
		<Pencil
			class="text-base-content/40 transition-colors group-hover:text-base-content absolute top-4 right-4 size-4"
		/>
	</button>
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
	<title>Obot | Branding</title>
</svelte:head>
