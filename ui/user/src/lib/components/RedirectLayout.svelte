<script lang="ts">
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { ArrowUpRight, LayoutDashboard } from '@lucide/svelte';
	import type { Component } from 'svelte';
	import { fade } from 'svelte/transition';

	export interface RedirectDestination {
		readonly kicker: string;
		readonly title: string;
		readonly description: string;
		readonly href: `/${string}`;
		readonly icon: Component;
	}

	interface Props {
		title: string;
		destinations: readonly RedirectDestination[];
	}

	let { title, destinations }: Props = $props();

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout {title}>
	<div class="mx-auto flex w-full max-w-3xl flex-col gap-6 py-6 @container" in:fade={{ duration }}>
		<div class="flex flex-col gap-1">
			<p class="text-6xl font-semibold uppercase">404</p>
			<div class="flex flex-col gap-3">
				<h2 class="text-2xl font-semibold tracking-tight">{m.core_page_no_longer_available()}</h2>
				<p class="text-muted-content text-sm leading-relaxed font-light">
					{m.core_page_no_longer_available_alternatives()}
				</p>
			</div>
		</div>

		<ul class="grid gap-3 @2xl:grid-cols-2">
			{#each destinations as destination (destination.href)}
				{@const Icon = destination.icon}
				<li>
					<a
						href={resolve(destination.href)}
						class="paper group hover:border-primary/50 rounded-lg p-4 transition-colors"
					>
						<div class="flex items-start justify-between gap-4">
							<span
								class="bg-base-200 text-muted-content group-hover:text-primary dark:bg-base-300 flex size-10 items-center justify-center rounded-md transition-colors"
							>
								<Icon class="size-5" />
							</span>
							<ArrowUpRight
								class="text-muted-content group-hover:text-primary size-4 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5"
							/>
						</div>
						<div class="flex flex-col gap-1">
							<p class="text-muted-content text-xs font-medium tracking-[0.16em] uppercase">
								{destination.kicker}
							</p>
							<p class="text-lg font-semibold">{destination.title}</p>
							<p class="text-muted-content min-h-10 text-sm font-light">
								{destination.description}
							</p>
						</div>
					</a>
				</li>
			{/each}
		</ul>

		<a href={resolve('/dashboard')} class="btn btn-primary w-fit self-center">
			<LayoutDashboard class="size-4" />
			{m.core_go_to_dashboard()}
		</a>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
