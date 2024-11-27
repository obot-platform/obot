import { redirect, useLoaderData } from "@remix-run/react";
import { $path } from "remix-routes";

export const clientLoader = async () => {
    throw redirect($path("/agents"));
};

export default function Index() {
    useLoaderData();
}
