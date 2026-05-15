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
		highlighted = highlighted.replace(/: (null)/g, ': <span class="text-muted-content">$1</span>');

		// Replace brackets and braces
		highlighted = highlighted.replace(/(".*?")|([{}[\]])/g, (match, stringContent, bracket) => {
			if (stringContent) {
				return stringContent;
			}
			return `<span class="text-base-content">${bracket}</span>`;
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

export type LineDiffOpType = 'unchanged' | 'removed' | 'added' | 'modified';

export type LineDiffOp = {
	type: LineDiffOpType;
	line: string;
	oldIndex?: number;
	newIndex?: number;
	/** When type is 'modified', which side this line belongs to (for filtering old vs new view) */
	modifiedSide?: 'old' | 'new';
};

export type LineDiffResult = {
	oldLines: string[];
	newLines: string[];
	unifiedLines: string[];
	diffOps: LineDiffOp[];
};

/** Line-based diff for plain text (e.g. workflow markdown). Treats consecutive remove+add as "modified". */
export function generateLineDiff(oldText: string, newText: string): LineDiffResult {
	const oldLines = oldText.split('\n');
	const newLines = newText.split('\n');
	const dp = computeLCS(oldLines, newLines);
	const rawOps = backtrackDiff(dp, oldLines, newLines);

	// Mark consecutive removed+added pairs as 'modified'
	const diffOps: LineDiffOp[] = rawOps.map((op, idx) => {
		const next = rawOps[idx + 1];
		const prev = rawOps[idx - 1];
		if (op.type === 'removed' && next?.type === 'added') {
			return { ...op, type: 'modified' as const, modifiedSide: 'old' as const };
		}
		if (op.type === 'added' && prev?.type === 'removed') {
			return { ...op, type: 'modified' as const, modifiedSide: 'new' as const };
		}
		return { ...op, type: op.type };
	});

	const unifiedLines = diffOps.map((op) => {
		switch (op.type) {
			case 'modified':
				return op.modifiedSide === 'old' ? `-${op.line}` : `+${op.line}`;
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

function escapeHtml(text: string): string {
	return text
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#039;');
}

/**
 * Renders diff ops as HTML with red (removal), green (addition), yellow (modified).
 * @param diff Result of generateLineDiff(currentWorkflowContents, latestVersionContents)
 * @param isOldVersion true = show current workflow (removed/modified); false = show latest (added/modified)
 */
export function formatTextWithDiffHighlighting(
	diff: LineDiffResult,
	isOldVersion: boolean
): string {
	const relevantOps = diff.diffOps.filter((op) => {
		if (isOldVersion) {
			return op.type === 'unchanged' || op.type === 'removed' || op.modifiedSide === 'old';
		}
		return op.type === 'unchanged' || op.type === 'added' || op.modifiedSide === 'new';
	});

	let html = '';
	for (const op of relevantOps) {
		const escaped = escapeHtml(op.line);
		let lineClass = 'text-base-content';
		if (op.type === 'removed') {
			lineClass = 'bg-error/20 text-error';
		} else if (op.type === 'added') {
			lineClass = 'bg-success/20 text-success';
		} else if (op.type === 'modified') {
			lineClass = 'bg-warning/20 text-warning';
		}
		html += `<div class="font-mono text-sm whitespace-pre-wrap wrap-break-word ${lineClass} px-2 py-0.5 border-l-2 border-transparent">${escaped}</div>`;
	}
	return html;
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
			? 'bg-success/10 text-success'
			: type === 'removed'
				? 'bg-error/10 text-error'
				: 'text-muted-content';

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
			let lineClass = 'text-muted-content';

			if (op.type === 'removed') {
				lineClass = 'bg-error/10 text-error';
			} else if (op.type === 'added') {
				lineClass = 'bg-success/10 text-success';
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
				': <span class="text-gray-600 dark:text-gray-300 whitespace-normal wrap-break-word">"$1"</span>'
			);

			// Replace null
			highlightedLine = highlightedLine.replace(
				/: (null)/g,
				': <span class="text-muted-content">$1</span>'
			);

			// Replace brackets and braces
			highlightedLine = highlightedLine.replace(
				/(".*?")|([{}[\]])/g,
				(match, stringContent, bracket) => {
					if (stringContent) {
						return stringContent;
					}
					return `<span class="text-base-content">${bracket}</span>`;
				}
			);

			highlighted += `<div class="font-mono text-sm ${lineClass} px-2 py-0.5">${highlightedLine}</div>`;
		}

		return highlighted;
	} catch (_error) {
		return String(json);
	}
}
