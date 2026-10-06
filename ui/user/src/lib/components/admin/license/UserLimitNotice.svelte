<script lang="ts">
	import { m } from '$lib/i18n';
	import { version } from '$lib/stores';
	import { ShieldAlert } from '@lucide/svelte';

	let hasUserLimitViolation = $derived(
		version.current.licenseEntitlementViolations?.some(
			(violation) => violation.type === 'userLimit'
		) ?? false
	);

	let userLimitText = $derived(
		version.current.userLimit && version.current.userCount
			? `(${version.current.userCount}/${version.current.userLimit})`
			: ''
	);
</script>

<div class="notification-alert p-3 text-sm font-light flex items-center gap-2">
	<ShieldAlert class="size-6" />
	<div>
		{hasUserLimitViolation
			? m.platform_license_notice_notice_at_limit({ limit: userLimitText })
			: m.platform_license_notice_notice_almost_at_limit({ limit: userLimitText })}
		<a
			href="https://obot.ai/contact-us/"
			class="text-link"
			target="_blank"
			rel="noopener noreferrer"
		>
			{m.platform_license_notice_contact_us_upgrade()}</a
		>
	</div>
</div>
