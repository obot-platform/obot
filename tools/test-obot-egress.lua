-- Run after chart/files/obot-egress.lua with obot_authority set to obot.obot.svc:8080.
local function check(host, path, key, want_status, want_auth)
  local values = {[":authority"] = host, [":path"] = path,
                  ["x-obot-egress-key"] = key, authorization = "actor-supplied"}
  local metadata = {}
  local headers = {
    get = function(_, name) return values[name] end,
    remove = function(_, name) values[name] = nil end,
    replace = function(_, name, value) values[name] = value end
  }
  local result
  local handle = {
    headers = function() return headers end,
    respond = function(_, h) result = h[":status"] end,
    streamInfo = function() return {dynamicMetadata = function() return {
      set = function(_, namespace, field, value) metadata[namespace .. "." .. field] = value end
    } end} end,
    clearRouteCache = function() end
  }
  envoy_on_request(handle)
  assert(values["x-obot-egress-key"] == nil, "private key header escaped")
  assert(result == want_status, "unexpected HTTP status for " .. path)
  assert(values.authorization == (want_auth or "actor-supplied"), "credential escaped its destination")
  assert((metadata["obot.mcp.authorized"] == true) == (want_auth ~= nil), "upstream was not pinned")
end
check("obot.obot.svc:8080", "/mcp-connect/vmcp_one", "Bearer secret", nil, "Bearer secret")
check("OBOT.OBOT.SVC.:8080", "/mcp-connect/vmcp_one?q=1", "Bearer secret", nil, "Bearer secret")
check("obot.obot.svc:8080", "/mcp-connect/vmcp_one/message?q=1", "Bearer secret", nil, "Bearer secret")
check("obot.obot.svc:8080", "/mcp-connect//vmcp_one", "Bearer secret", "403", nil)
check("other.example:8080", "/mcp-connect/vmcp_one", "Bearer secret", nil, nil)
check("obot.obot.svc:8081", "/mcp-connect/vmcp_one", "Bearer secret", nil, nil)
check("obot.obot.svc:8080", "/api/claude-runners/credentials", "Bearer secret", "403", nil)
check("obot.obot.svc:8080", "/mcp-connect/vmcp_one/../../echo", "Bearer secret", "403", nil)
check("obot.obot.svc:8080", "/mcp-connect/%2e%2e", "Bearer secret", "403", nil)
check("obot.obot.svc:8080", "/mcp-connect/vmcp_one", nil, nil, nil)
