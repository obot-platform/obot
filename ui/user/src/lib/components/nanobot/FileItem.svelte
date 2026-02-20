<script lang="ts">
	import { FileIcon, FileImage } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		uri?: string;
		classes?: {
			icon?: string;
		};
		compact?: boolean;
		type?: 'button' | 'label';
		isSelected?: boolean;
		onClick?: () => void;
	}

	let { uri, classes, compact, type = 'label', isSelected, onClick }: Props = $props();

	let name = $derived(uri ? uri.split('/').pop() : undefined);
	let extension = $derived(name?.split('.').pop()?.toLowerCase());

	// Devicon class for popular languages/frameworks; generic files (txt, images, etc.) fall back to FileIcon/FileImage
	const EXTENSION_TO_DEVICON: Record<string, string> = {
		js: 'devicon-javascript-plain',
		mjs: 'devicon-javascript-plain',
		cjs: 'devicon-javascript-plain',
		ts: 'devicon-typescript-plain',
		mts: 'devicon-typescript-plain',
		cts: 'devicon-typescript-plain',
		py: 'devicon-python-plain',
		pyw: 'devicon-python-plain',
		html: 'devicon-html5-plain',
		htm: 'devicon-html5-plain',
		css: 'devicon-css3-plain',
		scss: 'devicon-sass-original',
		sass: 'devicon-sass-original',
		jsx: 'devicon-react-original',
		tsx: 'devicon-react-original',
		vue: 'devicon-vuejs-plain',
		svelte: 'devicon-svelte-plain',
		go: 'devicon-go-plain',
		rs: 'devicon-rust-original',
		java: 'devicon-java-plain',
		kt: 'devicon-kotlin-plain',
		kts: 'devicon-kotlin-plain',
		rb: 'devicon-ruby-plain',
		php: 'devicon-php-plain',
		swift: 'devicon-swift-plain',
		cs: 'devicon-csharp-plain',
		md: 'devicon-markdown-original',
		markdown: 'devicon-markdown-original',
		sh: 'devicon-bash-plain',
		bash: 'devicon-bash-plain',
		json: 'devicon-javascript-plain'
	};

	// Brand colors from devicon (for colored icon display)
	const EXTENSION_TO_COLOR: Record<string, string> = {
		js: '#f0db4f',
		mjs: '#f0db4f',
		cjs: '#f0db4f',
		ts: '#007acc',
		mts: '#007acc',
		cts: '#007acc',
		py: '#ffd845',
		pyw: '#ffd845',
		html: '#e54d26',
		htm: '#e54d26',
		css: '#3d8fc6',
		scss: '#cc6699',
		sass: '#cc6699',
		jsx: '#61dafb',
		tsx: '#61dafb',
		vue: '#41b883',
		svelte: '#ff3e00',
		go: '#00acd7',
		java: '#ea2d2e',
		kt: '#c711e1',
		kts: '#c711e1',
		rb: '#d91404',
		php: '#777bb3',
		swift: '#f05138',
		cs: '#68217a',
		sh: '#293138',
		bash: '#293138',
		json: '#f0db4f'
	};

	let iconClass = $derived.by(() => {
		if (!extension) return undefined;
		return EXTENSION_TO_DEVICON[extension];
	});

	let iconColor = $derived.by(() => {
		if (!extension) return undefined;
		return EXTENSION_TO_COLOR[extension];
	});

	const IMAGE_EXTENSIONS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'ico']);
</script>

{#if uri}
	{#if type === 'button'}
		{@render button()}
	{:else}
		{@render label()}
	{/if}
{/if}

{#snippet button()}
	{#if compact}
		<button
			class={twMerge(
				'btn btn-ghost btn-circle tooltip tooltip-left text-base-content/50 size-10 self-center',
				isSelected ? 'bg-base-200 hover:bg-base-200 hover:border-base-200' : 'hover:bg-base-300'
			)}
			in:fly={{ x: 100, duration: 150 }}
			onclick={() => onClick?.()}
			data-tip={isSelected ? name : `Open ${name}`}
		>
			{@render icon()}
		</button>
	{:else}
		<button
			class={twMerge(
				'rounded-selector flex items-center gap-2 border border-transparent px-4 py-2',
				isSelected ? 'bg-base-200' : 'hover:bg-base-300 '
			)}
			onclick={() => onClick?.()}
		>
			{@render icon()}
			<span>{name}</span>
		</button>
	{/if}
{/snippet}

{#snippet label()}
	{#if compact}
		{@render icon()}
	{:else}
		<div class="flex items-center gap-2">
			{@render icon()}
			<span>{name}</span>
		</div>
	{/if}
{/snippet}

{#snippet icon()}
	{#if iconClass}
		<i
			class={twMerge('text-base-content inline-block leading-none', iconClass, classes?.icon)}
			style={iconColor ? `color: ${iconColor}` : undefined}
			aria-hidden="true"
		></i>
	{:else}
		{@const isImage = extension && IMAGE_EXTENSIONS.has(extension)}
		<div class="relative w-fit">
			{#if isImage}
				<FileImage class={twMerge('size-5', classes?.icon)} />
			{:else}
				<FileIcon class={twMerge('size-5', classes?.icon)} />
			{/if}
			<span class="bg-base-100 absolute right-0.5 bottom-1 text-[6px] font-semibold">
				{extension}
			</span>
		</div>
	{/if}
{/snippet}
