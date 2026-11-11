from dataclasses import dataclass


@dataclass(slots=True)
class WorkerError(Exception):
    code: str
    message: str
    retryable: bool = False
    phase: str = "request"
    upstream_attempted: bool = False

    def payload(self, request_id: str | None = None) -> dict:
        return {
            "kind": "error",
            "code": self.code,
            "message": self.message,
            "requestID": request_id,
            "retryable": self.retryable,
            "phase": self.phase,
            "upstream_attempted": self.upstream_attempted,
        }
