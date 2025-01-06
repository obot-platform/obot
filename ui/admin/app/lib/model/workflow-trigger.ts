export type WorkflowTrigger = {
    id: string;
    type: "webhook" | "schedule";
    name: string;
    workflow: string;
};
