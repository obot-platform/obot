<script lang="ts">
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import {
		ChevronDown,
		ChevronRight,
		Folder,
		FolderOpen,
		Search,
		LayoutList,
		FolderTree
	} from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import FileItem from '$lib/components/nanobot/FileItem.svelte';

	interface FileTimeResult {
		date?: Date;
		formatted: string;
	}

	function formatFileTime(timestamp: unknown): FileTimeResult {
		if (typeof timestamp !== 'string') return { date: undefined, formatted: '' };

		const value = timestamp.trim();
		if (!value) return { date: undefined, formatted: '' };

		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return { date: undefined, formatted: '' };

		let formatted = '';
		try {
			formatted = date
				.toLocaleString(undefined, {
					year: 'numeric',
					month: 'numeric',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit',
					hour12: false
				})
				.replace(/\//g, '-')
				.replace(/,/g, '');
		} catch {
			return { date: undefined, formatted: '' };
		}

		return { date, formatted };
	}

	let resourceFiles = $derived(
		$nanobotChat?.resources
			? $nanobotChat.resources.filter(
					(r) => r.uri.startsWith('file:///') && !r.uri.includes('workflows/')
				)
			: []
	);

	let filesContainer = $state<HTMLElement | undefined>(undefined);
	let query = $state('');
	let view = $state<'list' | 'tree'>('list');
	let showHiddenFiles = $state(false);

	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	type FileTreeNode =
		| { type: 'folder'; name: string; children: FileTreeNode[] }
		| {
				type: 'file';
				name: string;
				uri: string;
				size?: number;
				createdAt: FileTimeResult;
				modifiedAt: FileTimeResult;
		  };

	function onFileOpen(filename: string) {
		projectLayout?.handleFileOpen(filename);
	}

	function buildFileTreeSimple(
		files: { uri: string; name?: string; size?: number; _meta?: { [key: string]: unknown } }[]
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
			const fileName = segments.pop()!;
			const parent = ensurePath(segments);
			parent.children.push({
				type: 'file',
				name: fileName,
				uri: f.uri,
				size: f.size,
				createdAt: formatFileTime(f._meta?.createdAt),
				modifiedAt: formatFileTime(f._meta?.modifiedAt)
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

	function formatFileSize(bytes?: number): string {
		if (bytes == undefined) return '0 B';
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	let hasModifiedAt = $derived(
		resourceFiles.some((r) => !!formatFileTime(r._meta?.modifiedAt).date)
	);
	let hasCreatedAt = $derived(resourceFiles.some((r) => !!formatFileTime(r._meta?.createdAt).date));
	let columnCount = $derived(3 + (hasModifiedAt ? 1 : 0) + (hasCreatedAt ? 1 : 0));

	let fileTree = $derived(buildFileTreeSimple(resourceFiles));

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

	let flatFileList = $derived.by(() => {
		const open = folderOpen;
		return flattenTree(fileTree, 0, '', (path) => open[path] ?? true);
	});

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
</script>

<div class="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 md:px-8" bind:this={filesContainer}>
	<div class="mt-1 flex items-center justify-between gap-2">
		<label class="input w-full">
			<Search class="size-6" />
			<input type="search" required placeholder="Search files..." bind:value={query} />
		</label>
		<button
			class={twMerge(
				'btn btn-square tooltip tooltip-bottom',
				view === 'list' ? 'btn-soft btn-primary' : 'btn-ghost'
			)}
			onclick={() => (view = 'list')}
			data-tip="View as list"
		>
			<LayoutList class="size-5" />
		</button>
		<button
			class={twMerge(
				'btn btn-square tooltip tooltip-bottom',
				view === 'tree' ? 'btn-soft btn-primary' : 'btn-ghost'
			)}
			onclick={() => (view = 'tree')}
			data-tip="View as tree"
		>
			<FolderTree class="size-5" />
		</button>
	</div>
	<div class="flex items-center justify-between gap-4">
		<h2 class="text-2xl font-semibold">Files</h2>
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
		<table class="table">
			<thead>
				<tr>
					<th>Name</th>
					<th>Size</th>
					{#if hasModifiedAt}<th>Last Modified</th>{/if}
					{#if hasCreatedAt}<th>Created</th>{/if}
					<th>Location</th>
				</tr>
			</thead>
			<tbody>
				{#if filteredFlatFileList.length > 0}
					{#each filteredFlatFileList as { path, node } (node.type === 'file' ? node.uri : `folder:${path}`)}
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
								<td class="flex items-center gap-2">
									<FileItem uri={node.uri} classes={{ icon: 'size-4' }} />
								</td>
								<td>{formatFileSize(node.size)}</td>
								{#if hasModifiedAt}
									<td>{node.modifiedAt.formatted}</td>
								{/if}
								{#if hasCreatedAt}
									<td>{node.createdAt.formatted}</td>
								{/if}
								<td class="text-base-content/50 text-sm font-light italic">{node.uri}</td>
							</tr>
						{/if}
					{/each}
				{:else}
					<tr>
						<td
							colspan={columnCount}
							class="text-base-content/50 text-center text-sm font-light italic"
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
				<li class="text-base-content/50 flex items-start gap-2 px-4 font-light italic">
					<span>No files found.</span>
				</li>
			{/if}
		</ul>
	{/if}
</div>
