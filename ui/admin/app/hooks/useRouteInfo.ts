import { Location, useLocation, useParams } from "@remix-run/react";
import { useMemo } from "react";

import { RouteService } from "~/lib/service/routeService";

const urlFromLocation = (location: Location) => {
    const { pathname, search, hash } = location;
    return new URL(window.location.origin + pathname + search + hash);
};

export function useUrl() {
    const location = useLocation();

    return useMemo(() => urlFromLocation(location), [location]);
}

export function useUnknownPathParams() {
    const url = useUrl();
    const params = useParams();

    return useMemo(
        () => RouteService.getUnknownRouteInfo(url, params),
        [url, params]
    );
}
