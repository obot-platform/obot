export const title = "Snapshot miss and one-shot execution fence";

const labels = {
    "new": "Request not seen",
    "cache-miss": "Cold cache observed",
    "snapshot-required": "Caller told to send snapshot",
    "compiling": "Snapshot compiling",
    "ready": "Snapshot ready, fence unused",
    "snapshot-invalid": "Snapshot rejected",
    "compile-failed": "Compilation failed",
    "business-done": "Business result returned",
    "transport-unknown": "Transport outcome uncertain",
};

export function state(value) {
    const record = value.request.r1;
    if (record.phase === "executing") {
        return record.entries > 1
            ? "Execution entered again"
            : "Upstream execution in flight";
    }
    if (record.phase === "ready" && record.entries > 0) {
        return "Snapshot ready after reuse";
    }
    if ((record.phase === "business-done" || record.phase === "transport-unknown") &&
        record.entries > 1) {
        return `${labels[record.phase]} after duplicate entry`;
    }
    return labels[record.phase] ?? "Unexpected request state";
}

export function group(value) {
    const phase = value.request.r1.phase;
    if (phase === "new" || phase === "cache-miss" || phase === "snapshot-required") {
        return "Cold snapshot";
    }
    if (phase === "compiling" || phase === "ready") {
        return "Snapshot load";
    }
    if (phase === "snapshot-invalid" || phase === "compile-failed") {
        return "Snapshot rejected";
    }
    return value.request.r1.entries > 1 ? "Fence bypassed" : "Protected execution";
}

export const colors = {
    "Cold snapshot": "blue",
    "Snapshot load": "teal",
    "Snapshot rejected": "tan",
    "Protected execution": "purple",
    "Fence bypassed": "orange",
};

export function step(action, from, to) {
    if (action.startsWith("StartExecute")) return "first execute sees the cache";
    if (action.startsWith("ReturnSnapshotRequired")) return "worker asks for the snapshot";
    if (action.startsWith("SupplySnapshot")) {
        return to.request.r1.phase === "snapshot-invalid"
            ? "caller sends an invalid snapshot"
            : "caller sends the snapshot once";
    }
    if (action.startsWith("CompileCompletes")) {
        return to.request.r1.phase === "ready"
            ? "compiled plan becomes ready"
            : "compilation fails";
    }
    if (action.startsWith("AcquireExecutionFence")) {
        return from.request.r1.entries > 0
            ? "same request enters execution again"
            : "worker takes the one-shot fence";
    }
    if (action.startsWith("BeginUpstream")) return "worker starts the business API call";
    if (action.startsWith("UpstreamCompletes")) return "business response returns";
    if (action.startsWith("UpstreamTransportFails")) return "transport outcome becomes uncertain";
    if (action.startsWith("RejectReuse")) return "worker rejects same-request reuse";
    if (action.startsWith("RejectDigestMismatch")) return "worker rejects another digest";
    if (action === "Done") return null;
    return "state advances";
}
