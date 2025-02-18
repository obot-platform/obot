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
import { mockedTask } from "test/mocks/models/task";
import { overrideServer } from "test/server";

import { CronJob } from "~/lib/model/cronjobs";
import { EmailReceiver } from "~/lib/model/email-receivers";
import { OAuthApp } from "~/lib/model/oauthApps";
import { EntityList } from "~/lib/model/primitives";
import { Task as TaskModel } from "~/lib/model/tasks";
import { Webhook } from "~/lib/model/webhooks";
import { WorkspaceFile } from "~/lib/model/workspace";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { noop } from "~/lib/utils";

import { Task } from "~/components/task/Task";

describe(Task, () => {
	const setupServer = (task: TaskModel) => {
		const putSpy = vi.fn();
		overrideServer([
			http.get(ApiRoutes.tasks.getById(task.id).path, () => {
				return HttpResponse.json<TaskModel>(mockedTask);
			}),
			http.put(ApiRoutes.tasks.getById(task.id).path, async ({ request }) => {
				const body = await request.json();
				putSpy(body);
				return HttpResponse.json<TaskModel>(mockedTask);
			}),
			http.get(ApiRoutes.agents.getWorkspaceFiles(task.id).path, () => {
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
			...knowledgeHandlers(task.id),
		]);

		return putSpy;
	};

	let putSpy: ReturnType<typeof setupServer>;
	beforeEach(() => {
		putSpy = setupServer(mockedTask);
	});

	afterEach(() => {
		cleanup();
	});

	it.each([
		["name", mockedTask.name, undefined],
		[
			"description",
			mockedTask.description || "Add a description...",
			"placeholder",
		],
	])("Updating %s triggers save", async (field, searchFor, as) => {
		render(<Task task={mockedTask} onPersistThreadId={noop} />);

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
