import {
	overrideServer,
	render,
	screen,
	userEvent,
	waitFor,
	within,
} from "test";
import { toolsHandlers } from "test/mocks/handlers/tools";

import { noop } from "~/lib/utils/noop";

import { ToolCatalog } from "~/components/tools/ToolCatalog";

describe(ToolCatalog, () => {
	beforeEach(() => {
		overrideServer(toolsHandlers);

		// Mock scrollIntoView
		Element.prototype.scrollIntoView = vi.fn();
	});
	it("No setup option in tooltip if tool does not need oauth configuration", async () => {
		render(<ToolCatalog tools={[]} oauths={[]} onUpdateTools={noop} />);

		const tool = await screen.findByText("Browser");
		await userEvent.hover(tool);
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toBeInTheDocument()
		);

		expect(screen.queryByText("Setup")).not.toBeInTheDocument();
	});
	it("Clicking setup for a tool that needs oauth configuration opens the setup dialog", async () => {
		render(<ToolCatalog tools={[]} oauths={[]} onUpdateTools={noop} />);

		const tool = await screen.findByText("Gmail");
		await userEvent.hover(tool);
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toBeInTheDocument()
		);

		const setupButton = await within(screen.getByRole("tooltip")).findByText(
			"Setup"
		);
		await userEvent.click(setupButton);
		await waitFor(() =>
			expect(screen.getByText("Configure Google OAuth App")).toBeInTheDocument()
		);
	});
});
