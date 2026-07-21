export interface GuideStep {
	content: GuideContent[];
	action?: GuideAction | GuideAction[];
	buttons?: GuideButton[];
}

export interface GuideButton {
	text: string;
	action?: GuideAction | GuideAction[];
	steps?: GuideStep[];
}

export interface GuideSelector {
	id?: string;
	beginsWith?: string[];
}

export interface Guide {
	id: string;
	title: string;
	steps: GuideStep[];
}

export type GuideContent =
	| string
	| { text: string; type: 'text' | 'code' }
	| { videoUrl: string; title: string }
	| { imageUrl: string; alt: string };

export interface GuideDialog {
	title: string;
	content: GuideContent[];
	next?: GuideDialog;
}

export interface GuideHighlight {
	selector: GuideSelector;
	title?: string;
	description?: string;
	side?: 'top' | 'right' | 'bottom' | 'left';
	align?: 'start' | 'center' | 'end';
}

export interface GuideAction {
	closeExistingElement?: boolean;
	dialog?: GuideDialog;
	elementExists?: string;
	elementMissing?: string;
	highlight?: GuideHighlight;
	listener?: GuideListener;
	next?: { action: GuideAction | GuideAction[] };
	routeContains?: string;
	setPreferredClient?: boolean;
	skipClickTargetOnNext?: boolean;
	success?: boolean;
	waitFor?: string;
}

export interface GuideListener extends GuideSelector {
	action: GuideAction | GuideAction[];
	skipClickTargetOnNext?: boolean;
}
