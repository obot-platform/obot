import { toast } from "sonner";
import useSWR, { mutate } from "swr";

import { CronJobApiService } from "~/lib/service/api/cronjobApiService";

import { useAsync } from "~/hooks/useAsync";

export function useCronjob(workflowId?: string) {
    const { data: cronJobs } = useSWR(CronJobApiService.getCronJobs.key(), () =>
        CronJobApiService.getCronJobs()
    );

    const cronJob = cronJobs?.find(
        (cronJob) => cronJob.workflow === workflowId
    );

    const createCronJob = useAsync(CronJobApiService.createCronJob, {
        onSuccess: () => {
            mutate(CronJobApiService.getCronJobs.key());
        },
        onError: (error) => {
            if (error instanceof Error) toast.error("Something went wrong");
        },
    });

    const updateCronJob = useAsync(CronJobApiService.updateCronJob, {
        onSuccess: () => {
            mutate(CronJobApiService.getCronJobs.key());
        },
        onError: (error) => {
            if (error instanceof Error) toast.error("Something went wrong");
        },
    });

    const deleteCronJob = useAsync(CronJobApiService.deleteCronJob, {
        onSuccess: () => {
            mutate(CronJobApiService.getCronJobs.key());
        },
        onError: (error) => {
            if (error instanceof Error) toast.error("Something went wrong");
        },
    });

    return {
        cronJob,
        createCronJob,
        updateCronJob,
        deleteCronJob,
    };
}
