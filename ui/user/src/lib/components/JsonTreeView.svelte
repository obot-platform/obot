<script lang="ts">
	import Self from './JsonTreeView.svelte';

	interface Props {
		data: Record<string, unknown> | Array<unknown>;
		depth?: number;
		expanded?: boolean;
		isLast?: boolean;
	}

	const { data, depth = 0, expanded, isLast }: Props = $props();

	let nodeExpanded = $derived(expanded || depth === 0);
	let expandedChildren = $state<Record<string, boolean>>({});

	function getValueType(value: unknown): string {
		if (value === null) return 'null';
		if (Array.isArray(value)) return 'array';
		return typeof value;
	}

	function isPrimitive(value: unknown): boolean {
		const type = getValueType(value);
		return ['string', 'number', 'boolean', 'null', 'undefined'].includes(type);
	}

	function formatPrimitive(value: unknown): string {
		if (value === null) return 'null';
		if (value === undefined) return 'undefined';
		if (typeof value === 'string') return `"${value}"`;
		return String(value);
	}

	function getTypeColor(value: unknown): string {
		const type = getValueType(value);
		switch (type) {
			case 'string':
				return 'text-[#cb7832] dark:text-[#e5c07b]'; // String color
			case 'number':
				return 'text-[#6897bb] dark:text-[#d19a66] font-semibold'; // Number color
			case 'boolean':
				return 'text-[#6897bb] dark:text-[#d19a66] font-semibold'; // Boolean color
			case 'null':
			case 'undefined':
				return 'text-on-surface1'; // Null/undefined color
			default:
				return 'text-on-background';
		}
	}

	function getPreview(value: unknown): string {
		if (Array.isArray(value)) {
			if (value.length === 0) return '';
			return `... ${value.length} items`;
		}
		if (value && typeof value === 'object') {
			const keys = Object.keys(value);
			if (keys.length === 0) return '';
			return '...';
		}
		return '';
	}
</script>

<div class="inline-flex font-mono text-base leading-[18px]">
	<span class="inline-flex flex-col">
		<span class="inline-flex items-baseline">
			{#if depth === 0}
				<button
					class="mr-0.5 inline-flex items-center text-base"
					onclick={() => (nodeExpanded = !nodeExpanded)}
					aria-label={nodeExpanded ? 'Collapse' : 'Expand'}
				>
					<span class="inline-block w-3 text-center select-none">
						{nodeExpanded ? '▼' : '▶'}
					</span>
				</button>
				<span>
					{Array.isArray(data) ? '[' : '{'}
				</span>
			{/if}
			{#if !nodeExpanded}
				<span class="ml-1">
					{getPreview(data)}
				</span>
				<span>
					{Array.isArray(data) ? ']' : '}'}
				</span>
			{/if}
		</span>
	</span>
</div>
{#if nodeExpanded}
	{@const keyValuePairs = Object.entries(data)}
	<div class="ml-5 flex flex-col">
		{#each keyValuePairs as [key, value], i (key)}
			{@const isPrimitiveValue = isPrimitive(value)}
			<span class="inline-flex items-baseline">
				{#if !isPrimitiveValue}
					<button
						class="mr-1 inline-flex items-center text-base"
						onclick={() => {
							if (!expandedChildren[key]) {
								expandedChildren[key] = true;
							} else {
								expandedChildren[key] = !expandedChildren[key];
							}
						}}
						aria-label={expandedChildren[key] ? 'Collapse' : 'Expand'}
					>
						<span class="inline-block w-3 text-center select-none">
							{expandedChildren[key] ? '▼' : '▶'}
						</span>
					</button>
				{/if}
				<span class="text-primary">
					{key}
				</span>
				<span>:</span>
				<span class="ml-1">
					{#if isPrimitiveValue}
						<span class={getTypeColor(value)}>{formatPrimitive(value)}</span>
					{:else}
						<span>
							{Array.isArray(value) ? '[' : '{'}
						</span>
						{#if !expandedChildren[key]}
							<span>
								{getPreview(value)}
							</span>
							<span>
								{Array.isArray(value) ? ']' : '}'}
							</span>
						{/if}
					{/if}
				</span>
				{#if !expandedChildren[key]}
					<span>,</span>
				{/if}
			</span>
			{#if expandedChildren[key]}
				<Self
					data={value as Record<string, unknown> | Array<unknown>}
					depth={depth + 1}
					expanded={expandedChildren[key] || expanded}
					isLast={i === keyValuePairs.length - 1}
				/>
			{/if}
		{/each}
	</div>
	<span>
		{Array.isArray(data) ? ']' : '}'}{#if !isLast && depth > 0},{/if}
	</span>
{/if}
