<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { AdminService, type ProjectThread, type Project, type OrgUser } from '$lib/services';
	import { Eye, LoaderCircle, MessageCircle, Funnel } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { replaceState } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';
	import Search from '$lib/components/Search.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { page } from '$app/state';
	import FiltersDrawer from '$lib/components/admin/filters-drawer/FiltersDrawer.svelte';
	import { getUserDisplayName } from '$lib/utils';
	import type { FilterOptionsEndpoint } from '$lib/components/admin/filters-drawer/types';
	import { debounce } from 'es-toolkit';
	import { Group } from '$lib/services/admin/types';
	import { openUrl } from '$lib/utils';
	import { profile } from '$lib/stores';

	type SupportedFilter = 'username' | 'email' | 'project' | 'query';

	let threads = $state<ProjectThread[]>([]);
	let filteredThreads = $state<ProjectThread[]>([]);
	let projects = $state<Project[]>([]);
	let users = $state<OrgUser[]>([]);
	let projectMap = $derived(new Map(projects.map((p) => [p.id, p.name])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));

	let rightSidebar = $state<HTMLDialogElement>();

	let showFilters = $state(false);

	const supportedFilters: Exclude<SupportedFilter, 'query'>[] = ['username', 'email', 'project'];

	const searchParamsAsArray: [SupportedFilter, string | undefined | null][] = $derived(
		supportedFilters.map((d) => {
			const hasSearchParam = page.url.searchParams.has(d);

			const value = page.url.searchParams.get(d);
			const isValueDefined = isSafe(value);

			return [
				d,
				isValueDefined
					? // Value is defined then decode and use it
						decodeURIComponent(value)
					: hasSearchParam
						? // Value is not defined but has a search param then override with empty string
							''
						: // No search params return default value if exist otherwise return undefined
							null
			];
		})
	);

	// Extract search supported params from the URL and convert them to AuditLogURLFilters
	// This is used to filter the audit logs based on the URL parameters
	const searchParamFilters = $derived.by<Record<SupportedFilter, string | undefined | null>>(() => {
		return searchParamsAsArray.reduce(
			(acc, [key, value]) => {
				acc[key!] = value;
				return acc;
			},
			{} as Record<SupportedFilter, string | undefined | null>
		);
	});

	let query = $state(page.url.searchParams.get('query') || '');

	// Base filters with time filters and query and pagination
	const pageFilters = $derived({
		...searchParamFilters,
		query
	});

	const options = $derived.by(() => {
		const usernames = new Set<string>();
		const emails = new Set<string>();
		const projects = new Set<string>();

		threads.forEach((thread) => {
			const user = userMap.get(thread.userID || '');

			if (user?.displayName) {
				usernames.add(user.displayName);
			}

			if (user?.email) {
				emails.add(user.email);
			}

			if (thread.projectID) {
				projects.add(thread.projectID);
			}
		});

		return {
			username: { options: Array.from(usernames).sort() },
			email: { options: Array.from(emails).sort() },
			project: { options: Array.from(projects).sort() }
		};
	});

	let loading = $state(true);
	let tableData = $derived(
		filteredThreads.map((thread) => {
			return {
				...thread,
				projectName: projectMap.get(thread.projectID || '') || thread.projectID,
				userName: userMap.get(thread.userID || '')?.displayName || '-',
				userEmail: userMap.get(thread.userID || '')?.email || '-'
			};
		})
	);

	const updateQuery = debounce((value: string) => {
		query = value;

		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}

		// Update the query search param without cause app to react
		// Prevent losing focus from the input
		// history.replaceState(null, '', page.url);
		replaceState(page.url, { query });
	}, 100);

	onMount(() => {
		loadThreads();
	});

	$effect(() => {
		filteredThreads = applyFilters(threads, pageFilters);
	});

	function getFilterDisplayLabel(key: string) {
		if (key === 'email') return 'Email';
		if (key === 'project') return 'Project';
		if (key === 'username') return 'User Name';

		return key.replace(/_(\w)/g, ' $1');
	}

	function isSafe<T = unknown>(value: T) {
		return value !== undefined && value !== null;
	}

	async function loadThreads() {
		loading = true;
		try {
			// Load threads, projects, and users in parallel with individual error handling
			const threadsPromise = AdminService.listThreads().catch((err) => {
				console.error('Failed to load threads:', err);
				return [];
			});

			const projectsPromise = AdminService.listProjects().catch((err) => {
				console.error('Failed to load projects:', err);
				return [];
			});

			const usersPromise = AdminService.listUsers().catch((err) => {
				console.error('Failed to load users:', err);
				return [];
			});

			// Add timeout to prevent hanging
			const timeoutPromise = new Promise<never>((_, reject) => {
				setTimeout(() => reject(new Error('Request timeout')), 10000);
			});

			const [threadsData, projectsData, usersData] = await Promise.race([
				Promise.all([threadsPromise, projectsPromise, usersPromise]),
				timeoutPromise
			]);

			// threads = threadsData;
			projects = projectsData;
			users = usersData;
			// Filter out task & task runs
			threads = threadsData.filter((thread) => !thread.taskID && !thread.taskRunID);
		} catch (error) {
			console.error('Failed to load data:', error);
			// Set empty arrays as fallback
			threads = [];
			projects = [];
			users = [];
			// filteredThreads = [];
		} finally {
			loading = false;
		}
	}

	function applyFilters(
		data: ProjectThread[] = threads,
		filters: typeof pageFilters = pageFilters
	) {
		// First filter to only include project threads and exclude system tasks
		let filtered = data.filter((thread) => !thread.project && !thread.systemTask);

		type FilterFunction = [string | undefined | null, (array: ProjectThread[]) => ProjectThread[]];

		const queryFilterFunction = (array: ProjectThread[]) => {
			const lowercasedQuery = query.toLowerCase();
			return array.filter((thread) => {
				const user = userMap.get(thread.userID || '');
				return (
					thread.name?.toLowerCase().includes(lowercasedQuery) ||
					thread.id.toLowerCase().includes(lowercasedQuery) ||
					thread.userID?.toLowerCase().includes(lowercasedQuery) ||
					thread.projectID?.toLowerCase().includes(lowercasedQuery) ||
					user?.displayName?.toLowerCase().includes(lowercasedQuery) ||
					user?.email?.toLowerCase().includes(lowercasedQuery) ||
					projectMap
						.get(thread.projectID || '')
						?.toLowerCase()
						.includes(lowercasedQuery)
				);
			});
		};

		const usernameFilterFunction = (array: ProjectThread[]) => {
			return array.filter((thread) => {
				const user = userMap.get(thread.userID || '');
				return (filters?.username ?? '')
					?.toLowerCase()
					.includes(user?.displayName?.toLowerCase() || '');
			});
		};

		const emailFilterFunction = (array: ProjectThread[]) => {
			return array.filter((thread) => {
				const user = userMap.get(thread.userID || '');
				return (filters?.email ?? '')?.toLowerCase().includes(user?.email?.toLowerCase() || '');
			});
		};

		const projectFilterFunction = (array: ProjectThread[]) => {
			return array.filter((thread) => {
				return (filters?.project ?? '')
					?.toLowerCase()
					.includes(thread.projectID?.toLowerCase() || '');
			});
		};

		const filterFns: FilterFunction[] = [
			[pageFilters.query, queryFilterFunction],
			[filters.username, usernameFilterFunction],
			[filters.email, emailFilterFunction],
			[filters.project, projectFilterFunction]
		].filter((d) => !!d[0]) as FilterFunction[];

		// sort by most recent
		return filterFns
			.reduce((acc, val) => val[1](acc), filtered)
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime());
	}

	function formatThreadName(thread: ProjectThread) {
		return thread.name || 'Unnamed Thread';
	}

	function handleRightSidebarClose() {
		rightSidebar?.close();
		setTimeout(() => {
			showFilters = false;
		}, 300);
	}

	async function optionsEndpoint(filterId: SupportedFilter) {
		switch (filterId) {
			case 'username':
				return options.username;
			case 'email':
				return options.email;
			case 'project':
				return options.project;
			default:
				return [];
		}
	}
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex flex-col gap-8 pb-8">
			<h1 class="text-2xl font-semibold">Chat Threads</h1>

			<div class="flex flex-col gap-2">
				<div class="flex items-center gap-4">
					<Search
						value={query}
						class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
						onChange={updateQuery}
						placeholder="Search threads..."
					/>
					<button
						class="hover:bg-surface1 dark:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 button flex h-12 w-fit items-center justify-center gap-1 rounded-lg border border-transparent bg-white shadow-sm"
						onclick={() => {
							showFilters = true;
							rightSidebar?.show();
						}}
					>
						<Funnel class="size-4" />
						Filters
					</button>
				</div>

				{#if loading}
					<div class="flex w-full justify-center py-12">
						<LoaderCircle class="size-8 animate-spin text-blue-600" />
					</div>
				{:else if filteredThreads.length === 0}
					<div class="flex w-full flex-col items-center justify-center py-12 text-center">
						<MessageCircle class="size-24 text-gray-200 dark:text-gray-700" />
						<h3 class="mt-4 text-lg font-semibold text-gray-400 dark:text-gray-600">
							{#if query}
								No threads found
							{:else}
								No threads available
							{/if}
						</h3>
						<p class="mt-2 text-sm font-light text-gray-400 dark:text-gray-600">
							{#if query}
								Try adjusting your search terms.
							{:else}
								Threads will appear here once they are created.
							{/if}
						</p>
					</div>
				{:else}
					<Table
						data={tableData}
						fields={['name', 'userName', 'userEmail', 'projectName', 'created']}
						onSelectRow={(d, isCtrlClick) => {
							if (!profile.current.groups.includes(Group.AUDITOR)) return;
							const url = `/admin/chat-threads/${d.id}`;
							openUrl(url, isCtrlClick);
						}}
						headers={[
							{
								title: 'Name',
								property: 'name'
							},
							{
								title: 'User Name',
								property: 'userName'
							},
							{
								title: 'User Email',
								property: 'userEmail'
							},
							{
								title: 'Project',
								property: 'projectName'
							},
							{
								title: 'Created',
								property: 'created'
							}
						]}
						headerClasses={[
							{
								property: 'name',
								class: 'w-4/12 min-w-sm'
							}
						]}
						sortable={['name', 'userName', 'userEmail', 'projectName', 'created']}
						initSort={{ property: 'created', order: 'desc' }}
					>
						{#snippet actions()}
							<button class="icon-button hover:text-blue-500" title="View Thread">
								<Eye class="size-4" />
							</button>
						{/snippet}
						{#snippet onRenderColumn(property, thread)}
							{#if property === 'name'}
								<span>{formatThreadName(thread)}</span>
							{:else if property === 'created'}
								<span class="text-sm text-gray-600 dark:text-gray-400">
									{formatTimeAgo(thread.created).relativeTime}
								</span>
							{:else}
								{thread[property as keyof typeof thread]}
							{/if}
						{/snippet}
					</Table>
				{/if}
			</div>
		</div>
	</div>
</Layout>

<dialog
	bind:this={rightSidebar}
	use:clickOutside={[handleRightSidebarClose, true]}
	use:dialogAnimation={{ type: 'drawer' }}
	class="dark:border-surface1 dark:bg-surface1 fixed! top-0! right-0! bottom-0! left-auto! z-40 h-screen w-auto max-w-none rounded-none border-0 bg-white shadow-lg outline-none!"
>
	{#if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={searchParamFilters}
			{getFilterDisplayLabel}
			getUserDisplayName={(...args) => getUserDisplayName(userMap, ...args)}
			endpoint={optionsEndpoint as unknown as FilterOptionsEndpoint}
		/>
	{/if}
</dialog>

<svelte:head>
	<title>Obot | Admin - Chat Threads</title>
</svelte:head>
