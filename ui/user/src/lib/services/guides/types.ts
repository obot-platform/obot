export interface GuideStep {
	content: string[];
	button?: {
		text: string;
		action: GuideAction | GuideAction[];
	};
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

export type GuideDialogContent =
	| { text: string }
	| { videoUrl: string; title: string }
	| { imageUrl: string; alt: string };

export interface GuideDialog {
	title: string;
	content: GuideDialogContent[];
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
	routeContains?: string;
	elementExists?: string;
	elementMissing?: string;
	highlight?: GuideHighlight;
	listener?: GuideListener;
	dialog?: GuideDialog;
	setPreferredClient?: boolean;
	success?: boolean;
}

export interface GuideListener extends GuideSelector {
	action: GuideAction | GuideAction[];
}
