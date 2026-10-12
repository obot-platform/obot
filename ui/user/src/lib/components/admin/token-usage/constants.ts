import { m } from '$lib/i18n';

export const ALL_MODELS = 'all_models';
export const ALL_USERS = 'all_users';
export const ALL_API_KEYS = 'all_api_keys';

export const TOKEN_USAGE_PARAMS = {
	START: 'start',
	END: 'end',
	MODEL: 'model',
	USER: 'user',
	API_KEY: 'api_key',
	TOKEN_TYPE: 'token_type',
	GROUP_BY: 'group_by'
} as const;

export const TOKEN_TYPE = {
	INPUT: 'input',
	OUTPUT: 'output',
	SPEND: 'spend'
} as const;

export type TokenType = (typeof TOKEN_TYPE)[keyof typeof TOKEN_TYPE];
export const DEFAULT_TOKEN_TYPE = TOKEN_TYPE.INPUT;

export const TOKEN_GROUP_BY = {
	DEFAULT: 'group_by_default',
	USERS: 'group_by_users',
	MODELS: 'group_by_models'
} as const;

export type TokenGroupBy = (typeof TOKEN_GROUP_BY)[keyof typeof TOKEN_GROUP_BY] | null;
export const DEFAULT_TOKEN_GROUP_BY = TOKEN_GROUP_BY.DEFAULT;

export const USAGE_SUBVIEW = {
	MODELS: 'models',
	USERS: 'users',
	API_KEYS: 'api_keys',
	SPEND: 'spend'
} as const;

export type UsageSubView = (typeof USAGE_SUBVIEW)[keyof typeof USAGE_SUBVIEW];
export const DEFAULT_USAGE_SUBVIEW = USAGE_SUBVIEW.MODELS;

export const USAGE_SUBVIEW_SORT_BY = {
	NAME: 'sort_by_name',
	NAME_REVERSE: 'sort_by_name_reverse',
	TOTAL_TOKENS: 'sort_by_total_tokens',
	TOTAL_TOKENS_REVERSE: 'sort_by_total_tokens_reverse',
	TOTAL_SPEND: 'sort_by_total_spend',
	TOTAL_SPEND_REVERSE: 'sort_by_total_spend_reverse'
} as const;

export type UsageSubViewSortBy = (typeof USAGE_SUBVIEW_SORT_BY)[keyof typeof USAGE_SUBVIEW_SORT_BY];
export const DEFAULT_USAGE_SUBVIEW_SORT_BY = USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS;

export const GRAPH_MODE = {
	BUCKET: 'bucket',
	INPUT_OUTPUT: 'input_output'
} as const;

export type GraphMode = (typeof GRAPH_MODE)[keyof typeof GRAPH_MODE];

export const GRAPH_METRIC = {
	TOKENS: 'tokens',
	SPEND: 'spend'
} as const;

export type GraphMetric = (typeof GRAPH_METRIC)[keyof typeof GRAPH_METRIC];

export const USAGE_BUCKET_LABEL = {
	INPUT: m.core_col_input(),
	OUTPUT: m.core_col_output()
} as const;

export const TOKEN_USAGE_CATEGORY = {
	DEFAULT: m.audit_usage_usage_tokens_category_default(),
	UNKNOWN: m.core_unknown()
} as const;

export const CHART_LABEL = {
	INPUT_TOKENS: m.audit_usage_usage_tokens_chart_input_tokens(),
	OUTPUT_TOKENS: m.audit_usage_usage_tokens_chart_output_tokens(),
	SPEND: m.audit_usage_usage_tokens_chart_spend(),
	TOKENS: m.audit_usage_usage_tokens_chart_tokens()
} as const;

export const TOKEN_TYPE_OPTIONS: { label: string; id: TokenType }[] = [
	{ label: m.audit_usage_usage_tokens_input_tokens(), id: TOKEN_TYPE.INPUT },
	{ label: m.audit_usage_usage_tokens_output_tokens(), id: TOKEN_TYPE.OUTPUT },
	{ label: m.core_spend(), id: TOKEN_TYPE.SPEND }
];

export const TOKEN_GROUP_BY_OPTIONS: {
	label: string;
	id: (typeof TOKEN_GROUP_BY)[keyof typeof TOKEN_GROUP_BY];
}[] = [
	{ label: m.audit_usage_usage_tokens_group_by_token_type(), id: TOKEN_GROUP_BY.DEFAULT },
	{ label: m.audit_usage_usage_tokens_group_by_users(), id: TOKEN_GROUP_BY.USERS },
	{ label: m.audit_usage_usage_tokens_group_by_models(), id: TOKEN_GROUP_BY.MODELS }
];

export const USAGE_SUBVIEW_SORT_BY_TOKEN_OPTIONS: { label: string; id: UsageSubViewSortBy }[] = [
	{ label: m.audit_usage_usage_tokens_sort_name_az(), id: USAGE_SUBVIEW_SORT_BY.NAME },
	{ label: m.audit_usage_usage_tokens_sort_name_za(), id: USAGE_SUBVIEW_SORT_BY.NAME_REVERSE },
	{
		label: m.audit_usage_usage_tokens_sort_tokens_desc(),
		id: USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS
	},
	{
		label: m.audit_usage_usage_tokens_sort_tokens_asc(),
		id: USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS_REVERSE
	}
];

export const USAGE_SUBVIEW_SORT_BY_SPEND_OPTIONS: { label: string; id: UsageSubViewSortBy }[] = [
	{ label: m.audit_usage_usage_tokens_sort_name_az(), id: USAGE_SUBVIEW_SORT_BY.NAME },
	{ label: m.audit_usage_usage_tokens_sort_name_za(), id: USAGE_SUBVIEW_SORT_BY.NAME_REVERSE },
	{
		label: m.audit_usage_usage_tokens_sort_spend_desc(),
		id: USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND
	},
	{
		label: m.audit_usage_usage_tokens_sort_spend_asc(),
		id: USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND_REVERSE
	}
];

const TOTAL_SORT_OPTIONS = [
	USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS,
	USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS_REVERSE,
	USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND,
	USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND_REVERSE
] as const;

export function usageSubViewSortByForView(
	sortBy: UsageSubViewSortBy,
	view: UsageSubView
): UsageSubViewSortBy {
	if (view === USAGE_SUBVIEW.SPEND) {
		if (TOTAL_SORT_OPTIONS.includes(sortBy as (typeof TOTAL_SORT_OPTIONS)[number])) {
			return sortBy === USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS_REVERSE ||
				sortBy === USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND_REVERSE
				? USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND_REVERSE
				: USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND;
		}
		return sortBy;
	}

	if (
		sortBy === USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND ||
		sortBy === USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND_REVERSE
	) {
		return sortBy === USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND_REVERSE
			? USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS_REVERSE
			: USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS;
	}

	return sortBy;
}
