<script lang="ts">
	import AppNotificationBanner from '$lib/components/AppNotificationBanner.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import MarkdownInput from '$lib/components/MarkdownInput.svelte';
	import Select from '$lib/components/Select.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
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

	let {
		appNotification: initialAppNotification,
		saving = false,
		dirty = $bindable(false)
	}: {
		appNotification: AppNotification;
		saving?: boolean;
		dirty?: boolean;
	} = $props();
	let persisted = $state(untrack(() => withBanner(initialAppNotification)));
	let appNotification = $state(untrack(() => withBanner(initialAppNotification)));

	const duration = PAGE_TRANSITION_DURATION;
	let bannerTextValidationError = $state<string | null>(null);
	function bannerKey(notification: EditableAppNotification) {
		const banner = notification.banner;
		return JSON.stringify({
			dismissible: banner.dismissible,
			enabled: banner.enabled,
			resetDismissed: banner.resetDismissed,
			text: banner.text ?? '',
			type: banner.type
		});
	}

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let isDirty = $derived(bannerKey(appNotification) !== bannerKey(persisted));

	$effect(() => {
		if (dirty !== isDirty) dirty = isDirty;
	});

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

	export function validate() {
		const banner = appNotification.banner;
		const text = banner.text?.trim() ?? '';
		if ((!text || !banner.type) && banner.enabled) {
			bannerTextValidationError = m.platform_field_required();
			return false;
		}

		if (!hasOnlyAllowedMarkdown(text)) {
			bannerTextValidationError = m.platform_settings_notifications_markdown_invalid();
			return false;
		}

		bannerTextValidationError = null;
		return true;
	}

	export function reset() {
		appNotification = withBanner(persisted);
		bannerTextValidationError = null;
	}

	export async function save() {
		if (!validate()) return false;

		try {
			const response = await AdminService.updateAppNotification(appNotification);
			const next = withBanner(response);
			persisted = next;
			appNotification = withBanner(response);
			appNotificationStore.initialize(response);
			success.add(m.platform_settings_notifications_updated());
			return true;
		} catch (_err) {
			// errors are surfaced via the global HTTP error handling (errors store)
			return false;
		}
	}
</script>

<div class="relative h-full w-full @container flex flex-col gap-2" in:fade={{ duration }}>
	<div class="paper gap-0.5">
		<div>
			<p class="text-sm font-medium mb-2">{m.platform_settings_notifications_banner_preview()}</p>

			<div class="w-full mb-4">
				<AppNotificationBanner
					data={appNotification.banner}
					placeholder={m.platform_settings_notifications_banner_placeholder()}
				/>
			</div>

			<div class="divider mt-0"></div>

			<div class="flex flex-col gap-4">
				<div class="flex items-center gap-4">
					<label for="banner-type-selector" class="text-sm font-light shrink-0"
						>{m.core_type()}</label
					>
					<div class="w-full">
						<Select
							id="banner-type-selector"
							class="bg-base-200 dark:bg-base-100 dark:border-base-400 flex-1 border border-transparent shadow-none"
							selected={appNotification.banner.type}
							onSelect={(selected) => {
								appNotification.banner.type = selected.id as BannerType;
							}}
							disabled={isAdminReadonly || saving}
							options={[
								{ id: 'info', label: m.platform_settings_notifications_info() },
								{ id: 'warning', label: m.platform_branding_warning() }
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
						{m.platform_branding_text()}
						<InfoTooltip text={m.platform_settings_notifications_text_help()} />
					</p>
					<MarkdownInput
						bind:value={appNotification.banner.text}
						class={twMerge(
							'min-h-30',
							bannerTextValidationError && 'ring-2 ring-error border-error'
						)}
						classes={{ input: 'min-h-[120px]' }}
						placeholder={m.platform_settings_notifications_text_placeholder()}
						disabled={isAdminReadonly || saving}
						disablePreview
					/>
					{#if bannerTextValidationError}
						<p class="text-xs font-light text-error">{bannerTextValidationError}</p>
					{/if}
				</div>
				<div class="divider my-0"></div>
				<label for="dismiss-banner-toggle" class="flex items-center justify-between">
					<div>
						<p class="text-sm font-light">{m.platform_settings_notifications_dismissible()}</p>
						<p class="text-xs font-light text-muted-content mb-2">
							{appNotification.banner.dismissible
								? m.platform_settings_notifications_dismissible_on()
								: m.platform_settings_notifications_dismissible_off()}
						</p>
					</div>
					<input
						id="dismiss-banner-toggle"
						type="checkbox"
						class="toggle toggle-sm"
						bind:checked={appNotification.banner.dismissible}
						disabled={isAdminReadonly || saving}
					/>
				</label>
				<label for="reset-dismissed-toggle" class="flex items-center justify-between">
					<div>
						<p class="text-sm font-light">{m.platform_settings_notifications_reset_dismissed()}</p>
						<p class="text-xs font-light text-muted-content mb-2">
							{m.platform_settings_notifications_reset_dismissed_description()}
						</p>
					</div>
					<input
						id="reset-dismissed-toggle"
						type="checkbox"
						class="toggle toggle-sm"
						bind:checked={appNotification.banner.resetDismissed}
						disabled={isAdminReadonly || saving || !appNotification.banner.dismissible}
					/>
				</label>

				<label for="enable-banner" class="w-full flex items-start justify-between gap-4">
					<div class="text-sm">
						<p>{m.platform_settings_notifications_enable_banner()}</p>
						<p class="text-xs font-light text-muted-content mb-2">
							{m.platform_settings_notifications_enable_banner_description()}
						</p>
					</div>
					<input
						type="checkbox"
						class="toggle toggle-sm"
						bind:checked={appNotification.banner.enabled}
						id="enable-banner"
						disabled={isAdminReadonly || saving}
						onclick={() => {
							bannerTextValidationError = null;
						}}
					/>
				</label>
			</div>
		</div>
	</div>
</div>
