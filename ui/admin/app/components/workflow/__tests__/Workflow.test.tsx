import { faker } from "@faker-js/faker";
import {
	HttpResponse,
	cleanup,
	http,
	render,
	screen,
	userEvent,
	waitFor,
} from "test";
import { defaultModelAliasHandler } from "test/mocks/handlers/defaultModelAliases";
import { knowledgeHandlers } from "test/mocks/handlers/knowledge";
import { toolsHandlers } from "test/mocks/handlers/tools";
import { mockedWorkflow } from "test/mocks/models/workflow";
import { overrideServer } from "test/server";

import { CronJob } from "~/lib/model/cronjobs";
import { EmailReceiver } from "~/lib/model/email-receivers";
import { OAuthApp } from "~/lib/model/oauthApps";
import { EntityList } from "~/lib/model/primitives";
import { Webhook } from "~/lib/model/webhooks";
import { Workflow as WorkflowModel } from "~/lib/model/workflows";
import { WorkspaceFile } from "~/lib/model/workspace";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { noop } from "~/lib/utils";

import { Workflow } from "~/components/workflow/Workflow";

describe(Workflow, () => {
	const setupServer = (workflow: WorkflowModel) => {
		const putSpy = vi.fn();
		overrideServer([
			http.get(ApiRoutes.workflows.getById(workflow.id).path, () => {
				return HttpResponse.json<WorkflowModel>(mockedWorkflow);
			}),
			http.put(
				ApiRoutes.workflows.getById(workflow.id).path,
				async ({ request }) => {
					const body = await request.json();
					putSpy(body);
					return HttpResponse.json<WorkflowModel>(mockedWorkflow);
				}
			),
			http.get(ApiRoutes.agents.getWorkspaceFiles(workflow.id).path, () => {
				return HttpResponse.json<EntityList<WorkspaceFile>>({
					items: [],
				});
			}),
			http.get(ApiRoutes.cronjobs.getCronJobs().path, () => {
				return HttpResponse.json<EntityList<CronJob>>({
					items: [],
				});
			}),
			http.get(ApiRoutes.emailReceivers.getEmailReceivers().path, () => {
				return HttpResponse.json<EntityList<EmailReceiver>>({
					items: [],
				});
			}),
			http.get(ApiRoutes.webhooks.getWebhooks().path, () => {
				return HttpResponse.json<EntityList<Webhook>>({
					items: [],
				});
			}),
			http.get(ApiRoutes.oauthApps.getOauthApps().url, () => {
				return HttpResponse.json<EntityList<OAuthApp>>({
					items: [],
				});
			}),
			defaultModelAliasHandler,
			...toolsHandlers,
			...knowledgeHandlers(workflow.id),
		]);

		return putSpy;
	};

	let putSpy: ReturnType<typeof setupServer>;
	beforeEach(() => {
		putSpy = setupServer(mockedWorkflow);
	});

	afterEach(() => {
		cleanup();
	});

	it.each([
		["name", mockedWorkflow.name, undefined],
		[
			"description",
			mockedWorkflow.description || "Add a description...",
			"placeholder",
		],
	])("Updating %s triggers save", async (field, searchFor, as) => {
		render(<Workflow workflow={mockedWorkflow} onPersistThreadId={noop} />);

		const modifiedValue = faker.word.words({ count: { min: 2, max: 5 } });

		if (!as) {
			await userEvent.type(screen.getByDisplayValue(searchFor), modifiedValue);
		} else if (as === "placeholder") {
			await userEvent.type(
				screen.getByPlaceholderText(searchFor),
				modifiedValue
			);
		}

		await waitFor(() => screen.getByText(/Saving|Saved/i));

		expect(putSpy).toHaveBeenCalledWith(
			expect.objectContaining({
				[field]: expect.stringContaining(modifiedValue),
			})
		);
	});
});
