import useSWR from "swr";

import { Workflow } from "~/lib/model/workflows";
import { WorkflowService } from "~/lib/service/api/workflowService";

import { SelectModule } from "~/components/composed/SelectModule";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

type WorkflowSelectModuleProps = {
    onChange: (workflows: string[]) => void;
    selection: string[];
};

export function WorkflowSelectModule(props: WorkflowSelectModuleProps) {
    const { data: workflows } = useSWR(
        WorkflowService.getWorkflows.key(),
        WorkflowService.getWorkflows
    );

    return (
        <SelectModule
            selection={props.selection}
            onChange={props.onChange}
            getItemKey={(workflow) => workflow.id}
            renderDropdownItem={(workflow) => (
                <WorkflowText workflow={workflow} />
            )}
            renderListItem={(workflow) => <WorkflowText workflow={workflow} />}
            buttonText="Add Workflow"
            items={workflows}
        />
    );
}

function WorkflowText({ workflow }: { workflow: Workflow }) {
    const content = (
        <div className="flex items-center gap-2 overflow-hidden">
            <span className="min-w-fit">{workflow.name}</span>
            {workflow.description && (
                <>
                    <span>-</span>
                    <span className="text-muted-foreground truncate">
                        {workflow.description}
                    </span>
                </>
            )}
        </div>
    );

    return (
        <Tooltip>
            <TooltipTrigger asChild>{content}</TooltipTrigger>
            <TooltipContent>{content}</TooltipContent>
        </Tooltip>
    );
}
