import * as primitive from "@testing-library/react";
import { setupServer } from "msw/node";
import { ReactElement } from "react";
import { MemoryRouter } from "react-router";
import { SWRConfig } from "swr";

import { AuthProvider } from "~/components/auth/AuthContext";
import { ThemeProvider } from "~/components/theme";
import { TooltipProvider } from "~/components/ui/tooltip";

const { render: _render, ...rest } = primitive;

const AllTheProviders = ({ children }: { children: React.ReactNode }) => {
	return (
		<MemoryRouter>
			<SWRConfig value={{ revalidateOnFocus: false }}>
				<AuthProvider>
					<ThemeProvider>
						<TooltipProvider>{children}</TooltipProvider>
					</ThemeProvider>
				</AuthProvider>
			</SWRConfig>
		</MemoryRouter>
	);
};

export const mockServer = setupServer();

export const render = (
	ui: ReactElement,
	options?: Omit<primitive.RenderOptions, "wrapper">
) => _render(ui, { wrapper: AllTheProviders, ...options });

export const { screen, waitFor, within, act, cleanup, configure, prettyDOM } =
	rest;

export { default as userEvent } from "@testing-library/user-event";

export * from "test/server";

export { http, HttpResponse } from "msw";
