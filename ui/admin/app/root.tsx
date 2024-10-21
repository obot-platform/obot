import type { LinksFunction } from "@remix-run/node";
import {
    Links,
    Meta,
    Outlet,
    Scripts,
    ScrollRestoration,
} from "@remix-run/react";
import { SWRConfig } from "swr";

import { AuthProvider } from "~/components/auth/AuthContext";
import { LayoutProvider } from "~/components/layout/LayoutProvider";
import { ThemeProvider } from "~/components/theme";
import { Toaster } from "~/components/ui/sonner";

import { LoadingSpinner } from "./components/ui/LoadingSpinner";
import "./tailwind.css";

export const links: LinksFunction = () => [
    { rel: "preconnect", href: "https://fonts.googleapis.com" },
    {
        rel: "preconnect",
        href: "https://fonts.gstatic.com",
        crossOrigin: "anonymous",
    },
    {
        rel: "stylesheet",
        href: "https://fonts.googleapis.com/css2?family=Inter:ital,opsz,wght@0,14..32,100..900;1,14..32,100..900&display=swap",
    },
];

export function Layout({ children }: { children: React.ReactNode }) {
    return (
        <html lang="en">
            <head>
                <meta charSet="utf-8" />
                <meta
                    name="viewport"
                    content="width=device-width, initial-scale=1"
                />
                <link rel="icon shortcut icon apple-touch-icon" href="/admin/favicon.ico" />
                <Meta />
                <Links />
            </head>
            <body>
                {children}
                <Toaster closeButton />
                <ScrollRestoration />
                <Scripts />
            </body>
        </html>
    );
}

export default function App() {
    return (
        <SWRConfig value={{ revalidateOnFocus: false }}>
            <AuthProvider>
                <ThemeProvider>
                    <LayoutProvider>
                        <Outlet />
                    </LayoutProvider>
                </ThemeProvider>
            </AuthProvider>
        </SWRConfig>
    );
}

export function HydrateFallback() {
    return (
        <div className="flex min-h-screen w-full items-center justify-center p-4">
            <LoadingSpinner />
        </div>
    );
}
