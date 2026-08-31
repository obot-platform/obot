<script lang="ts">
	import { browser } from '$app/environment';
	import { invalidateAll } from '$app/navigation';
	import {
		computeTintedThemePatch,
		SHADE_TICK_NEUTRAL,
		type TintedSurfaceSnapshot
	} from '$lib/colors';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import UploadImage from '$lib/components/UploadImage.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type AppPreferences } from '$lib/services';
	import { darkMode, profile, responsive } from '$lib/stores';
	import appPreferences, {
		compileAppPreferences,
		FONT_FAMILY_PRESETS
	} from '$lib/stores/appPreferences.svelte';
	import { success } from '$lib/stores/success';
	import TintedSurfaceHueTintShadeControls from './TintedSurfaceHueTintShadeControls.svelte';
	import {
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
	import { PanelRightClose, PanelRightOpen, Pencil } from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let { initialAppPreferences }: { initialAppPreferences: AppPreferences } = $props();
	let form = $state<AppPreferences>(untrack(() => initialAppPreferences));
	let prevAppPreferences = $state<AppPreferences>(untrack(() => initialAppPreferences));
	let saving = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();
	let showConfigurationSidebar = $state(untrack(() => (responsive.isMobile ? false : true)));

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

	$effect(() => {
		if (!responsive.isMobile) {
			showConfigurationSidebar = true;
		}
	});

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
		surfacesSnapshotFromTheme(compileAppPreferences(untrack(() => initialAppPreferences)).theme)
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
			success.add('Your changes have been saved.');
		} catch (err) {
			console.error(err);
			// default behavior will show snackbar error
		} finally {
			saving = false;
		}
	}
</script>

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
		<div class="flex items-center justify-between px-4 py-2">
			{#if !responsive.isMobile}
				<h3 class="text-base font-semibold">Configuration</h3>
			{/if}
			<div
				class="flex items-center p-1.5 bg-base-200 dark:bg-base-300 rounded-4xl shadow-inner w-full md:w-auto"
			>
				<button
					class={twMerge(
						'btn btn-sm rounded-r-none! w-1/2 md:w-auto',
						selectedConfigurationMode === 'theme' ? 'btn-primary' : 'btn-secondary'
					)}
					onclick={() => {
						selectedConfigurationMode = 'theme';
					}}>Theme</button
				>
				<button
					class={twMerge(
						'btn btn-sm rounded-l-none! w-1/2 md:w-auto',
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
			<div class="flex items-center p-1.5 bg-base-200 dark:bg-base-300 rounded-4xl shadow-inner">
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
