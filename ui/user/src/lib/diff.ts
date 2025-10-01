// json diff utility functions
export function formatJsonWithHighlighting(json: unknown): string {
	try {
		const formatted = JSON.stringify(json, null, 2);

		// Replace decimal numbers
		let highlighted = formatted.replace(
			/: (\d+\.\d+)/g,
			': <span class="text-blue-600 dark:text-blue-400">$1</span>'
		);

		// Replace integer numbers
		highlighted = highlighted.replace(
			/: (\d+)(?!\d*\.)/g,
			': <span class="text-blue-600 dark:text-blue-400">$1</span>'
		);

		// Replace keys
		highlighted = highlighted.replace(
			/"([^"]+)":/g,
			'<span class="text-blue-600 dark:text-blue-400">"$1"</span>:'
		);

		// Replace string values
		highlighted = highlighted.replace(
			/: "([^"]+)"/g,
			': <span class="text-gray-600 dark:text-gray-300">"$1"</span>'
		);

		// Replace null
		highlighted = highlighted.replace(/: (null)/g, ': <span class="text-gray-500">$1</span>');

		// Replace brackets and braces
		highlighted = highlighted.replace(/(".*?")|([{}[\]])/g, (match, stringContent, bracket) => {
			if (stringContent) {
				return stringContent;
			}
			return `<span class="text-black dark:text-white">${bracket}</span>`;
		});

		return highlighted;
	} catch (_error) {
		return String(json);
	}
}

