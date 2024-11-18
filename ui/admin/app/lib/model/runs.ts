import { CallFrame, RunState } from "@gptscript-ai/gptscript";

import { EntityMeta } from "~/lib/model/primitives";

export type Run = EntityMeta & {
    threadID?: string;
    agentID: string;
    previousRunId?: string;
    input: string;
    output?: string;
    state?: RunState;
    error?: string;
};

export type RunDebug = {
    frames: Calls;
    // todo(tylerslaton): this needs to have the spec and status as that's also being returned, but it's not a priority for now
};

export type Calls = Record<string, CallFrame>;

export const runStateToBadgeColor = (state: RunState | undefined) => {
    switch (state) {
        case "running":
            return "bg-warning";
        case "finished":
        case "continue":
            return "bg-success";
        case "error":
            return "bg-error";
        default:
            return "bg-secondary";
    }
};
