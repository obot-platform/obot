import type { Model, OrgUser, TokenUsage, TokenUsageWithCategory } from '$lib/services';
import {
	CHART_LABEL,
	TOKEN_GROUP_BY,
	TOKEN_TYPE,
	TOKEN_USAGE_CATEGORY,
	USAGE_BUCKET_LABEL,
	type TokenGroupBy,
	type TokenType
} from './constants';
import { aggregateTimelineDataByBucket, getUserLabels } from './utils';

export type TokenUsageTimelineItem = TokenUsageWithCategory & {
	bucketTokens?: number;
	bucketSpend?: number;
};

type UsageBucket = {
	label: string;
	tokens: number;
	spend: number;
	thinkingTokens?: number;
};

export const TIMELINE_AGGREGATE_THRESHOLD = 500;

export const bucketTooltipValueKeys: (keyof TokenUsageTimelineItem)[] = [
	'bucketSpend',
	'cacheReadTokens',
	'cacheWriteTokens',
	'cacheReadSpend',
	'cacheWriteSpend',
	'thinkingTokens'
];

export const mainTooltipValueKeys: (keyof TokenUsageTimelineItem)[] = [
	'inputSpend',
	'outputSpend',
	'cacheReadTokens',
	'cacheWriteTokens',
	'cacheReadSpend',
	'cacheWriteSpend',
	'thinkingTokens'
];

export function formatTokenUsageUSD(value: number): string {
	const fractionDigits = value !== 0 && Math.abs(value) < 0.01 ? 4 : 2;
	return value.toLocaleString(undefined, {
		style: 'currency',
		currency: 'USD',
		minimumFractionDigits: 2,
		maximumFractionDigits: fractionDigits
	});
}

function positive(value: number | undefined): number {
	return Math.max(value ?? 0, 0);
}

export function toTimelineItem(r: TokenUsage, category: string): TokenUsageTimelineItem {
	return {
		...r,
		date: r.date,
		inputTokens: r.inputTokens ?? 0,
		cacheReadTokens: r.cacheReadTokens ?? 0,
		cacheWriteTokens: r.cacheWriteTokens ?? 0,
		outputTokens: r.outputTokens ?? 0,
		thinkingTokens: r.thinkingTokens ?? 0,
		totalTokens: r.totalTokens ?? (r.inputTokens ?? 0) + (r.outputTokens ?? 0),
		inputSpend: r.inputSpend ?? 0,
		cacheReadSpend: r.cacheReadSpend ?? 0,
		cacheWriteSpend: r.cacheWriteSpend ?? 0,
		outputSpend: r.outputSpend ?? 0,
		totalSpend: r.totalSpend ?? 0,
		category
	};
}

function tokenBuckets(r: TokenUsage): UsageBucket[] {
	const inputTokens = positive(r.inputTokens);
	const outputTokens = positive(r.outputTokens);

	return [
		{ label: USAGE_BUCKET_LABEL.INPUT, tokens: inputTokens, spend: positive(r.inputSpend) },
		{
			label: USAGE_BUCKET_LABEL.OUTPUT,
			tokens: outputTokens,
			spend: positive(r.outputSpend),
			thinkingTokens: positive(r.thinkingTokens)
		}
	].filter((bucket) => bucket.tokens > 0 || bucket.spend > 0);
}

export function toBucketTimelineItems(r: TokenUsage): TokenUsageTimelineItem[] {
	return tokenBuckets(r).map((bucket) => ({
		...toTimelineItem(r, bucket.label),
		bucketTokens: bucket.tokens,
		bucketSpend: bucket.spend,
		thinkingTokens: bucket.thinkingTokens ?? 0
	}));
}

export function computeMainTimelineData(
	filtered: TokenUsage[],
	group: TokenGroupBy,
	users: Map<string, OrgUser>,
	modelToName: Map<string, string>,
	tokenType: TokenType
): TokenUsageTimelineItem[] {
	if (group === TOKEN_GROUP_BY.USERS) {
		const userKeys = [
			...new Set(filtered.map((r) => r.userID ?? TOKEN_USAGE_CATEGORY.UNKNOWN))
		].sort();
		const userKeyToLabel = getUserLabels(users, userKeys);
		return filtered.map((r) =>
			toTimelineItem(
				r,
				userKeyToLabel.get(r.userID ?? TOKEN_USAGE_CATEGORY.UNKNOWN) ??
					r.userID ??
					TOKEN_USAGE_CATEGORY.UNKNOWN
			)
		);
	}
	if (group === TOKEN_GROUP_BY.MODELS) {
		return filtered.map((r) =>
			toTimelineItem(r, modelToName.get(r.model ?? '') ?? r.model ?? TOKEN_USAGE_CATEGORY.UNKNOWN)
		);
	}
	if (tokenType === TOKEN_TYPE.SPEND) {
		return filtered.flatMap(toBucketTimelineItems);
	}
	return filtered.map((r) => toTimelineItem(r, TOKEN_USAGE_CATEGORY.DEFAULT));
}

export function timelineDataForChartWithRange(
	items: TokenUsageTimelineItem[],
	start: Date,
	end: Date
): TokenUsageTimelineItem[] {
	if (items.length <= TIMELINE_AGGREGATE_THRESHOLD) return items;
	return aggregateTimelineDataByBucket(items, start, end) as TokenUsageTimelineItem[];
}

export function buildMainChartData(
	filtered: TokenUsage[],
	group: TokenGroupBy,
	tokenType: TokenType,
	start: Date,
	end: Date,
	users: Map<string, OrgUser>,
	modelToName: Map<string, string>
): TokenUsageTimelineItem[] {
	const timeline = computeMainTimelineData(filtered, group, users, modelToName, tokenType);
	if (filtered.length <= TIMELINE_AGGREGATE_THRESHOLD) return timeline;
	return timelineDataForChartWithRange(timeline, start, end);
}

export function mainChartUsesSpendBuckets(
	selectedTokenType: TokenType,
	groupBy: TokenGroupBy
): boolean {
	return selectedTokenType === TOKEN_TYPE.SPEND && groupBy === TOKEN_GROUP_BY.DEFAULT;
}

export function mainPrimaryValueKey(
	selectedTokenType: TokenType,
	usesSpendBuckets: boolean
): keyof TokenUsageTimelineItem {
	if (usesSpendBuckets) return 'bucketSpend';
	if (selectedTokenType === TOKEN_TYPE.INPUT) return 'inputTokens';
	if (selectedTokenType === TOKEN_TYPE.OUTPUT) return 'outputTokens';
	return 'totalSpend';
}

export function mainPrimaryLabel(selectedTokenType: TokenType, usesSpendBuckets: boolean): string {
	if (usesSpendBuckets) return CHART_LABEL.SPEND;
	if (selectedTokenType === TOKEN_TYPE.INPUT) return CHART_LABEL.INPUT_TOKENS;
	if (selectedTokenType === TOKEN_TYPE.OUTPUT) return CHART_LABEL.OUTPUT_TOKENS;
	return CHART_LABEL.SPEND;
}

export function mainChartTooltipValueKeys(
	usesSpendBuckets: boolean
): (keyof TokenUsageTimelineItem)[] {
	return usesSpendBuckets ? bucketTooltipValueKeys : mainTooltipValueKeys;
}

export function targetModelToDisplayNameMap(models: Model[]): Map<string, string> {
	return new Map(models.map((m) => [m.targetModel, m.displayName || m.name]));
}
