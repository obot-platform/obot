import { MetaFunction } from "react-router";

import { RouteHandle } from "~/lib/service/routeHandles";

import { ScheduleForm } from "~/components/workflow-triggers/ScheduleForm";

export default function CreateSchedulePage() {
	return <ScheduleForm />;
}

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Create Schedule" }],
};

export const meta: MetaFunction = () => {
	return [{ title: "Create Schedule" }];
};
