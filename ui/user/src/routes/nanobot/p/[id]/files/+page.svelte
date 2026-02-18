<script lang="ts">
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { ChevronDown, ChevronRight, Folder, File, FolderOpen, Search } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import { getContext } from 'svelte';

	let resourceFiles = $derived(
		$nanobotChat?.chat?.resources
			? $nanobotChat.chat.resources.filter((r) => r.uri.startsWith('file:///'))
			: []
	);

	let filesContainer = $state<HTMLElement | undefined>(undefined);
	let query = $state('');

	const projectLayout = getContext<{
		chat: import('$lib/services/nanobot/chat/index.svelte').ChatService | null;
		handleFileOpen: (filename: string) => void;
		setThreadContentWidth: (w: number) => void;
	}>('nanobot-project-layout');

	type FileTreeNode =
		| { type: 'folder'; name: string; children: FileTreeNode[] }
		| { type: 'file'; name: string; uri: string };

	function onFileOpen(filename: string) {
		projectLayout?.handleFileOpen(filename);
	}

	function buildFileTreeSimple(files: { uri: string; name?: string }[]): FileTreeNode[] {
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
			parent.children.push({ type: 'file', name: fileName, uri: f.uri });
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

	let filteredFlatFileList = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return flatFileList;

		const flat = flatFileList;
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
	<div>
		<h2 class="text-2xl font-semibold">Files</h2>
		<p class="text-base-content/50 text-sm font-light">
			Manage & view files accessible to the project.
		</p>
	</div>
	<label class="input w-full">
		<Search class="size-6" />
		<input type="search" required placeholder="Search" bind:value={query} />
	</label>
	<ul class="mb-8 flex w-full flex-col">
		{#if filteredFlatFileList.length > 0}
			{#each filteredFlatFileList as { depth, path, node } (node.type === 'file' ? node.uri : `folder:${path}`)}
				<li class="w-full font-light">
					{#if node.type === 'folder'}
						<button
							class="btn btn-ghost flex w-full min-w-0 items-center justify-start gap-2 rounded-none py-6 text-left"
							style="padding-left: {depth * 0.75}rem;"
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
									<FolderOpen class="text-primary/80 size-4" />
								{:else}
									<Folder class="text-primary/80 size-4" />
								{/if}
							</div>
							<span class="min-w-0 truncate font-normal">{node.name}</span>
						</button>
					{:else}
						<button
							class={twMerge(
								'btn btn-ghost flex w-full min-w-0 items-center justify-start gap-2 rounded-none py-6 text-left'
							)}
							style="padding-left: {depth * 0.65}rem;"
							onclick={() => onFileOpen?.(node.uri)}
							aria-label={`Open file ${node.name}`}
						>
							<span class="w-[14px] shrink-0" aria-hidden="true"></span>
							<div class="bg-base-200 shrink-0 rounded-md p-1">
								<File class="size-4" />
							</div>
							<span class="min-w-0 truncate font-normal">{node.name}</span>
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
</div>
