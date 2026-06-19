<script lang="ts">
	import AppNotificationBanner from '$lib/components/AppNotificationBanner.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import MarkdownInput from '$lib/components/MarkdownInput.svelte';
	import Select from '$lib/components/Select.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, type AppNotification, type BannerType } from '$lib/services';
	import { profile, appNotification as appNotificationStore } from '$lib/stores';
	import { defaultAppNotification } from '$lib/stores/appNotification.svelte';
	import { success } from '$lib/stores/success';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type BannerConfig = NonNullable<AppNotification['banner']>;
	type EditableAppNotification = AppNotification & {
		banner: BannerConfig;
	};

	function withBanner(notification: AppNotification): EditableAppNotification {
		const defaults: BannerConfig = defaultAppNotification.banner!;
		return {
			...notification,
			banner: {
				...defaults,
				...notification.banner
			}
		};
	}

	let { data } = $props();
	let appNotification = $state(untrack(() => withBanner(data.appNotification)));

	const duration = PAGE_TRANSITION_DURATION;
	let saving = $state(false);
	let bannerTextValidationError = $state<string | null>(null);
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	function hasOnlyAllowedMarkdown(text: string) {
		const disallowedPatterns = [
			/```/,
			/!\[[^\]]*]\([^)]*\)/,
			/<\/?[a-z][^>]*>/i,
			/^\s{0,3}#{1,6}\s/m,
			/^\s{0,3}>\s/m,
			/^\s{0,3}(?:[-*+]|\d+\.)\s/m,
			/^\s{0,3}(?:[-*_]\s*){3,}$/m,
			/\[[^\]]+]\[[^\]]*]/,
			/^\s*\|.+\|\s*$/m
		];
		if (disallowedPatterns.some((pattern) => pattern.test(text))) {
			return false;
		}

		const markdownLinks = [...text.matchAll(/\[([^\]]+)]\(([^)]+)\)/g)];
		for (const [, label, href] of markdownLinks) {
			if (!label.trim()) {
				return false;
			}

			try {
				const parsedUrl = new URL(href.trim());
				if (parsedUrl.protocol !== 'http:' && parsedUrl.protocol !== 'https:') {
					return false;
				}
			} catch {
				return false;
			}
		}

		const textWithoutLinks = text.replace(/\[([^\]]+)]\(([^)]+)\)/g, '$1');
		if (/[\\`]/.test(textWithoutLinks)) {
			return false;
		}

		return true;
	}

	function validate(banner: EditableAppNotification['banner']) {
		const text = banner.text?.trim() ?? '';
		if ((!text || !banner.type) && banner.enabled) {
			bannerTextValidationError = 'This field is required.';
			return false;
		}

		if (!hasOnlyAllowedMarkdown(text)) {
			bannerTextValidationError =
				'Only simple formatting and HTTP(S) text links are supported (bold, italic, strikethrough, and [text](url)).';
			return false;
		}

		bannerTextValidationError = null;
		return true;
	}

	async function handleSave() {
		if (!validate(appNotification.banner)) {
			return;
		}

		bannerTextValidationError = null;
		saving = true;
		try {
			const response = await AdminService.updateAppNotification(appNotification);
			appNotificationStore.initialize(response);
			success.add('App notifications updated successfully.');
		} catch (_err) {
			// errors are surfaced via the global HTTP error handling (errors store)
		} finally {
			saving = false;
		}
	}
</script>

