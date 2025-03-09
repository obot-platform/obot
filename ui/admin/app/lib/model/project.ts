import { AgentIcons } from "~/lib/model/agents";
import { EntityMeta } from "~/lib/model/primitives";
import { ThreadManifest } from "~/lib/model/threads";

type ProjectManifest = ThreadManifest & {
	parentID?: Nullish<string>;
	assistantID: string;
	editor: boolean;
	userID: string;
};

export type Project = EntityMeta & ProjectManifest;

type ProjectShareManifest = {
	public: Nullish<boolean>;
	users?: Nullish<string[]>;
};

export type ProjectShare = EntityMeta &
	ProjectShareManifest & {
		publicID?: Nullish<string>;
		projectID?: Nullish<string>;
		name?: Nullish<string>;
		description?: Nullish<string>;
		icons?: Nullish<AgentIcons>;
		featured?: Nullish<boolean>;
	};

export const ShareStatus = {
	Featured: "featured",
	Public: "public",
	Private: "private",
} as const;
export type ShareStatus = (typeof ShareStatus)[keyof typeof ShareStatus];

export function getShareStatusLabel(privacy: ShareStatus) {
	switch (privacy) {
		case ShareStatus.Featured:
			return "Featured";
		case ShareStatus.Public:
			return "Public";
		case ShareStatus.Private:
			return "Private";
	}
}
