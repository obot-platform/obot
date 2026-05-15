<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import FileItem from '$lib/components/nanobot/FileItem.svelte';
	import { formatFileSize, formatFileTime } from '$lib/format';
	import type { FileTimeResult, ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { responsive, userDeviceSettings } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import type { TimeDisplayFormat } from '$lib/time';
	import { tryDecodeURIComponent } from '$lib/url';
	import {
		ChevronDown,
		ChevronRight,
		Folder,
		FolderOpen,
		Search,
		LayoutList,
		FolderTree,
		ChevronUp
	} from 'lucide-svelte';
	import { getContext } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let resourceFiles = $derived(
		$nanobotChat?.resources
			? $nanobotChat.resources.filter((r) => r.uri.startsWith('file:///'))
			: []
	);

	let filesContainer = $state<HTMLElement | undefined>(undefined);
	let query = $state('');
	let view = $state<'list' | 'tree'>('list');
	let showHiddenFiles = $state(false);
	let sorted = $state<{ property: string; order: 'asc' | 'desc' }>({
		property: 'name',
		order: 'desc'
	});
	let loading = $state(false);

	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	type FileTreeNode =
		| { type: 'folder'; name: string; children: FileTreeNode[] }
		| {
				type: 'file';
				name: string;
				uri: string;
				size?: number;
				lastModified?: FileTimeResult;
		  };

	function onFileOpen(filename: string) {
		projectLayout?.handleFileOpen(filename);
	}

	function buildFileTreeSimple(
		files: { uri: string; name?: string; size?: number; annotations?: { lastModified?: string } }[],
		displayFormat: TimeDisplayFormat
	): FileTreeNode[] {
		const root: Extract<FileTreeNode, { type: 'folder' }> = {
			type: 'folder',
			name: '',
			children: []
		};
		function ensurePath(segments: string[]): Extract<FileTreeNode, { type: 'folder' }> {
			let current = root;
			for (const seg of segments) {
				let found = current.children.find((c) => c.type === 'folder' && c.name === seg) as
					| Extract<FileTreeNode, { type: 'folder' }>
					| undefined;
				if (!found) {
					found = { type: 'folder', name: seg, children: [] };
					current.children.push(found);
				}
				current = found;
			}
			return current;
		}

		for (const f of files) {
			const path = f.uri.replace(/^file:\/\/+/, '');
			const segments = path.split('/').filter(Boolean);
			if (segments.length === 0) continue;
			const fileName = tryDecodeURIComponent(segments.pop()!);
			const parent = ensurePath(segments);
			parent.children.push({
				type: 'file',
				name: fileName,
				uri: f.uri,
				size: f.size,
				lastModified: formatFileTime(f.annotations?.lastModified, displayFormat)
			});
		}
		// Sort: folders first then files, both alphabetically
		function sortNodes(nodes: FileTreeNode[]): void {
			nodes.sort((a, b) => {
				if (a.type === 'folder' && b.type === 'file') return -1;
				if (a.type === 'file' && b.type === 'folder') return 1;
				return (a.name || '').localeCompare(b.name || '');
			});
			for (const n of nodes) {
				if (n.type === 'folder') sortNodes(n.children);
			}
		}
		sortNodes(root.children);
		return root.children;
	}

	function toNumberOrUndefined(value: unknown): number | undefined {
		if (typeof value === 'number') return Number.isFinite(value) ? value : undefined;
		if (typeof value === 'string') {
			const parsed = Number(value);
			return Number.isFinite(parsed) ? parsed : undefined;
		}
		return undefined;
	}

	let columnCount = $derived(responsive.isMobile ? 2 : 4);
	let columnHeaders = $derived([
		{ property: 'name', title: 'Name' },
		{ property: 'size', title: 'Size' },
		...(responsive.isMobile
			? []
			: [
					{ property: 'lastModified', title: 'Last Modified' },
					{ property: 'uri', title: 'Location' }
				])
	]);

	let fileTree = $derived(buildFileTreeSimple(resourceFiles, userDeviceSettings.timeFormat));

	type FlatNode = { depth: number; path: string; node: FileTreeNode };
	function flattenTree(
		nodes: FileTreeNode[],
		depth: number,
		pathPrefix: string,
		isOpen: (path: string) => boolean
	): FlatNode[] {
		const out: FlatNode[] = [];
		for (const n of nodes) {
			const path = pathPrefix ? `${pathPrefix}/${n.name}` : n.name;
			out.push({ depth, path, node: n });
			if (n.type === 'folder' && isOpen(path)) {
				out.push(...flattenTree(n.children, depth + 1, path, isOpen));
			}
		}
		return out;
	}

	let folderOpen = $state<Record<string, boolean>>({});
	function toggleFolder(path: string) {
		folderOpen[path] = !(folderOpen[path] ?? true);
		folderOpen = { ...folderOpen };
	}
	function isFolderOpen(path: string): boolean {
		return folderOpen[path] ?? true;
	}

	let flatFileList = $derived.by(() =>
		flattenTree(fileTree, 0, '', view === 'list' ? () => true : (path) => folderOpen[path] ?? true)
	);

	function isHiddenPath(path: string): boolean {
		return path.split('/').some((seg) => seg.startsWith('.'));
	}

	let filteredFlatFileList = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const flat = showHiddenFiles
			? flatFileList
			: flatFileList.filter(({ path }) => !isHiddenPath(path));
		if (!q) return flat;
		//eslint-disable-next-line svelte/prefer-svelte-reactivity
		const toInclude = new Set<string>();

		for (const { path, node } of flat) {
			const pathLower = path.toLowerCase();
			const nameLower = node.name.toLowerCase();
			const matches = pathLower.includes(q) || nameLower.includes(q);

			if (matches) {
				if (node.type === 'folder') {
					toInclude.add(path);
					for (const { path: p } of flat) {
						if (p.startsWith(path + '/')) toInclude.add(p);
					}
				} else {
					const segments = path.split('/');
					for (let i = 1; i <= segments.length; i++) {
						toInclude.add(segments.slice(0, i).join('/'));
					}
				}
			}
		}

		return flat.filter(({ path }) => toInclude.has(path));
	});

	const sortValueByProperty: Record<string, (item: FlatNode) => string | number | undefined> = {
		name: (item) => item.node.name ?? '',
		size: (item) => (item.node.type === 'file' ? toNumberOrUndefined(item.node.size) : undefined),
		lastModified: (item) =>
			item.node.type === 'file' ? item.node.lastModified?.date?.getTime() : undefined,
		uri: (item) => (item.node.type === 'file' ? item.node.uri : item.path)
	};

	const numericSortProperties = new Set(['size', 'lastModified']);

	let sortedFlatFileList = $derived.by(() => {
		const list = [...filteredFlatFileList];
		const { property, order } = sorted;
		const mult = order === 'asc' ? 1 : -1;
		const getVal = sortValueByProperty[property] ?? sortValueByProperty.name;

		list.sort((a, b) => {
			if (numericSortProperties.has(property)) {
				if (a.node.type === 'folder' && b.node.type === 'file') return -1;
				if (a.node.type === 'file' && b.node.type === 'folder') return 1;
			}
			const aVal = getVal(a);
			const bVal = getVal(b);
			if (numericSortProperties.has(property)) {
				const aNum = toNumberOrUndefined(aVal);
				const bNum = toNumberOrUndefined(bVal);
				if (aNum == undefined && bNum == undefined) return 0;
				if (aNum == undefined) return 1;
				if (bNum == undefined) return -1;
				return mult * (aNum - bNum);
			}
			const cmp = (aVal as string).localeCompare(bVal as string);
			return mult * cmp;
		});
		return list;
	});

	$effect(() => {
		const container = filesContainer;
		if (!container) return;

		const ro = new ResizeObserver((entries) => {
			const entry = entries[0];
			projectLayout.setThreadContentWidth(entry.contentRect.width);
		});
		ro.observe(container);
		projectLayout.setThreadContentWidth(container.getBoundingClientRect().width);
		return () => ro.disconnect();
	});

	afterNavigate(({ from }) => {
		if (!from?.url || !$nanobotChat?.api) return;
		loading = true;
		$nanobotChat.api
			.listResources()
			.then((resources) => {
				nanobotChat.update((data) => {
					if (data) {
						data.resources = resources;
					}
					return data;
				});
			})
			.finally(() => {
				loading = false;
			});
	});
