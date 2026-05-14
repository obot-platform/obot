---
title: Filters
---

## Overview

Filters are a powerful mechanism for inspecting and controlling tool calls and their results in the MCP Gateway. They provide administrators with the ability to implement custom validation, logging, security checks, or other business logic by intercepting tool requests and responses before they are processed.

Filters can be implemented in two ways:

- **MCP filter servers**: MCP servers that expose a filter tool. Obot deploys and calls the configured tool when matching MCP traffic is processed.
- **HTTP webhook filters**: HTTP endpoints that receive MCP messages from the gateway.

When you configure a filter, you can narrow when it runs using selectors that target particular tool calls or MCP (Model Context Protocol) tool functions.

## How Filters Work

1. **MCP Request Interception**: When a request is made to an MCP server, the gateway intercepts it and sends the details to your configured filter
2. **Payload Inspection**: Your filter receives the payload and can perform any custom logic or validation
3. **Response Decision**: Your filter returns one of the following decisions:
   - Accept: Allow the tool call to proceed
   - Reject: Block execution and return an error to the user
   - Mutate: Return a modified MCP message, if mutation is allowed for the filter

## Gateway Configuration

Filters can be configured in Obot as HTTP webhooks, MCP servers, or by selecting one of the built-in filters. See below for the configuration setup for each type.

### Selectors

All filter types support selectors to control when your filter is triggered:

- **Specific MCP Tool Call Methods**: Target particular tools or functions
- **MCP Tool Names, URIS**: Choose which MCP servers the filter applies to

## MCP Filter Servers

MCP filters can be deployed as MCP servers. Their deployment configuration is similar to other MCP servers in Obot: choose a runtime such as `remote`, `containerized`, `npx`, or `uvx`, then provide the runtime-specific configuration and any required environment variables.

The additional requirement for an MCP filter server is a filter tool name. Obot needs this value so it knows which tool to call when the filter runs.

### Filter Tool Contract

When you write an MCP server that acts as a filter, implement the tool called by `filterConfig.toolName` with this contract:

1. Obot calls one tool on the MCP server for filtering.
2. The tool accepts the full MCP message as an argument.
3. The tool returns a response with these fields:

| Field | Description |
|-------|-------------|
| `accept` | Whether the MCP message should be allowed to continue. |
| `mutated` | Whether the filter returned a modified MCP message. Use this only when the filter is allowed to mutate traffic. |
| `message` | The MCP message to use after filtering. For non-mutating filters, return the original message. |
| `reason` | Human-readable explanation for the decision. Include this when rejecting or mutating a message so administrators can understand the result. |

