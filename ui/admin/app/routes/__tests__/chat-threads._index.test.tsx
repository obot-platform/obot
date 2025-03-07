import {
	HttpResponse,
	http,
	overrideServer,
	render,
	screen,
	within,
} from "test";
import { mockedAgent } from "test/mocks/models/agents";
import { mockedProject } from "test/mocks/models/projects";
import { mockedThreads } from "test/mocks/models/threads";
import { mockedUsers } from "test/mocks/models/users";

import { Agent } from "~/lib/model/agents";
import { EntityList } from "~/lib/model/primitives";
import { Project } from "~/lib/model/project";
import { Thread } from "~/lib/model/threads";
import { User } from "~/lib/model/users";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

import ChatThreads from "~/routes/_auth.chat-threads._index";

vi.mock("react-router", async () => {
	const actual = await vi.importActual("react-router");
	return {
		...actual,
		useLoaderData: vi.fn(() => ({
			// Mock the loader data that matches clientLoader return type
			agentId: undefined,
			userId: undefined,
		})),
		useNavigate: vi.fn(() => vi.fn()),
		useSearchParams: vi.fn(() => [new URLSearchParams(), vi.fn()]),
	};
});

describe(ChatThreads, () => {
	beforeEach(() => {
		overrideServer([
			http.get(ApiRoutes.threads.base().url, () => {
				return HttpResponse.json<EntityList<Thread>>({
					items: mockedThreads,
				});
			}),
			http.get(ApiRoutes.agents.base().url, () => {
				return HttpResponse.json<EntityList<Agent>>({
					items: [mockedAgent],
				});
			}),
			http.get(ApiRoutes.users.base().url, () => {
				return HttpResponse.json<EntityList<User>>({
					items: mockedUsers,
				});
			}),
			http.get(ApiRoutes.projects.getAll().url, () => {
				return HttpResponse.json<EntityList<Project>>({
					items: [mockedProject],
				});
			}),
		]);
	});

	it("Displays user email for an agent thread on initial render", async () => {
		render(<ChatThreads />);
		const groups = await screen.findAllByRole("rowgroup");
		const tableBody = groups[1]; // tbody
		const cells = await within(tableBody).findAllByRole("cell"); // td
		expect(cells[1]).toHaveTextContent(mockedUsers[0].email);
	});
});
