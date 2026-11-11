import asyncio
import ipaddress
import socket
from typing import AsyncIterator, Iterator
from typing import Awaitable, Callable

import httpx


class EgressPolicy:
    def __init__(self, allow_private: bool = False, allow_http: bool = False, allowed_hosts: set[str] | None = None) -> None:
        self.allow_private = allow_private
        self.allow_http = allow_http
        self.allowed_hosts = allowed_hosts or set()

    def validate_ip(self, host: str, ip: str) -> bool:
        address = ipaddress.ip_address(ip)
        if address.is_loopback or address.is_private or address.is_link_local or address.is_multicast or address.is_unspecified:
            return self.allow_private and (not self.allowed_hosts or host in self.allowed_hosts)
        return True


class PinnedTransport(httpx.AsyncBaseTransport):
    def __init__(self, policy: EgressPolicy, resolver: Callable[[str, int], Awaitable[list[str]]] | None = None, delegate: httpx.AsyncBaseTransport | None = None) -> None:
        self.policy = policy
        self.resolver = resolver
        self.delegate = delegate or httpx.AsyncHTTPTransport(retries=0)

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        url = request.url
        if url.scheme == "https" or (url.scheme == "http" and self.policy.allow_http):
            host = url.host
            port = url.port or (443 if url.scheme == "https" else 80)
            try:
                ipaddress.ip_address(host)
                answers = [host]
            except ValueError:
                if self.resolver is None:
                    loop = asyncio.get_running_loop()
                    infos = await loop.getaddrinfo(host, port, type=socket.SOCK_STREAM)
                    answers = list(dict.fromkeys(info[4][0] for info in infos))
                else:
                    answers = list(dict.fromkeys(await self.resolver(host, port)))
            if not answers or not all(self.policy.validate_ip(host, answer) for answer in answers):
                raise httpx.ConnectError("egress policy rejected upstream", request=request)
            pinned = answers[0]
            pinned_host = f"[{pinned}]" if ":" in pinned else pinned
            pinned_url = url.copy_with(host=pinned_host)
            pinned_request = httpx.Request(request.method, pinned_url, headers=request.headers, content=_AsyncByteStreamAdapter(request.stream))
            pinned_request.headers["host"] = f"{host}:{port}"
            pinned_request.extensions["sni_hostname"] = host
            pinned_request.extensions["timeout"] = request.extensions.get("timeout")
            return await self.delegate.handle_async_request(pinned_request)
        raise httpx.ConnectError("insecure upstream transport rejected", request=request)

    async def aclose(self) -> None:
        await self.delegate.aclose()


class _AsyncByteStreamAdapter(httpx.AsyncByteStream):
    def __init__(self, stream: httpx.SyncByteStream | httpx.AsyncByteStream) -> None:
        self.stream = stream

    def __aiter__(self):
        return self.aiter()

    async def aiter(self) -> AsyncIterator[bytes]:
        if hasattr(self.stream, "aiter"):
            async for chunk in self.stream.aiter():
                yield chunk
        else:
            for chunk in self.stream:
                yield chunk