export function generateJsonDiff(
	oldJson: unknown,
	newJson: unknown
): { oldLines: string[]; newLines: string[]; unifiedLines: string[] } {
	const oldStr = JSON.stringify(oldJson, null, 2);
	const newStr = JSON.stringify(newJson, null, 2);

	const oldLines = oldStr.split('\n');
	const newLines = newStr.split('\n');

	const oldLineMap = new Map<string, number[]>();
	const newLineMap = new Map<string, number[]>();

	oldLines.forEach((line, index) => {
		const key = line.trim();
		if (!oldLineMap.has(key)) {
			oldLineMap.set(key, []);
		}
		oldLineMap.get(key)!.push(index);
	});

	newLines.forEach((line, index) => {
		const key = line.trim();
		if (!newLineMap.has(key)) {
			newLineMap.set(key, []);
		}
		newLineMap.get(key)!.push(index);
	});

	const unifiedLines: string[] = [];
	let oldIndex = 0;
	let newIndex = 0;

	while (oldIndex < oldLines.length || newIndex < newLines.length) {
		const oldLine = oldLines[oldIndex] || '';
		const newLine = newLines[newIndex] || '';

		if (oldLine === newLine) {
			// Lines match, add as unchanged
			unifiedLines.push(` ${oldLine}`);
			oldIndex++;
			newIndex++;
		} else {
			// Lines don't match, look ahead to see if we can find a match
			let foundMatch = false;

			// Look ahead in new lines for a match with current old line
			for (let i = newIndex + 1; i < newLines.length; i++) {
				if (oldLine === newLines[i]) {
					// Found a match ahead, mark current new lines as added
					for (let j = newIndex; j < i; j++) {
						unifiedLines.push(`+${newLines[j]}`);
					}
					newIndex = i;
					foundMatch = true;
					break;
				}
			}

			// Look ahead in old lines for a match with current new line
			if (!foundMatch) {
				for (let i = oldIndex + 1; i < oldLines.length; i++) {
					if (newLine === oldLines[i]) {
						// Found a match ahead, mark current old lines as removed
						for (let j = oldIndex; j < i; j++) {
							unifiedLines.push(`-${oldLines[j]}`);
						}
						oldIndex = i;
						foundMatch = true;
						break;
					}
				}
			}

			// Check if this line content exists elsewhere in the other version (indicating movement)
			if (!foundMatch) {
				const oldLineContent = oldLine.trim();
				const newLineContent = newLine.trim();

				const oldExistsInNew = oldLineContent && newLineMap.has(oldLineContent);
				const newExistsInOld = newLineContent && oldLineMap.has(newLineContent);

				if (oldExistsInNew && newExistsInOld) {
					// Both lines exist in the other version, this suggests content was moved
					// Mark as unchanged to avoid false removal/addition
					if (oldLine) {
						unifiedLines.push(` ${oldLine}`);
					}
					if (newLine) {
						unifiedLines.push(` ${newLine}`);
					}
					oldIndex++;
					newIndex++;
					foundMatch = true;
				}
			}

			// No match found, mark as changed
			if (!foundMatch) {
				if (oldLine) {
					unifiedLines.push(`-${oldLine}`);
				}
				if (newLine) {
					unifiedLines.push(`+${newLine}`);
				}
				oldIndex++;
				newIndex++;
			}
		}
	}

	return {
		oldLines: oldLines.map((line) => line || ''),
		newLines: newLines.map((line) => line || ''),
		unifiedLines
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
	diff: { oldLines: string[]; newLines: string[]; unifiedLines: string[] },
	isOldVersion: boolean
): string {
	try {
		const formatted = JSON.stringify(json, null, 2);
		const lines = formatted.split('\n');

		let highlighted = '';

		const oldLines: string[] = diff.oldLines.map((line) => line.trim());
		const newLines: string[] = diff.newLines.map((line) => line.trim());

		const changes: [changeType: string | undefined, line: string][] = [];

		for (let i = 0; i < lines.length; i++) {
			const line = lines[i];

			const oldLine = oldLines[i] || '';
			const newLine = newLines[i] || '';

			// Check if this line is different between old and new
			const isChanged = oldLine !== newLine;

			// Check if this line content exists in the other version (indicating it was moved)
			const existsInOld = oldLines.at(i) === line;
			const existsInNew = newLines.at(i) === line;

			// A line is truly removed if it exists in old but not in new
			const isRemoved = isOldVersion && isChanged && oldLine && !newLine && !existsInNew;
			if (isRemoved) {
				changes.push(['removed', line]);
				continue;
			}

			// A line is truly added if it exists in new but not in old
			const isAdded = !isOldVersion && isChanged && newLine && !oldLine && !existsInOld;
			if (isAdded) {
				changes.push(['added', line]);
				continue;
			}

			// A line is modified if both exist but are different (and not just moved)
			const isModified = isChanged && oldLine && newLine && oldLine !== newLine;
			if (isModified) {
				changes.push(['modified', line]);
				continue;
			}

			changes.push([undefined, line]);
		}

		let i = -1;
		while (Math.abs(i) <= changes.length) {
			const oldLine = diff.oldLines.at(i);
			const newLine = diff.newLines.at(i);

			if (oldLine === undefined && newLine === undefined) {
				break;
			}

			const line = changes.at(i);
			if (line) {
				if (['}', ']'].includes(line[1]?.trim()) && oldLine === newLine) {
					line[0] = undefined;
				}
			}

			i--;
		}

		for (let i = 0; i < changes.length; i++) {
			const index = i;
			const [changeType, line] = changes[index];

			// A line is truly removed if it exists in old but not in new
			const isRemoved = changeType === 'removed';
			// A line is truly added if it exists in new but not in old
			const isAdded = changeType === 'added';
			// A line is modified if both exist but are different (and not just moved)
			const isModified = changeType === 'modified';

			// For old version: highlight removed and modified lines in red
			// For new version: highlight added and modified lines in green
			let lineClass = 'text-gray-700 dark:text-gray-300';

			if (isRemoved || (isOldVersion && isModified)) {
				lineClass = 'bg-red-500/10 text-red-500';
			} else if (isAdded || (!isOldVersion && isModified)) {
				lineClass = 'bg-green-500/10 text-green-500';
			}

			// Apply JSON syntax highlighting
			let highlightedLine = line;

			// Replace decimal numbers
			highlightedLine = highlightedLine.replace(
				/: (\d+\.\d+)/g,
				': <span class="text-blue-600 dark:text-blue-400">$1</span>'
			);

			// Replace integer numbers
			highlightedLine = highlightedLine.replace(
				/: (\d+)(?!\d*\.)/g,
				': <span class="text-blue-600 dark:text-blue-400">$1</span>'
			);

			// Replace keys
			highlightedLine = highlightedLine.replace(
				/"([^"]+)":/g,
				'<span class="text-blue-600 dark:text-blue-400">"$1"</span>:'
			);

			// Replace string values
			highlightedLine = highlightedLine.replace(
				/: "([^"]+)"/g,
				': <span class="text-gray-600 dark:text-gray-300 whitespace-normal break-words">"$1"</span>'
			);

			// Replace null
			highlightedLine = highlightedLine.replace(
				/: (null)/g,
				': <span class="text-gray-500">$1</span>'
			);

			// Replace brackets and braces
			highlightedLine = highlightedLine.replace(
				/(".*?")|([{}[\]])/g,
				(match, stringContent, bracket) => {
					if (stringContent) {
						return stringContent;
					}
					return `<span class="text-black dark:text-white">${bracket}</span>`;
				}
			);

			highlighted += `<div class="font-mono text-sm ${lineClass} px-2 py-0.5">${highlightedLine}</div>`;
		}

		return highlighted;
	} catch (_error) {
		return String(json);
	}
}
