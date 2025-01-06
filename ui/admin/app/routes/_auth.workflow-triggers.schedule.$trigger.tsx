import {
    ClientLoaderFunctionArgs,
    MetaFunction,
    useLoaderData,
    useMatch,
} from "react-router";
import useSWR, { preload } from "swr";

import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteService } from "~/lib/service/routeService";

import { ScheduleForm } from "~/components/workflow-triggers/ScheduleForm";

export async function clientLoader({
    request,
    params,
}: ClientLoaderFunctionArgs) {
    const { pathParams } = RouteService.getRouteInfo(
        "/workflow-triggers/schedule/:trigger",
        new URL(request.url),
        params
    );

    await preload(
        CronJobApiService.getCronJobById.key(pathParams.trigger),
        () => CronJobApiService.getCronJobById(pathParams.trigger)
    );

    return { cronJobId: pathParams.trigger };
}

export default function SchedulePage() {
    const { cronJobId } = useLoaderData<typeof clientLoader>();
    const { data: cronjob } = useSWR(
        CronJobApiService.getCronJobById.key(cronJobId),
        ({ cronJobId }) => CronJobApiService.getCronJobById(cronJobId)
    );

    return <ScheduleForm cronjob={cronjob} />;
}

const ScheduleBreadcrumb = () => {
    const match = useMatch("/workflow-triggers/schedule/:trigger");

    return match?.params?.trigger || "Edit";
};

export const handle: RouteHandle = {
    breadcrumb: () => [{ content: <ScheduleBreadcrumb /> }],
};

export const meta: MetaFunction<typeof clientLoader> = ({ data }) => {
    return [{ title: `Schedule • ${data?.cronJobId}` }];
};