<Layout title="App Notifications" classes={{ container: 'pb-0' }}>
	<div class="relative h-full w-full @container flex flex-col gap-4" transition:fade={{ duration }}>
		<div class="paper gap-0.5">
			<div>
				<p class="text-sm font-medium mb-2">Banner Preview</p>

				<div class="w-full mb-4">
					<AppNotificationBanner
						data={appNotification.banner}
						placeholder="[insert text to display here]"
					/>
				</div>

				<div class="divider mt-0"></div>

				<div class="flex flex-col gap-4">
					<div class="flex items-center gap-4">
						<label for="banner-type-selector" class="text-sm font-light">Type</label>
						<div class="w-full">
							<Select
								id="banner-type-selector"
								class="bg-base-200 dark:bg-base-100 dark:border-base-400 flex-1 border border-transparent shadow-none"
								selected={appNotification.banner.type}
								onSelect={(selected) => {
									appNotification.banner.type = selected.id as BannerType;
								}}
								disabled={isAdminReadonly}
								options={[
									{ id: 'info', label: 'Info' },
									{ id: 'warning', label: 'Warning' }
								]}
							/>
						</div>
					</div>

					<div class="flex flex-col gap-2">
						<p
							class={twMerge(
								'text-sm font-light inline-flex items-center gap-1',
								bannerTextValidationError && 'text-error'
							)}
						>
							Text <InfoTooltip text="Supports simple markdown formatting and text URL links." />
						</p>
						<MarkdownInput
							bind:value={appNotification.banner.text}
							class={twMerge(
								'min-h-[120px]',
								bannerTextValidationError && 'ring-2 ring-error border-error'
							)}
							classes={{ input: 'min-h-[120px]' }}
							placeholder="Add banner text. Supports simple formatting and [text](https://example.com) links."
							disabled={isAdminReadonly}
							disablePreview
						/>
						{#if bannerTextValidationError}
							<p class="text-xs font-light text-error">{bannerTextValidationError}</p>
						{/if}
					</div>
					<div class="divider my-0"></div>
					<label for="dismiss-banner-toggle" class="flex items-center justify-between">
						<div>
							<p class="text-sm font-light">Dismissible</p>
							<p class="text-xs font-light text-muted-content mb-2">
								The banner is {appNotification.banner.dismissible
									? 'dismissible'
									: 'not dismissible'}. {appNotification.banner.dismissible
									? 'The user can dismiss the banner and it will not appear again for their device.'
									: 'The banner will stay visible and cannot be hidden by the user.'}
							</p>
						</div>
						<input
							id="dismiss-banner-toggle"
							type="checkbox"
							class="toggle toggle-sm"
							bind:checked={appNotification.banner.dismissible}
							disabled={isAdminReadonly}
						/>
					</label>
					<label for="reset-dismissed-toggle" class="flex items-center justify-between">
						<div>
							<p class="text-sm font-light">Reset Dismissed</p>
							<p class="text-xs font-light text-muted-content mb-2">
								When enabled, the dismissed banner is shown again for users. They can dismiss the
								banner again after it is shown.
							</p>
						</div>
						<input
							id="reset-dismissed-toggle"
							type="checkbox"
							class="toggle toggle-sm"
							bind:checked={appNotification.banner.resetDismissed}
							disabled={isAdminReadonly || !appNotification.banner.dismissible}
						/>
					</label>

					<label for="enable-banner" class="w-full flex items-start justify-between gap-4">
						<div class="text-sm">
							<p>Enable Banner</p>
							<p class="text-xs font-light text-muted-content mb-2">
								Enabling the banner will display it at the top of the page across all pages (except
								agents, if enabled).
							</p>
						</div>
						<input
							type="checkbox"
							class="toggle toggle-sm"
							bind:checked={appNotification.banner.enabled}
							id="enable-banner"
							disabled={isAdminReadonly}
							onclick={() => {
								bannerTextValidationError = null;
							}}
						/>
					</label>
				</div>
			</div>
		</div>
		<div class="flex grow"></div>
		{#if !isAdminReadonly}
			<div
				class="bg-base-200 text-muted-content dark:bg-base-100 sticky bottom-0 left-0 z-50 flex w-full justify-end gap-2 py-4"
			>
				<div class="flex w-full justify-end gap-2">
					<button
						class="btn btn-secondary text-sm"
						onclick={() => {
							appNotification = withBanner(data.appNotification);
							bannerTextValidationError = null;
						}}
						disabled={saving}
					>
						Cancel
					</button>
					<button class="btn btn-primary text-sm" disabled={saving} onclick={handleSave}>
						Save
					</button>
				</div>
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | App Notifications</title>
</svelte:head>
