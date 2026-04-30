<script lang="ts">
	import { browser } from '$app/environment';
	import { invalidateAll } from '$app/navigation';
	import {
		computeTintedThemePatch,
		SHADE_TICK_NEUTRAL,
		type TintedSurfaceSnapshot
	} from '$lib/colors';
	import Layout from '$lib/components/Layout.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import UploadImage from '$lib/components/UploadImage.svelte';
	import CustomConfigurationForm from '$lib/components/mcp/CustomConfigurationForm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type AppPreferences } from '$lib/services';
	import { darkMode, profile } from '$lib/stores';
	import appPreferences, {
		compileAppPreferences,
		FONT_FAMILY_PRESETS
	} from '$lib/stores/appPreferences.svelte';
	import { formatTimeAgo } from '$lib/time';
	import TintedSurfaceHueTintShadeControls from './TintedSurfaceHueTintShadeControls.svelte';
	import {
		MOCK_CONNECTOR_TABLE_DATA,
		type BrandingMockConnectorRow,
		themeLightSurfaceFields,
		themeDarkSurfaceFields,
		themeLightIndicatorFields,
		themeDarkIndicatorFields,
		textLightFields,
		textDarkFields,
		themeLightLogoFields,
		themeDarkLogoFields,
		standardIconFields
	} from './constants.js';
	import 'devicon/devicon.min.css';
	import { CircleAlert, HouseIcon, Info, Pencil, X } from 'lucide-svelte';
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

	let editUrlDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let uploadImage = $state<ReturnType<typeof UploadImage>>();
	let selectedImageField = $state<keyof AppPreferences['logos']>();
	let editImageUrl = $state<string>('');

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	let selectedColorScheme = $state(untrack(() => (darkMode.isDark ? 'dark' : 'light')));
	let initialColorScheme = $state(untrack(() => (darkMode.isDark ? 'dark' : 'light')));
	let selectedSurfaceMode = $state<'solid' | 'tinted'>('solid');
	let selectedConfigurationMode = $state<'theme' | 'logos'>('theme');
	let isPerThemeColorsEnabled = $state(false);

	function isLogoAssetUrl(s: string): boolean {
		const t = s.trim();
		if (!t) return false;
		return (
			t.startsWith('http://') ||
			t.startsWith('https://') ||
			t.startsWith('blob:') ||
			t.startsWith('data:') ||
			t.startsWith('/')
		);
	}

	const HEX_COLOR_RE = /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

	/**
	 * When Per-Theme Colors is off, editing mirrors to the paired light/dark key.
	 * Background & surface colors are omitted—they always stay independent per theme.
	 */
	const THEME_COLOR_COUNTERPART: Partial<
		Record<keyof AppPreferences['theme'], keyof AppPreferences['theme']>
	> = {
		primaryColor: 'darkPrimaryColor',
		darkPrimaryColor: 'primaryColor',
		secondaryColor: 'darkSecondaryColor',
		darkSecondaryColor: 'secondaryColor',
		successColor: 'darkSuccessColor',
		darkSuccessColor: 'successColor',
		warningColor: 'darkWarningColor',
		darkWarningColor: 'warningColor',
		errorColor: 'darkErrorColor',
		darkErrorColor: 'errorColor',
		onBackgroundColor: 'darkOnBackgroundColor',
		darkOnBackgroundColor: 'onBackgroundColor',
		onPrimaryColor: 'darkOnPrimaryColor',
		darkOnPrimaryColor: 'onPrimaryColor',
		onSuccessColor: 'darkOnSuccessColor',
		darkOnSuccessColor: 'onSuccessColor',
		onWarningColor: 'darkOnWarningColor',
		darkOnWarningColor: 'onWarningColor',
		onErrorColor: 'darkOnErrorColor',
		darkOnErrorColor: 'onErrorColor'
	};

	function isValidThemeColorString(raw: string): boolean {
		const value = raw.trim();
		if (!value) return false;
		if (value.startsWith('#')) {
			return HEX_COLOR_RE.test(value);
		}
		const lower = value.toLowerCase();
		if (lower.startsWith('hsl(') || lower.startsWith('hsla(')) {
			return typeof CSS !== 'undefined' && CSS.supports('color', value);
		}
		if (lower.startsWith('oklch(')) {
			return typeof CSS !== 'undefined' && CSS.supports('color', value);
		}
		return false;
	}

	function surfacesSnapshotFromTheme(theme: AppPreferences['theme']): TintedSurfaceSnapshot {
		return {
			light: {
				backgroundColor: theme.backgroundColor,
				surface1Color: theme.surface1Color,
				surface2Color: theme.surface2Color,
				surface3Color: theme.surface3Color
			},
			dark: {
				darkBackgroundColor: theme.darkBackgroundColor,
				darkSurface1Color: theme.darkSurface1Color,
				darkSurface2Color: theme.darkSurface2Color,
				darkSurface3Color: theme.darkSurface3Color
			}
		};
	}

	/** Surfaces under Custom (solid); unchanged while editing Tinted so switching modes restores each side. */
	let customSurfaces = $state<TintedSurfaceSnapshot>(
		surfacesSnapshotFromTheme(compileAppPreferences(untrack(() => data.appPreferences)).theme)
	);

	let tintedHueLight = $state(0);
	let tintedHueDark = $state(0);
	let tintedTintLight = $state(0);
	let tintedTintDark = $state(0);
	let tintedShadeLight = $state(SHADE_TICK_NEUTRAL);
	let tintedShadeDark = $state(SHADE_TICK_NEUTRAL);
	let tintedSurfaceSnapshot = $state<TintedSurfaceSnapshot | null>(null);

	function themeSurfacePatch(snapshot: TintedSurfaceSnapshot): Partial<AppPreferences['theme']> {
		return {
			backgroundColor: snapshot.light.backgroundColor,
			surface1Color: snapshot.light.surface1Color,
			surface2Color: snapshot.light.surface2Color,
			surface3Color: snapshot.light.surface3Color,
			darkBackgroundColor: snapshot.dark.darkBackgroundColor,
			darkSurface1Color: snapshot.dark.darkSurface1Color,
			darkSurface2Color: snapshot.dark.darkSurface2Color,
			darkSurface3Color: snapshot.dark.darkSurface3Color
		};
	}

	function patchCustomSurface(
		snapshot: TintedSurfaceSnapshot,
		key: keyof AppPreferences['theme'],
		value: string
	): TintedSurfaceSnapshot {
		switch (key) {
			case 'backgroundColor':
			case 'surface1Color':
			case 'surface2Color':
			case 'surface3Color':
				return {
					...snapshot,
					light: { ...snapshot.light, [key]: value }
				};
			case 'darkBackgroundColor':
			case 'darkSurface1Color':
			case 'darkSurface2Color':
			case 'darkSurface3Color':
				return {
					...snapshot,
					dark: { ...snapshot.dark, [key]: value }
				};
			default:
				return snapshot;
		}
	}

	function applyCustomSurfacesToForm(): AppPreferences {
		return {
			...form,
			theme: { ...form.theme, ...themeSurfacePatch(customSurfaces) }
		};
	}

	/** Tinted bases always use the stock default surface ladder from compile defaults—not Custom colors. */
	function captureTintedSurfaceSnapshot() {
		const defaultTheme = compileAppPreferences().theme;
		tintedSurfaceSnapshot = surfacesSnapshotFromTheme(defaultTheme);
	}

	function resetTintedControls() {
		tintedHueLight = 0;
		tintedHueDark = 0;
		tintedTintLight = 0;
		tintedTintDark = 0;
		tintedShadeLight = SHADE_TICK_NEUTRAL;
		tintedShadeDark = SHADE_TICK_NEUTRAL;
	}

	$effect(() => {
		if (!browser) return;
		if (selectedSurfaceMode !== 'tinted') return;
		const snap = tintedSurfaceSnapshot;
		if (!snap) return;
		const patch = computeTintedThemePatch(
			snap,
			{
				hueDeg: tintedHueLight,
				tint0to100: tintedTintLight,
				shadeTick: tintedShadeLight
			},
			{
				hueDeg: tintedHueDark,
				tint0to100: tintedTintDark,
				shadeTick: tintedShadeDark
			}
		);
		untrack(() => {
			form = { ...form, theme: { ...form.theme, ...patch } };
			appPreferences.setThemeColors(form.theme);
		});
	});

	onDestroy(() => {
		if (browser) {
			darkMode.setDark(initialColorScheme === 'dark');
			appPreferences.setThemeColors(appPreferences.current.theme);
		}
	});

	function isSurfaceThemeKey(id: keyof AppPreferences['theme']): boolean {
		return (
			id === 'backgroundColor' ||
			id === 'surface1Color' ||
			id === 'surface2Color' ||
			id === 'surface3Color' ||
			id === 'darkBackgroundColor' ||
			id === 'darkSurface1Color' ||
			id === 'darkSurface2Color' ||
			id === 'darkSurface3Color'
		);
	}

	async function handleSave() {
		if (timeout) {
			clearTimeout(timeout);
		}
		saving = true;
		try {
			let saveForm = form;
			if (selectedSurfaceMode === 'solid') {
				saveForm = applyCustomSurfacesToForm();
				form = saveForm;
			}
			appPreferences.current = saveForm;
			appPreferences.setThemeColors(saveForm.theme);
			await AdminService.updateAppPreferences(saveForm);
			await invalidateAll();
			prevAppPreferences = saveForm;
			customSurfaces = surfacesSnapshotFromTheme(saveForm.theme);
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
</script>

<Layout title="Branding" classes={{ container: 'pb-0' }}>
	{#snippet rightSidebar()}
		<div
			class="bg-base-100 dark:bg-base-200 border-base-300 h-dvh w-sm min-w-sm overflow-y-auto border-l flex flex-col"
		>
			<div class="flex flex-col divide-y divide-base-300">
				<div class="flex items-center justify-between px-4 py-2">
					<h3 class="text-base font-semibold">Configuration</h3>
					<div
						class="flex items-center p-1.5 bg-base-200 dark:bg-base-300 rounded-4xl shadow-inner"
					>
						<button
							class={twMerge(
								'btn btn-sm rounded-r-none!',
								selectedConfigurationMode === 'theme' ? 'btn-primary' : 'btn-secondary'
							)}
							onclick={() => {
								selectedConfigurationMode = 'theme';
							}}>Theme</button
						>
						<button
							class={twMerge(
								'btn btn-sm rounded-l-none!',
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
					<div
						class="flex items-center p-1.5 bg-base-200 dark:bg-base-300 rounded-4xl shadow-inner"
					>
						<button
							class={twMerge(
								'btn btn-sm rounded-r-none!',
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
								'btn btn-sm rounded-l-none!',
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
							class="btn btn-sm btn-secondary font-medium"
							onclick={() => {
								form = compileAppPreferences();
								customSurfaces = surfacesSnapshotFromTheme(form.theme);
								appPreferences.current = compileAppPreferences(form);
								appPreferences.setThemeColors(form.theme);
								if (selectedSurfaceMode === 'tinted') {
									captureTintedSurfaceSnapshot();
								}
								resetTintedControls();
								editUrlDialog?.close();
							}}
						>
							Restore Default
						</button>
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
									form = prevAppPreferences;
									customSurfaces = surfacesSnapshotFromTheme(prevAppPreferences.theme);
									form = applyCustomSurfacesToForm();
									appPreferences.current = compileAppPreferences(prevAppPreferences);
									appPreferences.setThemeColors(form.theme);
									resetTintedControls();
									selectedSurfaceMode = 'solid';
									editUrlDialog?.close();
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
				class="text-muted-content flex min-h-10 items-center px-4 text-sm font-extralight"
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
					<p class="text-md text-muted-content mb-1 text-center font-light">
						Log in or create your account to continue
					</p>

					<div
						class="dark:border-base-400 dark:bg-base-200 bg-base-100 flex w-sm flex-col gap-4 rounded-xl border border-transparent p-4 shadow-sm"
					>
						<button class="btn btn-secondary w-full">
							<img
								class="h-6 w-6 rounded-full bg-base-100 p-1 dark:bg-gray-600"
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
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
					<button class="btn btn-circle btn-primary"><HouseIcon /></button>
					<button class="btn btn-primary">Confirm</button>
				</div>
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
					<button class="btn btn-circle btn-secondary"><HouseIcon /></button>
					<button class="btn btn-secondary">Confirm</button>
				</div>
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
					<button class="btn btn-circle btn-success"><HouseIcon /></button>
					<button class="btn btn-success">Confirm</button>
				</div>
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
					<button class="btn btn-circle btn-warning"><HouseIcon /></button>
					<button class="btn btn-warning">Confirm</button>
				</div>
				<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
					<button class="btn btn-circle btn-error"><HouseIcon /></button>
					<button class="btn btn-error">Confirm</button>
				</div>
			</div>
		</div>
		<div class="w-full mt-8">
			<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
				<div class="flex">
					<button class="page-tab w-1/2 max-w-1/2 page-tab-active"> Servers </button>
					<button class="page-tab w-1/2 max-w-1/2"> Users </button>
				</div>
				<Table
					data={MOCK_CONNECTOR_TABLE_DATA}
					fields={['name', 'status', 'created']}
					filterable={['name', 'status']}
					sortable={['name', 'created', 'status']}
				>
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
						class="bg-base-200 dark:bg-base-100 dark:border-base-400 border border-transparent shadow-inner"
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
			config={[
				{
					key: 'Example Key',
					value: 'Example Value',
					description: 'Example Description',
					name: 'Example Name',
					required: true,
					sensitive: false
				}
			]}
		/>
	</div>
</Layout>

{#snippet themeConfiguration()}
	<div class="flex flex-col gap-2 px-4 pt-2 pb-4">
		<div class="flex items-center justify-between">
			<p class="text-sm font-medium">Surfaces</p>

			<div class="flex items-center p-1.5 bg-base-200 dark:bg-base-300 rounded-4xl shadow-inner">
				<button
					class={twMerge(
						'btn btn-sm rounded-r-none!',
						selectedSurfaceMode === 'solid' ? 'btn-primary' : 'btn-secondary'
					)}
					onclick={() => {
						selectedSurfaceMode = 'solid';
						form = applyCustomSurfacesToForm();
						appPreferences.setThemeColors(form.theme);
					}}>Custom</button
				>
				<button
					class={twMerge(
						'btn btn-sm rounded-l-none!',
						selectedSurfaceMode === 'tinted' ? 'btn-primary' : 'btn-secondary'
					)}
					onclick={() => {
						selectedSurfaceMode = 'tinted';
						captureTintedSurfaceSnapshot();
					}}>Tinted</button
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
			<p class="text-xs font-light text-muted-content">
				Tinted always starts from the built-in default surface ramp; hue, tint, and shade adjust
				that. Custom picker colors stay on Custom only.
			</p>
			{#if selectedColorScheme === 'light'}
				<TintedSurfaceHueTintShadeControls
					bind:hue={tintedHueLight}
					bind:tint={tintedTintLight}
					bind:shade={tintedShadeLight}
					hueAriaLabel="Light mode surface hue"
				/>
			{:else}
				<TintedSurfaceHueTintShadeControls
					bind:hue={tintedHueDark}
					bind:tint={tintedTintDark}
					bind:shade={tintedShadeDark}
					hueAriaLabel="Dark mode surface hue"
				/>
			{/if}
			<p class="text-xs font-light text-muted-content pl-22 mt-1">
				{SHADE_TICK_NEUTRAL} is neutral for shade. Light and dark each have their own hue, tint, and shade—adjusting
				one scheme does not change the other’s sliders.
			</p>
		{/if}
	</div>

	<div class="flex flex-col gap-2 p-4">
		<div class="flex justify-between items-center gap-4 pb-1">
			<p class="text-sm font-medium">Per-Theme Colors</p>
			<input type="checkbox" class="toggle toggle-sm" bind:checked={isPerThemeColorsEnabled} />
		</div>

		<p class="text-xs font-light text-muted-content">
			Per-Theme Colors is <span class="font-semibold text-base-content"
				>{isPerThemeColorsEnabled ? 'enabled' : 'disabled'}
			</span>.
			{#if isPerThemeColorsEnabled}
				Accent color, button & indicator colors, and text options are separately customizable for
				light &amp; dark modes.
			{:else}
				Accent color, button & indicator colors, and text options stay aligned between light & dark
				i.e. changing one updates the paired value for the other mode.
			{/if}
		</p>
	</div>

	<div class="flex justify-between items-center gap-4 px-4 py-2">
		<p class="text-sm font-medium">Accent Color</p>
		{@render colorSelector({
			id: selectedColorScheme === 'light' ? 'primaryColor' : 'darkPrimaryColor',
			label: 'Primary'
		})}
	</div>

	<div class="flex flex-col gap-2 p-4">
		<p class="text-sm font-medium">Buttons & Indicators</p>
		{#each selectedColorScheme === 'light' ? themeLightIndicatorFields : themeDarkIndicatorFields as field (field.id)}
			<div class="flex items-center justify-between">
				<p class="text-sm font-light">{field.label}</p>
				{@render colorSelector({ id: field.id, label: field.label })}
			</div>
		{/each}
	</div>

	<div class="flex flex-col gap-2 p-4">
		<p class="text-sm font-medium">Text</p>
		{#each selectedColorScheme === 'light' ? textLightFields : textDarkFields as field (field.id)}
			<div class="flex items-center justify-between">
				<p class="text-sm font-light">{field.label}</p>
				{@render colorSelector({ id: field.id, label: field.label })}
			</div>
		{/each}
		<div class="flex items-center justify-between gap-2">
			<p class="text-sm font-light shrink-0">Font Family</p>
			<select
				class="select select-sm max-w-46 min-w-0"
				value={form.theme.fontFamily}
				onchange={(e) => {
					const fontFamily = e.currentTarget.value;
					const newForm = { ...form, theme: { ...form.theme, fontFamily } };
					form = newForm;
					appPreferences.setThemeColors(newForm.theme);
				}}
			>
				{#each FONT_FAMILY_PRESETS as preset (preset.value)}
					<option value={preset.value}>{preset.label}</option>
				{/each}
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
		<div class="relative group">
			<div
				class="size-7 rounded-full border dark:border-white group-focus-within:ring-2 group-focus-within:ring-base-content"
				style="background-color: {form.theme[field.id]}"
			></div>
			<input
				class="absolute top-0 left-0 size-7 cursor-pointer opacity-0 focus:outline-none"
				type="color"
				id={field.id}
				value={form.theme[field.id].startsWith('#') ? form.theme[field.id] : '#ffffff'}
				oninput={(e) => {
					if (!e.currentTarget.value.startsWith('#')) {
						return;
					}
					const value = e.currentTarget.value;
					const counterpart = !isPerThemeColorsEnabled
						? THEME_COLOR_COUNTERPART[field.id]
						: undefined;
					let nextTheme = { ...form.theme, [field.id]: value };
					if (counterpart) {
						nextTheme = { ...nextTheme, [counterpart]: value };
					}
					const newForm = { ...form, theme: nextTheme };
					appPreferences.setThemeColors(newForm.theme);
					form = newForm;
					if (selectedSurfaceMode === 'solid' && isSurfaceThemeKey(field.id)) {
						customSurfaces = patchCustomSurface(customSurfaces, field.id, value);
					}
				}}
			/>
		</div>
		<input
			type="text"
			class="input input-sm"
			value={form.theme[field.id]}
			oninput={(e) => {
				const value = e.currentTarget.value.trim();
				if (!isValidThemeColorString(value)) {
					return;
				}
				const counterpart = !isPerThemeColorsEnabled
					? THEME_COLOR_COUNTERPART[field.id]
					: undefined;
				let nextTheme = { ...form.theme, [field.id]: value };
				if (counterpart) {
					nextTheme = { ...nextTheme, [counterpart]: value };
				}
				const newForm = { ...form, theme: nextTheme };
				appPreferences.setThemeColors(newForm.theme);
				form = newForm;
				if (selectedSurfaceMode === 'solid' && isSurfaceThemeKey(field.id)) {
					customSurfaces = patchCustomSurface(customSurfaces, field.id, value);
				}
			}}
		/>
	</div>
{/snippet}

{#snippet iconSelector(
	field: { id: keyof AppPreferences['logos']; label: string },
	type: 'standard' | 'light' | 'dark'
)}
	<button
		class={twMerge('group flex flex-col items-center justify-center gap-2')}
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
				'shrink-0 object-contain transition-transform group-hover:scale-115 group-active:brightness-135',
				type === 'standard' ? 'size-18' : 'max-h-18 max-w-full'
			)}
		/>
		<Pencil
			class="text-muted-content transition-colors group-hover:text-base-content absolute top-4 right-4 size-4"
		/>
	</button>
{/snippet}

<ResponsiveDialog
	bind:this={editUrlDialog}
	title={editImageUrl ? 'Edit Image URL' : 'Add Image URL'}
	onClose={() => {
		editImageUrl = '';
		selectedImageField = undefined;
		uploadImage?.clearPreview();
	}}
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
	<div class="flex flex-wrap justify-end gap-2">
		<button
			type="button"
			class="btn btn-secondary mt-4 w-full md:w-fit"
			onclick={() => editUrlDialog?.close()}>Cancel</button
		>
		<button
			type="button"
			class="btn btn-primary mt-4 w-full md:w-fit"
			onclick={() => {
				if (!selectedImageField) return;
				const candidate = editImageUrl.trim();
				const resolvedUrl =
					candidate !== '' && isLogoAssetUrl(candidate)
						? candidate
						: form.logos[selectedImageField];
				const newForm = {
					...form,
					logos: { ...form.logos, [selectedImageField]: resolvedUrl }
				};
				form = newForm;
				appPreferences.current = compileAppPreferences(newForm);
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
