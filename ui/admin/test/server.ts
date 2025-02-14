import { RequestHandler } from "msw";
import { setupServer } from "msw/node";
import { defaultMockedHandlers } from "test/mocks/handlers/default";

// Setup requests interception using the given handlers
export const server = setupServer(...defaultMockedHandlers);
export const overrideServer = (handlers: RequestHandler[]) => {
	server.use(...handlers, ...defaultMockedHandlers);
};
