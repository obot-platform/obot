import queryString from "query-string";
import { $params, $path, Routes, RoutesWithParams } from "remix-routes";
import { ZodNull, ZodSchema, ZodType, z } from "zod";

const QuerySchemas = {
    agentSchema: z.object({
        threadId: z.string().nullish(),
        from: z.string().nullish(),
    }),
    threadsListSchema: z.object({
        agentId: z.string().nullish(),
        userId: z.string().nullish(),
        workflowId: z.string().nullish(),
        from: z.enum(["workflows", "agents", "users"]).nullish().catch(null),
    }),
    workflowSchema: z.object({
        threadId: z.string().nullish(),
    }),
} as const;

function parseQuery<T extends ZodType>(search: string, schema: T) {
    if (schema instanceof ZodNull) return null;

    const obj = queryString.parse(search);
    const { data, success } = schema.safeParse(obj);

    if (!success) {
        console.error("Failed to parse query params", search);
        return null;
    }

    return data;
}

const exactRegex = (path: string) => new RegExp(`^${path}$`);

type RouteHelper = {
    regex: RegExp;
    path: keyof Routes;
    schema: ZodSchema;
};

export const RouteHelperMap = {
    "": {
        regex: exactRegex($path("")),
        path: "/",
        schema: z.null(),
    },
    "/": {
        regex: exactRegex($path("/")),
        path: "/",
        schema: z.null(),
    },
    "/agents": {
        regex: exactRegex($path("/agents")),
        path: "/agents",
        schema: z.null(),
    },
    "/agents/:agent": {
        regex: exactRegex($path("/agents/:agent", { agent: "(.+)" })),
        path: "/agents/:agent",
        schema: QuerySchemas.agentSchema,
    },
    "/debug": {
        regex: exactRegex($path("/debug")),
        path: "/debug",
        schema: z.null(),
    },
    "/home": {
        regex: exactRegex($path("/home")),
        path: "/home",
        schema: z.null(),
    },
    "/models": {
        regex: exactRegex($path("/models")),
        path: "/models",
        schema: z.null(),
    },
    "/oauth-apps": {
        regex: exactRegex($path("/oauth-apps")),
        path: "/oauth-apps",
        schema: z.null(),
    },
    "/thread/:id": {
        regex: exactRegex($path("/thread/:id", { id: "(.+)" })),
        path: "/thread/:id",
        schema: z.null(),
    },
    "/threads": {
        regex: exactRegex($path("/threads")),
        path: "/threads",
        schema: QuerySchemas.threadsListSchema,
    },
    "/tools": {
        regex: exactRegex($path("/tools")),
        path: "/tools",
        schema: z.null(),
    },
    "/users": {
        regex: exactRegex($path("/users")),
        path: "/users",
        schema: z.null(),
    },
    "/webhooks": {
        regex: exactRegex($path("/webhooks")),
        path: "/webhooks",
        schema: z.null(),
    },
    "/webhooks/create": {
        regex: exactRegex($path("/webhooks/create")),
        path: "/webhooks/create",
        schema: z.null(),
    },
    "/webhooks/:webhook": {
        regex: exactRegex($path("/webhooks/:webhook", { webhook: "(.+)" })),
        path: "/webhooks/:webhook",
        schema: z.null(),
    },
    "/workflows": {
        regex: exactRegex($path("/workflows")),
        path: "/workflows",
        schema: z.null(),
    },
    "/workflows/:workflow": {
        regex: exactRegex($path("/workflows/:workflow", { workflow: "(.+)" })),
        path: "/workflows/:workflow",
        schema: QuerySchemas.workflowSchema,
    },
} satisfies Record<keyof Routes, RouteHelper>;

type QueryInfo<T extends keyof Routes> = z.infer<
    (typeof RouteHelperMap)[T]["schema"]
>;

type PathInfo<T extends keyof RoutesWithParams> = ReturnType<
    typeof $params<T, Routes[T]["params"]>
>;

export type RouteInfo<T extends keyof Routes = keyof Routes> = {
    path: T;
    query: QueryInfo<T> | null;
    pathParams: T extends keyof RoutesWithParams ? PathInfo<T> : unknown;
};

function getRouteHelper(
    url: URL,
    params: Record<string, string | undefined>
): RouteInfo | null {
    for (const route of Object.values(RouteHelperMap)) {
        if (route.regex.test(url.pathname))
            return {
                path: route.path,
                query: parseQuery(url.search, route.schema as ZodSchema),
                pathParams: $params(
                    route.path as keyof RoutesWithParams,
                    params
                ),
            };
    }

    return null;
}

function getRouteInfo<T extends keyof Routes>(
    path: T,
    url: URL,
    params: Record<string, string | undefined>
): RouteInfo<T> {
    const helper = RouteHelperMap[path];

    return {
        path,
        query: parseQuery(url.search, helper.schema),
        pathParams: $params(path as keyof RoutesWithParams, params) as Todo,
    };
}

function getUnknownRouteInfo(
    url: URL,
    params: Record<string, string | undefined>
) {
    const routeInfo = getRouteHelper(url, params);

    switch (routeInfo?.path) {
        case "/":
            return routeInfo as RouteInfo<"/">;
        case "/agents":
            return routeInfo as RouteInfo<"/agents">;
        case "/agents/:agent":
            return routeInfo as RouteInfo<"/agents/:agent">;
        case "/debug":
            return routeInfo as RouteInfo<"/debug">;
        case "/home":
            return routeInfo as RouteInfo<"/home">;
        case "/models":
            return routeInfo as RouteInfo<"/models">;
        case "/oauth-apps":
            return routeInfo as RouteInfo<"/oauth-apps">;
        case "/thread/:id":
            return routeInfo as RouteInfo<"/thread/:id">;
        case "/threads":
            return routeInfo as RouteInfo<"/threads">;
        case "/tools":
            return routeInfo as RouteInfo<"/tools">;
        case "/users":
            return routeInfo as RouteInfo<"/users">;
        case "/webhooks":
            return routeInfo as RouteInfo<"/webhooks">;
        case "/webhooks/create":
            return routeInfo as RouteInfo<"/webhooks/create">;
        case "/webhooks/:webhook":
            return routeInfo as RouteInfo<"/webhooks/:webhook">;
        case "/workflows":
            return routeInfo as RouteInfo<"/workflows">;
        case "/workflows/:workflow":
            return routeInfo as RouteInfo<"/workflows/:workflow">;
        default:
            return null;
    }
}

export type RouteQueryParams<T extends keyof typeof QuerySchemas> = z.infer<
    (typeof QuerySchemas)[T]
>;

const getQueryParams = <T extends keyof Routes>(path: T, search: string) =>
    parseQuery(search, RouteHelperMap[path].schema) as RouteInfo<T>["query"];

export const RouteService = {
    schemas: QuerySchemas,
    getUnknownRouteInfo,
    getRouteInfo,
    getQueryParams,
};
