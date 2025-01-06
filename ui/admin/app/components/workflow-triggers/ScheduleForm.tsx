import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { useNavigate } from "react-router";
import { $path } from "safe-routes";
import { toast } from "sonner";
import useSWR, { mutate } from "swr";
import { z } from "zod";

import { CronJob } from "~/lib/model/cronjobs";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { WorkflowService } from "~/lib/service/api/workflowService";

import { TypographyH2 } from "~/components/Typography";
import {
    ControlledCustomInput,
    ControlledInput,
} from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "~/components/ui/select";
import { ScheduleSelection } from "~/components/workflow-triggers/shared/ScheduleSelection";
import { useAsync } from "~/hooks/useAsync";

const formSchema = z.object({
    description: z.string(),
    workflow: z.string().min(1, "Workflow is required"),
    schedule: z.string(),
});

export type ScheduleFormValues = z.infer<typeof formSchema>;

export function ScheduleForm({ cronjob }: { cronjob?: CronJob }) {
    const navigate = useNavigate();
    const getWorkflows = useSWR(WorkflowService.getWorkflows.key(), () =>
        WorkflowService.getWorkflows()
    );

    const handleSubmitSuccess = () => {
        if (cronjob) {
            mutate(CronJobApiService.getCronJobById(cronjob.id));
        }
        mutate(CronJobApiService.getCronJobs.key());
        navigate($path("/workflow-triggers"));
    };

    const createSchedule = useAsync(CronJobApiService.createCronJob, {
        onSuccess: handleSubmitSuccess,
        onError: () => {
            toast.error("Failed to create schedule.");
        },
    });

    const updateSchedule = useAsync(CronJobApiService.updateCronJob, {
        onSuccess: handleSubmitSuccess,
        onError: () => {
            toast.error("Failed to update schedule.");
        },
    });

    const form = useForm<ScheduleFormValues>({
        resolver: zodResolver(formSchema),
        mode: "onChange",
        defaultValues: {
            description: cronjob?.description || "",
            workflow: cronjob?.workflow || "",
            schedule: cronjob?.schedule || "0 * * * *", // default to hourly
        },
    });

    useEffect(() => {
        if (cronjob) {
            form.reset(cronjob);
        }
    }, [cronjob, form]);

    const handleSubmit = form.handleSubmit((values: ScheduleFormValues) =>
        cronjob?.id
            ? updateSchedule.execute(cronjob.id, values)
            : createSchedule.execute(values)
    );

    const workflows = getWorkflows.data;
    const hasCronJob = !!cronjob?.id;
    const loading = createSchedule.isLoading || updateSchedule.isLoading;

    return (
        <ScrollArea className="h-full">
            <Form {...form}>
                <form
                    className="space-y-8 p-8 max-w-3xl mx-auto"
                    onSubmit={handleSubmit}
                >
                    <TypographyH2>
                        {hasCronJob ? "Edit" : "Create"} Schedule
                    </TypographyH2>

                    <ControlledInput
                        control={form.control}
                        name="description"
                        label="Description (Optional)"
                    />

                    <ScheduleSelection
                        label="Schedule"
                        onChange={(schedule) => {
                            form.setValue("schedule", schedule, {
                                shouldValidate: true,
                                shouldDirty: true,
                            });
                        }}
                        value={form.watch("schedule")}
                    />

                    <ControlledCustomInput
                        control={form.control}
                        name="workflow"
                        label="Workflow"
                        description="The workflow that will be called on the interval determined by the schedule set above."
                    >
                        {({ field: { ref: _, ...field }, className }) => (
                            <Select
                                defaultValue={field.value}
                                onValueChange={field.onChange}
                                key={field.value}
                            >
                                <SelectTrigger className={className}>
                                    <SelectValue placeholder="Select a workflow" />
                                </SelectTrigger>

                                <SelectContent>
                                    {getWorkflowOptions()}
                                </SelectContent>
                            </Select>
                        )}
                    </ControlledCustomInput>

                    <Button
                        className="w-full"
                        type="submit"
                        disabled={loading}
                        loading={loading}
                    >
                        {hasCronJob ? "Update" : "Create"} Schedule
                    </Button>
                </form>
            </Form>
        </ScrollArea>
    );

    function getWorkflowOptions() {
        const workflow = form.watch("workflow");

        if (getWorkflows.isLoading)
            return (
                <SelectItem value={workflow || "loading"} disabled>
                    Loading workflows...
                </SelectItem>
            );

        if (!workflows?.length)
            return (
                <SelectItem value={workflow || "empty"} disabled>
                    No workflows found
                </SelectItem>
            );

        return workflows.map((workflow) => (
            <SelectItem key={workflow.id} value={workflow.id}>
                {workflow.name}
            </SelectItem>
        ));
    }
}
