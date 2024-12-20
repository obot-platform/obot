import { redirect, useLoaderData } from "react-router";
import { $path } from "safe-routes";

export const clientLoader = async () => {
    throw redirect($path("/agents"));
};

export default function Index() {
    useLoaderData();
}
