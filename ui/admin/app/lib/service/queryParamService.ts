import { z } from "zod";

export const QueryParamSchemas = {
    Runs: z.object({
        threadId: z.string(),
    }),
    Threads: z.object({
        agentId: z.string().optional(),
        workflowId: z.string().optional(),
    }),
};
