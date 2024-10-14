import { type MetaFunction, redirect, useLoaderData } from "@remix-run/react";
import { $path } from "remix-routes";

export const meta: MetaFunction = () => {
    return [
        { title: "New Remix App" },
        { name: "description", content: "Welcome to Remix!" },
    ];
};

export const clientLoader = async () => {
    throw redirect($path("/agents"));
};

export default function Index() {
    useLoaderData();
}
