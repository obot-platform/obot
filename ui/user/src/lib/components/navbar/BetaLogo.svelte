<script lang="ts">
	import { darkMode } from '$lib/stores';
	import appPreferences from '$lib/stores/appPreferences.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		chat?: boolean;
		enterprise?: boolean;
		class?: string;
	}
	let { chat, enterprise, class: klass }: Props = $props();

	let logos = $derived({
		dark: {
			chat: appPreferences.current.logos?.darkLogoChat,
			enterprise: appPreferences.current.logos?.darkLogoEnterprise,
			default: appPreferences.current.logos?.darkLogoDefault
		},
		light: {
			chat: appPreferences.current.logos?.logoChat,
			enterprise: appPreferences.current.logos?.logoEnterprise,
			default: appPreferences.current.logos?.logoDefault
		}
	});

	const logoSrc = $derived.by(() => {
		const theme = darkMode.isDark ? 'dark' : 'light';
		if (chat) {
			return logos[theme].chat;
		} else if (enterprise) {
			return logos[theme].enterprise;
		}
		return logos[theme].default;
	});

	const heightClass = $derived(chat ? 'h-[43px]' : 'h-12');
	const paddingClass = $derived(chat ? 'pl-[1px]' : '');
</script>

<div class={twMerge('flex flex-shrink-0', klass)}>
	<img src={logoSrc} class={twMerge(heightClass, paddingClass)} alt="Obot logo" />
</div>
