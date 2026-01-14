// json diff utility functions
export function formatJsonWithHighlighting(json: unknown): string {
	try {
		const formatted = JSON.stringify(json, null, 2);

		// Replace decimal numbers
		let highlighted = formatted.replace(/: (\d+\.\d+)/g, ': <span class="text-primary">$1</span>');

		// Replace integer numbers
		highlighted = highlighted.replace(
			/: (\d+)(?!\d*\.)/g,
			': <span class="text-primary">$1</span>'
		);

		// Replace keys
		highlighted = highlighted.replace(/"([^"]+)":/g, '<span class="text-primary">"$1"</span>:');

		// Replace string values
		highlighted = highlighted.replace(
			/: "([^"]+)"/g,
			': <span class="text-gray-600 dark:text-gray-300">"$1"</span>'
		);

		// Replace null
		highlighted = highlighted.replace(/: (null)/g, ': <span class="text-on-surface1">$1</span>');

		// Replace brackets and braces
		highlighted = highlighted.replace(/(".*?")|([{}[\]])/g, (match, stringContent, bracket) => {
			if (stringContent) {
				return stringContent;
			}
			return `<span class="text-on-background">${bracket}</span>`;
		});

		return highlighted;
	} catch (_error) {
		return String(json);
	}
}

// Compute Longest Common Subsequence using dynamic programming
function computeLCS(oldLines: string[], newLines: string[]): number[][] {
	const m = oldLines.length;
	const n = newLines.length;

	// Create DP table
	const dp: number[][] = Array(m + 1)
		.fill(null)
		.map(() => Array(n + 1).fill(0));

	for (let i = 1; i <= m; i++) {
		for (let j = 1; j <= n; j++) {
			if (oldLines[i - 1] === newLines[j - 1]) {
				dp[i][j] = dp[i - 1][j - 1] + 1;
			} else {
				dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1]);
			}
		}
	}

	return dp;
}

// Backtrack to find the diff operations
function backtrackDiff(
	dp: number[][],
	oldLines: string[],
	newLines: string[]
): {
	type: 'unchanged' | 'removed' | 'added';
	line: string;
	oldIndex?: number;
	newIndex?: number;
}[] {
	const result: {
		type: 'unchanged' | 'removed' | 'added';
		line: string;
		oldIndex?: number;
		newIndex?: number;
	}[] = [];

	let i = oldLines.length;
	let j = newLines.length;

	while (i > 0 || j > 0) {
		if (i > 0 && j > 0 && oldLines[i - 1] === newLines[j - 1]) {
			// Lines match - unchanged
			result.unshift({
				type: 'unchanged',
				line: oldLines[i - 1],
				oldIndex: i - 1,
				newIndex: j - 1
			});
			i--;
			j--;
		} else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
			// Line added in new version
			result.unshift({ type: 'added', line: newLines[j - 1], newIndex: j - 1 });
			j--;
		} else if (i > 0) {
			// Line removed from old version
			result.unshift({ type: 'removed', line: oldLines[i - 1], oldIndex: i - 1 });
			i--;
		}
	}

	return result;
}

export function generateJsonDiff(
	oldJson: unknown,
	newJson: unknown
): {
	oldLines: string[];
	newLines: string[];
	unifiedLines: string[];
	diffOps: {
		type: 'unchanged' | 'removed' | 'added';
		line: string;
		oldIndex?: number;
		newIndex?: number;
	}[];
} {
	const oldStr = JSON.stringify(oldJson, null, 2);
	const newStr = JSON.stringify(newJson, null, 2);

	const oldLines = oldStr.split('\n');
	const newLines = newStr.split('\n');

	// Compute LCS and get diff operations
	const dp = computeLCS(oldLines, newLines);
	const diffOps = backtrackDiff(dp, oldLines, newLines);

	// Generate unified diff lines
	const unifiedLines: string[] = diffOps.map((op) => {
		switch (op.type) {
			case 'unchanged':
				return ` ${op.line}`;
			case 'removed':
				return `-${op.line}`;
			case 'added':
				return `+${op.line}`;
		}
	});

	return {
		oldLines,
		newLines,
		unifiedLines,
		diffOps
	};
}

export function formatDiffLine(line: string, type: 'added' | 'removed' | 'unchanged'): string {
	const prefix = type === 'added' ? '+' : type === 'removed' ? '-' : ' ';
	const baseClass = 'font-mono text-sm';
	const typeClass =
		type === 'added'
			? 'bg-green-500/10 dark:bg-green-900/30 text-green-500'
			: type === 'removed'
				? 'bg-red-500/10 text-red-500'
				: 'text-gray-700 dark:text-gray-300';

	return `<div class="${baseClass} ${typeClass} px-2 py-0.5">${prefix}${line}</div>`;
}

export function formatJsonWithDiffHighlighting(
	json: unknown,
	diff: {
		oldLines: string[];
		newLines: string[];
		unifiedLines: string[];
		diffOps: {
			type: 'unchanged' | 'removed' | 'added';
			line: string;
			oldIndex?: number;
			newIndex?: number;
		}[];
	},
	isOldVersion: boolean
): string {
	try {
		let highlighted = '';

		// Filter diff operations based on which version we're displaying
		const relevantOps = diff.diffOps.filter((op) => {
			if (isOldVersion) {
				// For old version: show unchanged and removed lines
				return op.type === 'unchanged' || op.type === 'removed';
			} else {
				// For new version: show unchanged and added lines
				return op.type === 'unchanged' || op.type === 'added';
			}
		});

		for (const op of relevantOps) {
			const line = op.line;

			// Determine line styling based on operation type
			let lineClass = 'text-gray-700 dark:text-gray-300';

			if (op.type === 'removed') {
				lineClass = 'bg-red-500/10 text-red-500';
			} else if (op.type === 'added') {
				lineClass = 'bg-green-500/10 text-green-500';
			}

			// Apply JSON syntax highlighting
			let highlightedLine = line;

			// Replace decimal numbers
			highlightedLine = highlightedLine.replace(
				/: (\d+\.\d+)/g,
				': <span class="text-primary">$1</span>'
			);

			// Replace integer numbers
			highlightedLine = highlightedLine.replace(
				/: (\d+)(?!\d*\.)/g,
				': <span class="text-primary">$1</span>'
			);

			// Replace keys
			highlightedLine = highlightedLine.replace(
				/"([^"]+)":/g,
				'<span class="text-primary">"$1"</span>:'
			);

			// Replace string values
			highlightedLine = highlightedLine.replace(
				/: "([^"]+)"/g,
				': <span class="text-gray-600 dark:text-gray-300 whitespace-normal break-words">"$1"</span>'
			);

			// Replace null
			highlightedLine = highlightedLine.replace(
				/: (null)/g,
				': <span class="text-on-surface1">$1</span>'
			);

			// Replace brackets and braces
			highlightedLine = highlightedLine.replace(
				/(".*?")|([{}[\]])/g,
				(match, stringContent, bracket) => {
					if (stringContent) {
						return stringContent;
					}
					return `<span class="text-on-background">${bracket}</span>`;
				}
			);

			highlighted += `<div class="font-mono text-sm ${lineClass} px-2 py-0.5">${highlightedLine}</div>`;
		}

		return highlighted;
	} catch (_error) {
		return String(json);
	}
}
