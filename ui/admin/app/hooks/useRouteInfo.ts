import { useCallback, useMemo } from "react";
import {
	Location,
	useLocation,
	useParams,
	useSearchParams,
} from "react-router";
import { Routes } from "safe-routes";

import { QueryInfo, RouteService } from "~/lib/service/routeService";

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

export function useQueryInfo<T extends keyof Routes>(route: T) {
	const [searchParams, setSearchParams] = useSearchParams();

	const params = useMemo(
		() => RouteService.getQueryParams(route, searchParams.toString()),
		[route, searchParams]
	);

	const update = useCallback(
		<TKey extends keyof QueryInfo<T>>(
			param: TKey,
			value: QueryInfo<T>[TKey]
		) => {
			setSearchParams((prev) => {
				prev.set(param as string, String(value));
				return prev;
			});
		},
		[setSearchParams]
	);

	const remove = useCallback(
		(param: keyof QueryInfo<T>) => {
			setSearchParams(
				(prev) => {
					prev.delete(param as string);
					return prev;
				},
				{ replace: true }
			);
		},
		[setSearchParams]
	);

	return { params, update, remove };
}
