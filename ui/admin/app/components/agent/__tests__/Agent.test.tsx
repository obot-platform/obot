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
import { defaultModelAliasHandler } from "test/mocks/handlers/defaultModelAliases";
import { knowledgeHandlers } from "test/mocks/handlers/knowledge";
import { toolsHandlers } from "test/mocks/handlers/tools";
import { mockedAgent } from "test/mocks/models/agents";
import {
	mockedBrowserToolBundle,
	mockedImageToolBundle,
} from "test/mocks/models/toolReferences";
import { mockedUsers } from "test/mocks/models/users";
import { overrideServer } from "test/server";

import { Agent as AgentModel } from "~/lib/model/agents";
import { Assistant } from "~/lib/model/assistants";
import { OAuthApp } from "~/lib/model/oauthApps";
import { EntityList } from "~/lib/model/primitives";
import { Thread } from "~/lib/model/threads";
import { User } from "~/lib/model/users";
import { WorkspaceFile } from "~/lib/model/workspace";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

import { Agent } from "~/components/agent/Agent";
import { AgentProvider } from "~/components/agent/AgentContext";

describe(Agent, () => {
	const setupServer = (agent: AgentModel) => {
		const putSpy = vi.fn();
		overrideServer([
			http.get(ApiRoutes.agents.getById(agent.id).path, () => {
				return HttpResponse.json<AgentModel>(agent);
			}),
			http.put(ApiRoutes.agents.getById(agent.id).path, async ({ request }) => {
				const body = await request.json();
				putSpy(body);
				return HttpResponse.json<AgentModel>(agent);
			}),
			http.get(ApiRoutes.agents.getWorkspaceFiles(agent.id).path, () => {
				return HttpResponse.json<EntityList<WorkspaceFile>>({
					items: [],
				});
			}),
			http.get(ApiRoutes.assistants.getAssistants().path, () => {
				return HttpResponse.json<EntityList<Assistant>>({
					items: [],
				});
			}),
			http.get(ApiRoutes.users.base().path, () => {
				return HttpResponse.json<EntityList<User>>({
					items: mockedUsers,
				});
			}),
			http.get(ApiRoutes.threads.getByAgent(agent.id).path, () => {
				return HttpResponse.json<EntityList<Thread> | null>({
					items: null,
				});
			}),
			http.get(ApiRoutes.oauthApps.getOauthApps().url, () => {
				return HttpResponse.json<EntityList<OAuthApp>>({
					items: [],
				});
			}),
			defaultModelAliasHandler,
			...knowledgeHandlers(agent.id),
			...toolsHandlers,
		]);

		return putSpy;
	};

	afterEach(() => {
		cleanup();
		vi.clearAllMocks();
	});

	it.each([
		["name", mockedAgent.name, undefined],
		[
			"description",
			mockedAgent.description || "Add a description...",
			"placeholder",
		],
		["prompt", "Instructions", "textbox", 2],
		["introductionMessage", "Introductions", "textbox"],
	])("Updating %s triggers save", async (field, searchFor, as, index = 0) => {
		const putSpy = setupServer(mockedAgent);
		render(
			<AgentProvider agent={mockedAgent}>
				<Agent />
			</AgentProvider>
		);

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

			await userEvent.type(textbox, modifiedValue, { delay: 0 });
		}

		await waitFor(
			() =>
				expect(putSpy).toHaveBeenCalledWith(
					expect.objectContaining({
						[field]: expect.stringContaining(modifiedValue),
					})
				),
			{ timeout: 1000 }
		);
	});

	it("Updating icon triggers save", async () => {
		const putSpy = setupServer(mockedAgent);
		render(
			<AgentProvider agent={mockedAgent}>
				<Agent />
			</AgentProvider>
		);

		const title = screen.getByDisplayValue(mockedAgent.name);
		const iconButton = within(
			title.parentElement!.parentElement!.parentElement!.parentElement!
		).getByRole("button");
		// https://github.com/radix-ui/primitives/issues/856#issuecomment-2141002364
		// note: experience oddity with ShadCN MenuDropdown interaction,
		// skipping hover on click resolved the menu not opening.
		await userEvent.click(iconButton, { pointerEventsCheck: 0 });

		const selectIconMenuItem = await screen.findByText(/Select Icon/i);
		await userEvent.click(selectIconMenuItem, { pointerEventsCheck: 0 });

		await waitFor(() => expect(screen.getAllByRole("menu")).toHaveLength(2));

		const iconSelections = await screen.findAllByAltText(/Agent Icon/);
		const iconSrc = iconSelections[0].getAttribute("src");
		await userEvent.click(iconSelections[0]);

		await waitFor(() => expect(putSpy).toHaveBeenCalled());

		expect(putSpy).toHaveBeenCalledWith(
			expect.objectContaining({
				icons: expect.objectContaining({
					icon: iconSrc,
				}),
			})
		);
	});

	it("Deleting a tool deletes the tool", async () => {
		const mockedAgentWithTools: AgentModel = {
			...mockedAgent,
			tools: [mockedImageToolBundle[0].id, mockedBrowserToolBundle[0].id],
			toolInfo: {
				[mockedImageToolBundle[0].id]: {
					authorized: true,
				},
				[mockedBrowserToolBundle[0].id]: {
					authorized: true,
				},
			},
		};

		const putSpy = setupServer(mockedAgentWithTools);
		render(
			<AgentProvider agent={mockedAgentWithTools}>
				<Agent />
			</AgentProvider>
		);

		const toolHeader = screen.getByRole("heading", { name: "Tools" });
		const browserTool = await within(toolHeader.parentElement!).findByAltText(
			mockedBrowserToolBundle[0].name
		);
		const imageTool = await within(toolHeader.parentElement!).findByAltText(
			mockedImageToolBundle[0].name
		);

		expect(browserTool).toBeInTheDocument();
		expect(imageTool).toBeInTheDocument();

		// deleting images-bundle tool
		const imageDeleteButton = within(
			imageTool.parentElement!.parentElement!
		).getAllByRole("button")[1];
		await userEvent.click(imageDeleteButton);

		await waitFor(() => expect(putSpy).toHaveBeenCalled());

		expect(putSpy).toHaveBeenCalledWith(
			expect.objectContaining({
				tools: [mockedBrowserToolBundle[0].id],
			})
		);
	});
});
