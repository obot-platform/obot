import { faker } from "@faker-js/faker";
import {
	HttpResponse,
	cleanup,
	http,
	render,
	screen,
	userEvent,
	waitFor,
	within,
} from "test";
import { overrideServer } from "test/server";

import { mockedDefaultModelAliases } from "~/lib/model/__mocks__/defaultModelAliases";
import {
	mockedDatabaseToolReference,
	mockedKnowledgeToolReference,
	mockedTasksToolReference,
	mockedToolReferences,
	mockedWorkspaceFilesToolReference,
} from "~/lib/model/__mocks__/toolReferences";
import { mockedWorkflow } from "~/lib/model/__mocks__/workflow";
import { CronJob } from "~/lib/model/cronjobs";
import { DefaultModelAlias } from "~/lib/model/defaultModelAliases";
import { EmailReceiver } from "~/lib/model/email-receivers";
import { KnowledgeFile, KnowledgeSource } from "~/lib/model/knowledge";
import { EntityList } from "~/lib/model/primitives";
import { ToolReference } from "~/lib/model/toolReferences";
import { Webhook } from "~/lib/model/webhooks";
import { Workflow as WorkflowModel } from "~/lib/model/workflows";
import { WorkspaceFile } from "~/lib/model/workspace";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { noop } from "~/lib/utils";

import { Workflow } from "~/components/workflow/Workflow";

describe(Workflow, () => {
	const toolReferences = {
		database: mockedDatabaseToolReference,
		knowledge: mockedKnowledgeToolReference,
		tasks: mockedTasksToolReference,
		"workspace-files": mockedWorkspaceFilesToolReference,
	};

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
			...Object.entries(toolReferences).map(([id, toolReference]) =>
				http.get(ApiRoutes.toolReferences.getById(id).path, () => {
					return HttpResponse.json<ToolReference>(toolReference);
				})
			),
			http.get(ApiRoutes.toolReferences.base({ type: "tool" }).path, () => {
				return HttpResponse.json<EntityList<ToolReference>>({
					items: mockedToolReferences,
				});
			}),
			http.get(ApiRoutes.defaultModelAliases.getAliases().path, () => {
				return HttpResponse.json<EntityList<DefaultModelAlias>>({
					items: mockedDefaultModelAliases,
				});
			}),
			http.get(
				ApiRoutes.knowledgeFiles.getKnowledgeFiles("agents", workflow.id).path,
				() => {
					return HttpResponse.json<EntityList<KnowledgeFile>>({
						items: [],
					});
				}
			),
			http.get(
				ApiRoutes.knowledgeSources.getKnowledgeSources("agents", workflow.id)
					.path,
				() => {
					return HttpResponse.json<EntityList<KnowledgeSource> | null>({
						items: null,
					});
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
		["prompt", "Instructions", "textbox", 2],
	])("Updating %s triggers save", async (field, searchFor, as, index = 0) => {
		render(<Workflow workflow={mockedWorkflow} onPersistThreadId={noop} />);

		const modifiedValue = faker.word.words({ count: { min: 2, max: 5 } });

		if (!as) {
			await userEvent.type(screen.getByDisplayValue(searchFor), modifiedValue);
		} else if (as === "placeholder") {
			await userEvent.type(
				screen.getByPlaceholderText(searchFor),
				modifiedValue
			);
		} else if (as === "textbox") {
			const heading = screen.getByRole("heading", { name: searchFor });
			const textbox = within(heading.parentElement!).queryAllByRole("textbox")[
				index ?? 0
			];

			await userEvent.type(textbox, modifiedValue);
		}

		await waitFor(() => screen.getByText(/Saving|Saved/i));

		expect(putSpy).toHaveBeenCalledWith(
			expect.objectContaining({
				[field]: expect.stringContaining(modifiedValue),
			})
		);
	});
});
