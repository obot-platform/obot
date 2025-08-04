<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import Select from '$lib/components/Select.svelte';
	import { AdminService, type ProjectThread, type Project, type OrgUser } from '$lib/services';
	import { Eye, LoaderCircle, MessageCircle, Funnel, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { afterNavigate, goto } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';
	import Search from '$lib/components/Search.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';

	let threads = $state<ProjectThread[]>([]);
	let filteredThreads = $state<ProjectThread[]>([]);
	let projects = $state<Project[]>([]);
	let users = $state<OrgUser[]>([]);
	let projectMap = $derived(new Map(projects.map((p) => [p.id, p.name])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));

	let rightSidebar = $state<HTMLDialogElement>();
	let filters = $state({
		username: '',
		email: '',
		project: ''
	});
	let modifiedFilters = $state({
		username: '',
		email: '',
		project: ''
	});

	// Get unique options from thread data for Select components
	let usernameOptions = $derived.by(() => {
		const usernames = new Set<string>();
		threads.forEach((thread) => {
			const user = userMap.get(thread.userID || '');
			if (user?.displayName) {
				usernames.add(user.displayName);
			}
		});
		return Array.from(usernames)
			.sort()
			.map((username) => ({ id: username, label: username }));
	});

	let emailOptions = $derived.by(() => {
		const emails = new Set<string>();
		threads.forEach((thread) => {
			const user = userMap.get(thread.userID || '');
			if (user?.email) {
				emails.add(user.email);
			}
		});
		return Array.from(emails)
			.sort()
			.map((email) => ({ id: email, label: email }));
	});

	let projectOptions = $derived.by(() => {
		const projectNames = new Set<string>();
		threads.forEach((thread) => {
			const projectName = projectMap.get(thread.projectID || '') || thread.projectID;
			if (projectName) {
				projectNames.add(projectName);
			}
		});
		return Array.from(projectNames)
			.sort()
			.map((projectName) => ({ id: projectName, label: projectName }));
	});
	let loading = $state(true);
	let searchTerm = $state('');
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

	function getFiltersFromUrl() {
		const url = new URL(window.location.href);
		return {
			username: url.searchParams.get('username') || '',
			email: url.searchParams.get('email') || '',
			project: url.searchParams.get('project') || ''
		};
	}

	onMount(() => {
		loadThreads();
	});

	afterNavigate(() => {
		filters = getFiltersFromUrl();
		modifiedFilters = { ...filters };
	});

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

			threads = threadsData;
			projects = projectsData;
			users = usersData;
			// Filter to only include project threads (project: true) and exclude system tasks
			filteredThreads = threads.filter((thread) => thread.project && !thread.systemTask);
		} catch (error) {
			console.error('Failed to load data:', error);
			// Set empty arrays as fallback
			threads = [];
			projects = [];
			users = [];
			filteredThreads = [];
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		// First filter to only include project threads and exclude system tasks
		let filtered = threads.filter((thread) => !thread.project && !thread.systemTask);

		// Then apply search filter
		if (searchTerm.trim() !== '') {
			const term = searchTerm.toLowerCase();
			filtered = filtered.filter((thread) => {
				const user = userMap.get(thread.userID || '');
				return (
					thread.name?.toLowerCase().includes(term) ||
					thread.id.toLowerCase().includes(term) ||
					thread.userID?.toLowerCase().includes(term) ||
					thread.projectID?.toLowerCase().includes(term) ||
					user?.displayName?.toLowerCase().includes(term) ||
					user?.email?.toLowerCase().includes(term) ||
					projectMap
						.get(thread.projectID || '')
						?.toLowerCase()
						.includes(term)
				);
			});
		}

		// Apply specific filters
		if (filters.username.trim() !== '') {
			const usernameTerm = filters.username.toLowerCase();
			filtered = filtered.filter((thread) => {
				const user = userMap.get(thread.userID || '');
				return user?.displayName?.toLowerCase().includes(usernameTerm);
			});
		}

		if (filters.email.trim() !== '') {
			const emailTerm = filters.email.toLowerCase();
			filtered = filtered.filter((thread) => {
				const user = userMap.get(thread.userID || '');
				return user?.email?.toLowerCase().includes(emailTerm);
			});
		}

		if (filters.project.trim() !== '') {
			const projectTerm = filters.project.toLowerCase();
			filtered = filtered.filter((thread) => {
				const projectName = projectMap.get(thread.projectID || '') || thread.projectID;
				return projectName?.toLowerCase().includes(projectTerm);
			});
		}

		filteredThreads = filtered;
	});

	function handleViewThread(thread: ProjectThread) {
		// Navigate to thread view
		goto(`/admin/chat-threads/${thread.id}`);
	}

	function formatThreadName(thread: ProjectThread) {
		return thread.name || 'Unnamed Thread';
	}

	function handleRightSidebarClose() {
		rightSidebar?.close();
		modifiedFilters = filters;
	}

	function handleSetFilters() {
		filters = modifiedFilters;
		rightSidebar?.close();
		const url = new URL(window.location.href);
		url.searchParams.set('username', filters.username);
		url.searchParams.set('email', filters.email);
		url.searchParams.set('project', filters.project);
		goto(url.toString(), { noScroll: true });
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
						class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
						onChange={(val) => (searchTerm = val)}
						placeholder="Search threads..."
					/>
					<button
						class="hover:bg-surface1 dark:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 button flex h-12 w-fit items-center justify-center gap-1 rounded-lg border border-transparent bg-white shadow-sm"
						onclick={() => {
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
							{#if searchTerm}
								No threads found
							{:else}
								No threads available
							{/if}
						</h3>
						<p class="mt-2 text-sm font-light text-gray-400 dark:text-gray-600">
							{#if searchTerm}
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
						onSelectRow={handleViewThread}
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
					>
						{#snippet actions(thread)}
							<button
								class="icon-button hover:text-blue-500"
								onclick={(e) => {
									e.stopPropagation();
									handleViewThread(thread);
								}}
								title="View Thread"
							>
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
	<div class="dark:border-surface3 h-full w-screen border-l border-transparent md:w-sm">
		<div class="relative w-full text-center">
			<h4 class="p-4 text-xl font-semibold">Filters</h4>
			<button
				class="icon-button absolute top-1/2 right-4 -translate-y-1/2"
				onclick={handleRightSidebarClose}
			>
				<X class="size-5" />
			</button>
		</div>
		<div
			class="default-scrollbar-thin flex h-[calc(100%-60px)] flex-col gap-4 overflow-y-auto p-4 pt-0"
		>
			<div class="mb-2 flex flex-col gap-1">
				<label for="username-select" class="text-md font-light"> Username </label>
				<Select
					classes={{
						clear: 'hover:bg-surface3 bg-transparent'
					}}
					id="username-select"
					class="dark:border-surface3 bg-surface1 border border-transparent shadow-inner dark:bg-black"
					options={usernameOptions}
					selected={modifiedFilters.username}
					onSelect={(option) => (modifiedFilters.username = option.id)}
					position="top"
					onClear={() => (modifiedFilters.username = '')}
				/>
			</div>
			<div class="mb-2 flex flex-col gap-1">
				<label for="email-select" class="text-sm"> Email </label>
				<Select
					classes={{
						clear: 'hover:bg-surface3 bg-transparent'
					}}
					id="email-select"
					class="bg-surface1 dark:border-surface3 border border-transparent shadow-inner dark:bg-black"
					options={emailOptions}
					selected={modifiedFilters.email}
					onSelect={(option) => (modifiedFilters.email = option.id)}
					position="top"
					onClear={() => (modifiedFilters.email = '')}
				/>
			</div>
			<div class="mb-2 flex flex-col gap-1">
				<label for="project-select" class="text-sm"> Project Name </label>
				<Select
					classes={{
						clear: 'hover:bg-surface3 bg-transparent'
					}}
					id="project-select"
					class="bg-surface1 dark:border-surface3 border border-transparent shadow-inner dark:bg-black"
					options={projectOptions}
					selected={modifiedFilters.project}
					onSelect={(option) => (modifiedFilters.project = option.id)}
					position="top"
					onClear={() => (modifiedFilters.project = '')}
				/>
			</div>
			<div class="mt-auto">
				<button
					class="button-primary text-md w-full rounded-lg px-4 py-2"
					onclick={handleSetFilters}>Apply Filters</button
				>
			</div>
		</div>
	</div>
</dialog>

<svelte:head>
	<title>Obot | Admin - Chat Threads</title>
</svelte:head>
