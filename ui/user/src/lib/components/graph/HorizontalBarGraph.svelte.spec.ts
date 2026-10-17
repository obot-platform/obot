import HorizontalBarGraph from './HorizontalBarGraph.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

type Row = { name: string; count: number };

// Instantiate the generic component so prop keys are checked against Row.
const BarGraph = HorizontalBarGraph<Row>;

const data: Row[] = [
	{ name: 'github', count: 42 },
	{ name: 'slack', count: 7 }
];

describe('HorizontalBarGraph', () => {
	it('renders one labeled row per item with formatted values', async () => {
		render(BarGraph, {
			data,
			labelKey: 'name',
			valueKey: 'count',
			formatValue: (v: number) => `${v} calls`
		});

		await expect.element(page.getByRole('listitem', { name: 'github: 42 calls' })).toBeVisible();
		await expect.element(page.getByRole('listitem', { name: 'slack: 7 calls' })).toBeVisible();
		await expect.element(page.getByText('42 calls')).toBeVisible();
	});

	it('applies formatLabel to row labels', async () => {
		render(BarGraph, {
			data,
			labelKey: 'name',
			valueKey: 'count',
			formatLabel: (label: unknown) => String(label).toUpperCase()
		});

		await expect.element(page.getByText('GITHUB')).toBeVisible();
		await expect.element(page.getByText('github')).not.toBeInTheDocument();
	});

	it('shows a tooltip for the hovered row', async () => {
		render(BarGraph, { data, labelKey: 'name', valueKey: 'count' });

		await page.getByRole('listitem', { name: 'slack: 7' }).hover();

		await expect.element(page.getByCSS('.tooltip').getByText('slack')).toBeVisible();
	});
});
