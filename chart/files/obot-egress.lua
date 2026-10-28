-- Substrate injects this private header; never forward it to an actor's target.
local function authority(value)
  local host, port = string.lower(value):match("^([^:]+):?(%d*)$")
  if not host then return nil end
  return host:gsub("%.$", "") .. ":" .. tostring(tonumber(port) or 80)
end

function envoy_on_request(handle)
  local headers = handle:headers()
  local key = headers:get("x-obot-egress-key")
  headers:remove("x-obot-egress-key")
  if authority(headers:get(":authority") or "") ~= authority(obot_authority) then
    return
  end

  -- Substrate injects the session's LLM key into x-obot-api-key; only the LLM proxy can receive it.
  local path = (headers:get(":path") or ""):match("^[^?]*")
  if not path:match("^/api/llm%-proxy/") or path:find("//", 1, true) or path:find("/.", 1, true) or path:lower():find("%2e", 1, true) then
    headers:remove("x-obot-api-key")
  end
  if not key then
    return
  end

  -- Only MCP connections and the component OAuth check can receive a key.
  -- The OAuth check returns pending authorization links, or an empty list when complete.
  local oauth = headers:get(":method") == "GET" and path:match("^/api/oauth/vmcp/[%w_-]+$")
  if (not path:match("^/mcp%-connect/[%w_/-]+$") and not oauth) or path:find("//", 1, true) then
    handle:respond({[":status"] = "403"}, "Invalid Obot MCP credential destination")
    return
  end
  headers:replace("authorization", key)
  handle:streamInfo():dynamicMetadata():set("obot.mcp", "authorized", true)
  handle:clearRouteCache()
end
