import type { MCPServerTool, ToolOverride } from '../user/types';

export interface VMcpComponentView {
	key: string;
	name: string;
	icon?: string;
	description?: string;
	id?: string;
	toolOverrides?: ToolOverride[];
	toolPreview?: MCPServerTool[];
}

export type Point = { x: number; y: number };
export type RectLike = Pick<DOMRect, 'left' | 'top' | 'right' | 'bottom' | 'width' | 'height'>;

export type Camera = { x: number; y: number; zoom: number };

export type WorldRect = { left: number; top: number; right: number; bottom: number };

export type RowContext = {
	rowY: number;
	viewTop: number;
	viewBottom: number;
};

export type VMcpSortBy = 'name' | 'created' | 'componentServers';

export type VMcpFilterOption = { id: string; label: string; disabled?: boolean };

export type VMcpFilters = {
	names?: string;
	owners?: string;
	components?: string;
};
