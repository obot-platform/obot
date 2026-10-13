These generated Go clients are copied unchanged from
agent-substrate/substrate, commit
`531c935370b9a14463c6588a63102922daebb810`, `pkg/proto/ateapipb`.
The upstream Apache 2.0 license is included here.

Keep this client pinned with the bundled chart. Copying the two generated files
avoids importing Substrate's server module and upgrading Obot's Kubernetes and
cloud dependencies solely for an RPC client.
