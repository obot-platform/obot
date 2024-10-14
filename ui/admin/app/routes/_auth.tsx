import { Outlet } from "@remix-run/react";

import { HeaderNav } from "~/components/header/HeaderNav";
import { Sidebar } from "~/components/sidebar";

export default function AuthLayout() {
    return (
        <div className="flex flex-col h-screen w-screen">
            <HeaderNav />

            <div className="flex flex-auto overflow-y-hidden">
                <Sidebar className="flex-shrink-0" />

                <div className="flex-grow overflow-auto">
                    <Outlet />
                </div>
            </div>
        </div>
    );
}