See the [obot-platform/pii-filter](https://github.com/obot-platform/pii-filter) repository for an example MCP filter server.

## Built-in Filters

Obot ships with a default set of built-in filters. These are MCP filter servers that are already configured for deployment in Obot through the system MCP catalog.

The default built-in filter catalog is maintained in the [obot-platform/system-mcp-catalog](https://github.com/obot-platform/system-mcp-catalog) repository.

## HTTP-based Filters

You can also use HTTP-based webhooks for filtering. In this case, the HTTP server would have to be deployed outside of Obot. You can then provide the following information to Obot:

### Required Configuration

- **Name**: A descriptive name for your filter
- **URL**: The webhook endpoint URL where the gateway will send payloads
- **Secret** (optional): A shared secret with the webhook receiver for payload signature verification

### Security with Secrets

If you configure a secret, the gateway will sign each payload using this shared secret. This allows both sides (the gateway and your webhook service) to verify the authenticity of the communication:

- The gateway signs outgoing payloads with the secret
- Your webhook service can verify the signature to ensure the payload is legitimate
- This prevents unauthorized or tampered requests from being processed

### Webhook Receiver

To implement a filter, you need to create a web service that can handle POST requests from the gateway.

### Payload Structure

The webhook will receive a payload with this data structure:

```python
# Pydantic Model
class WebhookMessage(BaseModel):
    """JSON-RPC message structure for webhook payloads."""
    jsonrpc: str
    id: Optional[Union[str, int]] = None
    method: Optional[str] = None
    params: Optional[Dict[str, Any]] = None
    result: Optional[Dict[str, Any]] = None
    error: Optional[Dict[str, Any]] = None
```

- **Signature Header**: Used for verifying the payload authenticity (when secrets are configured)

`X-Obot-Signature-256`

### Response Codes

Your webhook service should respond with:

- **HTTP 200**: Accept the tool call - allows execution to proceed
- **Non-200 codes**: Reject the tool call - blocks execution and may return an error to the user

### Example

This Python example inspects the search tools query param in the DuckDuckGo MCP Server.

To run this example, execute the following once you have saved the script in `simple_webhook_example.py`:

```bash
uv add fastapi uvicorn pydantic
PORT=8000 WEBHOOK_SECRET=somethingsecret uv run simple_webhook_example.py
```

The filter target url will be `http://<host>:8000/webhook`

The Webhook Secret will also need to be configured in the gateway.

```python
#!/usr/bin/env python3
"""
Simple webhook endpoint with signature validation

Usage:
    uv run simple_webhook_example.py

Environment Variables:
    WEBHOOK_SECRET: Secret for webhook signature validation (default: "test_secret")
    PORT: Port to run the server on (default: 8000)
"""

import hmac
import hashlib
import os
import logging
from typing import Dict, Any, List, Optional, Union
from fastapi import FastAPI, Request, HTTPException, Header
from pydantic import BaseModel

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def validate_signature(body: bytes, signature: str, secret: str) -> bool:
    """
    Validate HMAC-SHA256 signature for webhook security.
    
    Args:
        body: Raw request payload
        signature: Signature from X-Obot-Signature-256 header
        secret: Shared secret for validation
    
    Returns:
        True if signature is valid, False otherwise
    """
    # Remove "sha256=" prefix if present
    if signature.startswith("sha256="):
        signature = signature[7:]
    
    # Calculate expected signature
    expected = hmac.new(
        secret.encode('utf-8'), 
        body, 
        hashlib.sha256
    ).hexdigest()
    
    # Secure comparison
    return hmac.compare_digest(signature, expected)




class WebhookMessage(BaseModel):
    """JSON-RPC message structure for webhook payloads."""
    jsonrpc: str
    id: Optional[Union[str, int]] = None
    method: Optional[str] = None
    params: Optional[Dict[str, Any]] = None
    result: Optional[Dict[str, Any]] = None
    error: Optional[Dict[str, Any]] = None


# Initialize app
app = FastAPI(title="Simple Webhook with Filtering")

# Configuration
SECRET = os.getenv("WEBHOOK_SECRET", "test_secret")
PORT = int(os.getenv("PORT", "8000"))


@app.post("/webhook")
async def webhook_endpoint(
    request: Request,
    x_obot_signature_256: str = Header(alias="X-Obot-Signature-256")
):
    """
    Main webhook endpoint that validates signatures and processes messages.
    """
    body = await request.body()
    
    # Log the incoming request
    logger.info(f"📥 Webhook called - Method: {request.method}, URL: {request.url}")
    logger.info(f"📄 Request body: {body.decode('utf-8', errors='replace')}")
    logger.info(f"🔐 Signature header: {x_obot_signature_256}")
    
    # Validate signature
    if not validate_signature(body, x_obot_signature_256, SECRET):
        logger.error("❌ Invalid webhook signature")
        raise HTTPException(status_code=401, detail="Invalid signature")
    
    try:
        # Parse message
        message = WebhookMessage.model_validate_json(body)
        logger.info(f"✅ Processing {message.method} message")
        
        # Check all requests for suspicious content
        check_message_for_threats(message)
        
        logger.info(f"🎉 Webhook processed successfully: {message.method}")
        return {"status": "accepted", "message": "Webhook processed successfully"}
        
    except HTTPException:
        # Re-raise HTTP exceptions (like 403 from threat detection)
        raise
    except Exception as e:
        logger.error(f"Error processing webhook: {e}")
        raise HTTPException(status_code=400, detail=f"Invalid payload: {str(e)}")


def check_message_for_threats(message: WebhookMessage) -> None:
    """
    Check DuckDuckGo search requests for unsafe query content.
    
    This example specifically looks for DuckDuckGo search tool usage
    and filters based on the search query content.
    
    Args:
        message: JSON-RPC message to check
        
    Raises:
        HTTPException: 403 status if unsafe search query detected
    """
    logger.info(f"Checking message for threats: {message.method}")
    
    # Look for DuckDuckGo search tool calls
    if message.method == "tools/call" and message.params:
        tool_name = message.params.get("name")
        arguments = message.params.get("arguments", {})
        
        if tool_name == "search" and "query" in arguments:
            query = arguments["query"]
            logger.info(f"🔍 Checking DuckDuckGo search query: '{query}'")
            
            if is_unsafe_search_query(query):
                logger.error(f"🚫 UNSAFE SEARCH QUERY BLOCKED: '{query}'")
                raise HTTPException(
                    status_code=403, 
                    detail="Search query rejected due to content policy"
                )
            else:
                logger.info(f"✅ Safe search query: '{query}'")
    
    logger.debug(f"✅ Clean message: {message.method}")


def is_unsafe_search_query(query: str) -> bool:
    """
    Check if a DuckDuckGo search query contains unsafe content.
    
    Args:
        query: The search query string
        
    Returns:
        True if query should be blocked, False if safe
    """
    if not query or not isinstance(query, str):
        return False
    
    query_lower = query.lower()
    
    # Simple list of terms we don't want to allow in searches
    unsafe_terms = [
        "how to hack",
        "how to exploit", 
        "malware download",
        "virus download",
        "illegal drugs",
        "how to make bomb",
        "assassination",
        "terrorist",
    ]
    
    for term in unsafe_terms:
        if term in query_lower:
            logger.warning(f"Found unsafe search term: '{term}'")
            return True
    
    # Block excessively long queries (potential injection attempts)
    if len(query) > 200:
        logger.warning(f"Query too long: {len(query)} chars")
        return True
    
    return False


@app.get("/health")
async def health_check():
    """Simple health check endpoint."""
    return {"status": "healthy", "filter": "ready"}


@app.get("/")
async def root():
    """Root endpoint with basic information."""
    return {
        "message": "Simple Webhook with Tool Filtering",
        "endpoints": {
            "/webhook": "Main webhook endpoint (POST)",
            "/health": "Health check (GET)",
            "/": "This information (GET)"
        },
        "note": "Send JSON-RPC messages to /webhook with proper signatures. Suspicious content will result in 403 responses."
    }


def main():
    """Start the webhook server."""
    import uvicorn
    
    logger.info("🚀 Starting Simple Webhook Server with Content Filtering")
    logger.info(f"🌐 Host: 0.0.0.0")
    logger.info(f"🔌 Port: {PORT}")
    logger.info(f"🔐 Secret configured: {'Yes' if SECRET != 'test_secret' else 'Using default (change WEBHOOK_SECRET)'}")
    logger.info(f"📋 Available endpoints:")
    logger.info(f"   POST /webhook - Main webhook endpoint")
    logger.info(f"   GET /health - Health check")
    logger.info(f"   GET / - Server information")
    
    uvicorn.run(
        app,
        host="0.0.0.0",
        port=PORT,
        log_level="info"
    )


if __name__ == "__main__":
    main()
```
