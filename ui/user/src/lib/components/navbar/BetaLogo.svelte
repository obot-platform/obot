<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import appPreferences from '$lib/stores/appPreferences.svelte';

	interface Props {
		chat?: boolean;
		enterprise?: boolean;
		workflow?: boolean;
		class?: string;
	}
	let { chat, enterprise, workflow, class: klass }: Props = $props();

	let logos = $derived({
		dark: {
			chat: appPreferences.current.logos?.darkLogoChat,
			enterprise: appPreferences.current.logos?.darkLogoEnterprise,
			default: appPreferences.current.logos?.darkLogoDefault,
			workflow: appPreferences.current.logos?.darkLogoWorkflow
		},
		light: {
			chat: appPreferences.current.logos?.logoChat,
			enterprise: appPreferences.current.logos?.logoEnterprise,
			default: appPreferences.current.logos?.logoDefault,
			workflow: appPreferences.current.logos?.logoWorkflow
		}
	});

	const logoSrc = $derived.by(() => {
		const theme = darkMode.isDark ? 'dark' : 'light';
		if (chat) {
			return logos[theme].chat;
		} else if (workflow) {
			return logos[theme].workflow;
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
