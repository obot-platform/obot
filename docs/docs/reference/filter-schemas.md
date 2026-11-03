---
description: MCP filter tool and HTTP webhook contracts.
title: "Filter schemas"
---

Filters receive MCP messages and return an enforcement decision. Choose the contract that matches the implementation you are deploying.

| Format | Reference |
|---|---|
| MCP filter tool request/response | [Filter tool contract](../functionality/filters.md#filter-tool-contract) |
| AuthZEN authorization request and decision | [Optional authorization callback](../functionality/filters.md#authzen-authorization) |
| HTTP webhook payload and signature | [HTTP filters](../functionality/filters.md#http-based-filters) |

An MCP filter tool returns the decision fields described in its contract. An HTTP webhook uses HTTP status codes and can verify the configured request signature. Do not interchange these response formats.

For the ordering of authorization, request filtering, tool execution, and response filtering, see the [request pipeline](../functionality/filters.md#request-pipeline). For deployment and traffic selectors, see [Filter MCP traffic](../functionality/filters.md).
