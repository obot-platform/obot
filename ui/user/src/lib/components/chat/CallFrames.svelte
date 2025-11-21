<script lang="ts">
	import type { CallFrame } from '$lib/services';

	interface Props {
		calls: Record<string, CallFrame>;
	}

	const { calls }: Props = $props();

	// Build tree structure
	function buildTree(calls: Record<string, CallFrame>) {
		const tree: Record<string, string[]> = {};
		const rootNodes: string[] = [];

		// Sort calls by start timestamp
		const sortedCalls = Object.entries(calls).sort(
			(a, b) => new Date(a[1].start).getTime() - new Date(b[1].start).getTime()
		);

		sortedCalls.forEach(([id, call]) => {
			if (
				call.tool?.modelProvider &&
				(call.tool?.name === 'GPTScript Gateway Provider' || call.tool?.name === 'Obot')
			) {
				return;
			}

			const parentId = call.parentID || '';
			if (!parentId) {
				rootNodes.push(id);
			} else {
				if (!tree[parentId]) {
					tree[parentId] = [];
				}
				tree[parentId].push(id);
			}
		});

		return { tree, rootNodes };
	}

	function handleDownload() {
		const dataStr =
			'data:text/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(calls, null, 2));
		const downloadAnchorNode = document.createElement('a');
		downloadAnchorNode.setAttribute('href', dataStr);
		downloadAnchorNode.setAttribute('download', 'calls.json');
		document.body.appendChild(downloadAnchorNode);
		downloadAnchorNode.click();
		downloadAnchorNode.remove();
	}
</script>
