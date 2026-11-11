
import httpx
import pytest

from openapi_worker.httpclient import EgressPolicy, PinnedTransport


async def test_dns_all_answers_are_validated_and_connection_is_pinned():
    observed = []

    def handler(request):
        observed.append(request)
        return httpx.Response(200)

    resolver_calls = []

    async def resolver(host, port):
        resolver_calls.append((host, port))
        return ["93.184.216.34"]

    transport = PinnedTransport(EgressPolicy(), resolver=resolver, delegate=httpx.MockTransport(handler))
    response = await transport.handle_async_request(httpx.Request("GET", "https://business.example.test/items"))
    assert response.status_code == 200
    assert resolver_calls == [("business.example.test", 443)]
    assert observed[0].url.host == "93.184.216.34"
    assert observed[0].headers["host"] == "business.example.test:443"
    assert observed[0].extensions["sni_hostname"] == "business.example.test"


async def test_mixed_public_private_dns_is_rejected_before_delegate():
    calls = []

    async def resolver(host, port):
        return ["93.184.216.34", "10.0.0.8"]

    def handler(request):
        calls.append(request)
        return httpx.Response(200)

    transport = PinnedTransport(EgressPolicy(), resolver=resolver, delegate=httpx.MockTransport(handler))
    with pytest.raises(httpx.ConnectError):
        await transport.handle_async_request(httpx.Request("GET", "https://business.example.test/items"))
    assert not calls