</script>

<div
	class="mx-auto flex w-full max-w-full flex-col gap-6 overflow-x-hidden px-4 md:max-w-4xl md:px-8"
	bind:this={filesContainer}
>
	<div class="mt-1 flex items-center justify-between gap-2">
		<label class="input w-full">
			<Search class="size-6" />
			<input type="search" required placeholder="Search files..." bind:value={query} />
		</label>
		<button
			class={twMerge(
				'btn btn-square tooltip',
				responsive.isMobile ? 'tooltip-left' : 'tooltip-bottom',
				view === 'list' ? 'btn-soft btn-primary' : 'btn-ghost'
			)}
			onclick={() => (view = 'list')}
			data-tip="View as list"
		>
			<LayoutList class="size-5" />
		</button>
		<button
			class={twMerge(
				'btn btn-square tooltip',
				responsive.isMobile ? 'tooltip-left' : 'tooltip-bottom',
				view === 'tree' ? 'btn-soft btn-primary' : 'btn-ghost'
			)}
			onclick={() => (view = 'tree')}
			data-tip="View as tree"
		>
			<FolderTree class="size-5" />
		</button>
	</div>
	<div class="flex items-center justify-between gap-4">
		<div class="flex items-center gap-1">
			<h2 class="text-xl font-semibold md:text-2xl">Files</h2>
			{#if loading}
				<div class="loading loading-spinner text-primary loading-sm"></div>
			{/if}
		</div>
		<label class="label text-sm">
			<input
				type="checkbox"
				bind:checked={showHiddenFiles}
				class={twMerge(
					'checkbox checkbox-xs rounded-field',
					showHiddenFiles ? 'checkbox-primary' : ''
				)}
			/>
			Show hidden files
		</label>
	</div>
	{#if view === 'list'}
		<table class="table w-full table-fixed">
			<thead>
				<tr>
					{#each columnHeaders as header (header.property)}
						<th
							class="group min-w-0 {header.title === 'Size'
								? 'w-20'
								: header.title === 'Last Modified' || header.title === 'Created'
									? 'w-36'
									: ''}"
						>
							{header.title}
							<button
								class="btn btn-ghost tooltip btn-circle btn-xs opacity-0 transition-opacity group-hover:opacity-100"
								onclick={() => {
									if (sorted.property === header.property) {
										sorted.order = sorted.order === 'asc' ? 'desc' : 'asc';
									} else {
										sorted = { property: header.property, order: 'asc' };
									}
								}}
								data-tip={`Sort by ${header.title}: ${sorted.order === 'asc' || sorted.property !== header.property ? 'Descending' : 'Ascending'}`}
							>
								{#if (sorted.property === header.property && sorted.order === 'asc') || sorted.property !== header.property}
									<ChevronDown class="size-3" />
								{:else}
									<ChevronUp class="size-3" />
								{/if}
							</button>
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#if sortedFlatFileList.length > 0}
					{#each sortedFlatFileList as { path, node } (node.type === 'file' ? node.uri : `folder:${path}`)}
						{#if node.type === 'file'}
							<tr
								onclick={() => onFileOpen?.(node.uri)}
								class="hover:bg-base-200 cursor-pointer"
								role="button"
								tabindex="0"
								onkeydown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') {
										e.preventDefault();
										onFileOpen?.(node.uri);
									}
								}}
							>
								<td>
									<div class="flex items-center gap-2">
										<FileItem uri={node.uri} classes={{ icon: 'size-4' }} compact />
										<span class="min-w-0 truncate font-normal">
											{node.name}
										</span>
									</div>
								</td>
								<td
									><p class="truncate text-nowrap break-all">
										{formatFileSize(node.size ?? 0)}
									</p></td
								>
								{#if !responsive.isMobile}
									<td
										><p class="text-nowrap">
											{node.lastModified?.formatted || '-'}
										</p></td
									>
									<td>
										<div class="w-full min-w-0">
											<p
												class="text-muted-content w-full min-w-0 truncate text-sm font-light break-all italic"
											>
												{node.uri}
											</p>
										</div>
									</td>
								{/if}
							</tr>
						{/if}
					{/each}
				{:else}
					<tr>
						<td
							colspan={columnCount}
							class="text-muted-content text-center text-sm font-light italic"
						>
							<span>No files found.</span>
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	{:else}
		<ul class="mb-8 flex w-full flex-col">
			{#if filteredFlatFileList.length > 0}
				{#each filteredFlatFileList as { depth, path, node } (node.type === 'file' ? node.uri : `folder:${path}`)}
					<li class="w-full font-light">
						{#if node.type === 'folder'}
							<button
								class="btn btn-ghost flex w-full min-w-0 items-center justify-start gap-2 rounded-none py-6 text-left"
								style="padding-left: {depth * 1.65}rem;"
								onclick={() => toggleFolder(path)}
								aria-expanded={isFolderOpen(path)}
								aria-label={`Toggle folder ${node.name}`}
							>
								<span class="flex shrink-0 pl-2">
									{#if isFolderOpen(path)}
										<ChevronDown class="text-base-content/60 size-4" />
									{:else}
										<ChevronRight class="text-base-content/60 size-4" />
									{/if}
								</span>
								<div class="bg-base-200 shrink-0 rounded-md p-1">
									{#if isFolderOpen(path)}
										<FolderOpen class="text-primary/80 size-6" />
									{:else}
										<Folder class="text-primary/80 size-6" />
									{/if}
								</div>
								<span class="min-w-0 truncate font-normal">{node.name}</span>
							</button>
						{:else}
							<button
								class={twMerge(
									'btn btn-ghost flex w-full min-w-0 items-center justify-start gap-2 rounded-none py-6 text-left font-normal'
								)}
								style="padding-left: {depth * 1.6}rem;"
								onclick={() => onFileOpen?.(node.uri)}
								aria-label={`Open file ${node.name}`}
							>
								<span class="min-w-0 shrink-0" aria-hidden="true"></span>
								<FileItem uri={node.uri} classes={{ icon: 'size-4' }} />
							</button>
						{/if}
					</li>
				{/each}
			{:else}
				<li class="text-muted-content flex items-start gap-2 px-4 font-light italic">
					<span>No files found.</span>
				</li>
			{/if}
		</ul>
	{/if}
</div>

<svelte:head>
	<title>Obot | Files</title>
</svelte:head>
