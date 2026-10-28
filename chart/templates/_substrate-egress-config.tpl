{{- define "obot.substrate.egressConfig" -}}
{{- $enabled := .Values.substrate.enabled -}}
{{- $obotURL := printf "http://%s.%s.svc:%v" (include "obot.fullname" .) .Release.Namespace .Values.service.port -}}
{{- $url := urlParse $obotURL -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: atenet-egress
  namespace: ate-system
data:
  envoy.yaml: |
    admin:
      address:
        # Administrative operations are only available within this pod.
        socket_address: { address: "127.0.0.1", port_value: 15000 }

    # Internal listeners need this extension registered; without it they
    # silently fail to load.
    bootstrap_extensions:
    - name: envoy.bootstrap.internal_listener
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.bootstrap.internal_listener.v3.InternalListener

    static_resources:
      listeners:
      # ---------------------------------------------------------------------
      # Listener A: terminates atunnel's mTLS and the CONNECT, authenticates
      # actor, supplies the SNI rules, and hands the tunnel to the inner
      # listener. That listener authorizes TLS before choosing a TCP route.
      # ---------------------------------------------------------------------
      - name: egress
        address:
          # ipv4_compat rather than a second socket: IPv4 peers arrive as
          # ::ffff: and nothing here reads the peer -- identity is the cert.
          socket_address: { address: "::", ipv4_compat: true, port_value: 443 }
        filter_chains:
        # Named so ext_proc can read it back as xds.filter_chain_name. Must
        # match EgressFilterChainName in
        # cmd/atenet/internal/router/extproc/dispatch.go.
        - name: egress
          transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              require_client_certificate: true
              common_tls_context:
                # Gateway server identity (servicedns signer), via filesystem
                # SDS: watched_directory only works on SDS-delivered secrets,
                # so an inline cert would never pick up kubelet's rotation.
                tls_certificate_sds_secret_configs:
                - name: servicedns_serving_cert
                  sds_config:
                    resource_api_version: V3
                    path_config_source:
                      path: /etc/envoy/sds-servicedns-cert.yaml
                validation_context:
                  # Actors authenticate with an actor-identity client cert.
                  # Envoy enforces chain, signature, and validity period here, so
                  # an unsigned or expired actor cert never reaches ext_proc; the
                  # ActorIdentity extension is checked there because Envoy cannot
                  # read custom X.509 extensions.
                  trusted_ca: { filename: /run/actor-id-ca-certs/ca.crt }
          filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              stat_prefix: egress_connect
              codec_type: HTTP1
              upgrade_configs:
              - upgrade_type: CONNECT
              # TODO(liorlieberman): Can we make this cleaner?
              forward_client_cert_details: SANITIZE_SET
              set_current_client_cert_details:
                chain: true
              # Emit the access log as soon as the CONNECT tunnel is established
              # (atunnel keeps the tunnel open, so the default log-on-close would
              # not fire during the demo).
              access_log_options:
                flush_log_on_tunnel_successfully_established: true
              access_log:
              - name: envoy.access_loggers.stdout
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
                  log_format:
                    text_format_source:
                      inline_string: "[egress] authority=%REQ(:AUTHORITY)% peer_san=%DOWNSTREAM_PEER_URI_SAN% peer_serial=%DOWNSTREAM_PEER_SERIAL% code=%RESPONSE_CODE% flags=%RESPONSE_FLAGS% up_bytes=%BYTES_RECEIVED% down_bytes=%BYTES_SENT%\n"
              route_config:
                name: connect_route
                virtual_hosts:
                - name: connect
                  domains: ["*"]
                  routes:
                  - match: { connect_matcher: {} }
                    route:
                      # The tunnel goes to the inner listener, which classifies
                      # what it carries and polices each kind on what it exposes.
                      cluster: inner_internal
                      # Set request timeout to disabled (0) as CONNECT is a long-running stream.
                      # There will still be a stream_idle_timeout (5 minutes by default).
                      timeout: 0s
                      upgrade_configs:
                      - upgrade_type: CONNECT
                        connect_config: {}
              http_filters:
              # The actor's identity, as filter state: headers do not cross the
              # CONNECT boundary, and one sent from inside the tunnel would be
              # actor-controlled. The value is the verified peer certificate's SAN.
              - name: envoy.filters.http.set_filter_state
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.set_filter_state.v3.Config
                  on_request_headers:
                  - object_key: dev.ate.actor.identity
                    factory_key: envoy.string
                    shared_with_upstream: ONCE
                    skip_if_empty: true
                    format_string:
                      text_format_source:
                        inline_string: "%DOWNSTREAM_PEER_URI_SAN%"
                      # Without this an absent SAN renders as "-", which is a
                      # non-empty string and fails open at whatever reads it.
                      omit_empty_values: true
                  # The IP:port the actor dialed, for the request legs to match a
                  # rule's ports against: a port in a request's Host is not it.
                  - object_key: dev.ate.connect.authority
                    factory_key: envoy.string
                    shared_with_upstream: ONCE
                    skip_if_empty: true
                    format_string:
                      text_format_source:
                        inline_string: "%REQ(:AUTHORITY)%"
                      omit_empty_values: true
                  # Isolate inner connections by actor and CONNECT destination so
                  # a reused tunnel cannot carry another destination's filter state.
                  # Not shared upstream, so this never overrides external TLS SNI.
                  - object_key: envoy.network.upstream_server_name
                    skip_if_empty: true
                    format_string:
                      text_format_source:
                        inline_string: "%DOWNSTREAM_PEER_URI_SAN%|%REQ(:AUTHORITY)%"
                      omit_empty_values: true
              # Authenticate the actor and obtain its port-specific SNI rules.
              # Fail closed if the policy service is unavailable.
              - name: envoy.filters.http.ext_proc
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor
                  stat_prefix: egress_identity
                  grpc_service:
                    envoy_grpc:
                      cluster_name: ext_proc_server
                    timeout: 2s
                  failure_mode_allow: false
                  # Set ext_proc timeout to 5s (from default 200ms) to handle slower ext_proc transactions.
                  message_timeout: 5s
                  # How the ext_proc server tells egress from ingress. It applies
                  # opposite trust models to the two directions, and dispatches on
                  # this Envoy-asserted filter chain name so that no client can
                  # select the egress path by crafting a request. The value must
                  # match EgressFilterChainName in
                  # cmd/atenet/internal/router/extproc/dispatch.go; renaming the
                  # filter chain above without updating it fails closed (the
                  # request is classified as ingress, which this --mode=egress
                  # instance does not serve, and is refused with a 404).
                  request_attributes:
                  - xds.filter_chain_name
                  processing_mode:
                    request_header_mode: SEND
                    response_header_mode: SKIP
                    request_body_mode: NONE
                    response_body_mode: NONE
                    request_trailer_mode: SKIP
                    response_trailer_mode: SKIP
                  # This leg authenticates and never rewrites; the sidecar may
                  # not touch routing or Envoy's own headers here.
                  mutation_rules:
                    disallow_system: true
                    disallow_is_error: true
                  # Envoy silently drops metadata from a namespace not listed
                  # here, and the filter state below would never be set.
                  metadata_options:
                    receiving_namespaces:
                      untyped:
                      - dev.ate.egress
                      - dev.ate.policy.egress
              # Carry the destination and authenticated policy across CONNECT.
              # The inner listener must approve SNI before selecting TCP; having
              # a destination alone does not authorize a connection.
              - name: envoy.filters.http.set_filter_state
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.set_filter_state.v3.Config
                  on_request_headers:
                  - object_key: envoy.upstream.dynamic_port
                    skip_if_empty: true
                    shared_with_upstream: ONCE
                    format_string:
                      text_format_source:
                        inline_string: "%DYNAMIC_METADATA(dev.ate.egress:dialed_port)%"
                      omit_empty_values: true
                  - object_key: dev.ate.policy.egress
                    factory_key: envoy.string
                    skip_if_empty: true
                    shared_with_upstream: ONCE
                    format_string:
                      text_format_source:
                        inline_string: "%DYNAMIC_METADATA(dev.ate.policy.egress)%"
                      omit_empty_values: true
              - name: envoy.filters.http.router
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

      # ---------------------------------------------------------------------
      # Listener B is internal: only the authenticated CONNECT leg reaches it.
      # Classify TLS by SNI, then forward approved connections without decrypting.
      # Cleartext HTTP is still authorized per request by ext_proc.
      # ---------------------------------------------------------------------
      - name: inner_listener
        stat_prefix: inner
        internal_listener: {}
        {{- if $enabled }}
        # SNI denials have no HTTP filter chain, so capture them at the listener.
        access_log:
        - name: envoy.access_loggers.tcp_grpc
          filter:
            response_flag_filter:
              flags: ["NR"]
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.TcpGrpcAccessLogConfig
            common_config:
              log_name: obot_egress_denied
              transport_api_version: V3
              grpc_service:
                envoy_grpc:
                  cluster_name: obot_audit
              filter_state_objects_to_log:
              - dev.ate.actor.identity
        {{- end }}
        # An inspector timeout must not fall through to an unchecked TCP route.
        listener_filters_timeout: 1s
        continue_on_listener_filters_timeout: true
        listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
        - name: envoy.filters.listener.http_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.http_inspector.v3.HttpInspector
        - name: envoy.filters.listener.dynamic_modules
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.dynamic_modules.v3.DynamicModuleListenerFilter
            dynamic_module_config:
              name: envoy_substrate_egress_policy
            filter_name: envoy_substrate_egress_policy
        filter_chain_matcher:
          matcher_tree:
            input:
              name: filter_state
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.matching.common_inputs.network.v3.FilterStateInput
                key: dev.ate.egress.filter_chain
            exact_match_map:
              map:
                {{- if $enabled }}
                "mitm":
                  action:
                    name: egress_tls_mitm
                    typed_config:
                      "@type": type.googleapis.com/google.protobuf.StringValue
                      value: egress_tls_mitm
                {{- end }}
                {{- if not $enabled }}
                "passthrough":
                  action:
                    name: egress_passthrough
                    typed_config:
                      "@type": type.googleapis.com/google.protobuf.StringValue
                      value: egress_passthrough
                {{- end }}
                "cleartext":
                  action:
                    name: egress_cleartext
                    typed_config:
                      "@type": type.googleapis.com/google.protobuf.StringValue
                      value: egress_cleartext
                "denied":
                  action:
                    name: non_existent_filter_chain
                    typed_config:
                      "@type": type.googleapis.com/google.protobuf.StringValue
                      value: denied
        filter_chains:
        {{- if $enabled }}
        - name: egress_tls_mitm
          filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              stat_prefix: mitm_http
              # Absent this, Envoy answers an Upgrade request itself instead of
              # proxying it, so every WebSocket through this leg fails at the
              # handshake. Note it covers the HTTP/1.1 path only; WebSockets over
              # h2 are RFC 8441 extended CONNECT and would additionally need
              # http2_protocol_options.allow_connect.
              upgrade_configs:
              - upgrade_type: websocket
              # Disable stream_idle_timeout as there is already an overall timeout in the outer tunnel in the HCM.
              stream_idle_timeout: 0s
              # This is the leg that knows where the traffic actually went.
              # REQUESTED_SERVER_NAME is the SNI the leaf was minted for and it
              # should equal the authority, which is what a by-name route
              # resolves and what both routes verify the origin against.
              access_log:
              {{- if $enabled }}
              - name: envoy.access_loggers.http_grpc
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig
                  common_config:
                    log_name: obot_egress
                    transport_api_version: V3
                    grpc_service:
                      envoy_grpc:
                        cluster_name: obot_audit
              {{- end }}
              - name: envoy.access_loggers.file
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                  path: /dev/stdout
                  log_format:
                    formatters:
                    - name: envoy.formatter.req_without_query
                      typed_config:
                        "@type": type.googleapis.com/envoy.extensions.formatter.req_without_query.v3.ReqWithoutQuery
                    json_format:
                      leg: mitm
                      time: "%START_TIME%"
                      actor: "%FILTER_STATE(dev.ate.actor.identity:PLAIN)%"
                      sni: "%REQUESTED_SERVER_NAME%"
                      authority: "%REQ(:AUTHORITY)%"
                      method: "%REQ(:METHOD)%"
                      path: "%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%"
                      protocol: "%PROTOCOL%"
                      status: "%RESPONSE_CODE%"
                      grpc_status: "%GRPC_STATUS%"
                      flags: "%RESPONSE_FLAGS%"
                      duration_ms: "%DURATION%"
                      bytes_sent: "%BYTES_SENT%"
                      bytes_rcvd: "%BYTES_RECEIVED%"
                      upstream: "%UPSTREAM_HOST%"
                      upstream_failure: "%UPSTREAM_TRANSPORT_FAILURE_REASON%"
                      termination: "%CONNECTION_TERMINATION_DETAILS%"
              route_config:
                name: local
                virtual_hosts:
                - name: all
                  domains: ["*"]
                  routes:
                  # Upgrade is what cannot cross an h2 upstream: forwarding one
                  # needs RFC 8441 extended CONNECT, which almost no origin
                  # advertises. So the default route below stays pinned to
                  # HTTP/1.1, which every origin accepts, rather than letting
                  # upstream ALPN choose -- ALPN takes h2 against nearly
                  # everything today, and every WebSocket then fails with a 502.
                  #
                  # gRPC is the one thing that default cannot serve: its status
                  # rides in trailers, which HTTP/1.1 cannot express.
                  # content-type is how it opts out. The prefix also catches
                  # application/grpc-web, which is deliberate --
                  # egress_forward_proxy_grpc reads its protocol from upstream
                  # ALPN, so a grpc-web endpoint that only speaks HTTP/1.1 still
                  # gets HTTP/1.1.
                  #
                  # The cost of the pinned default is that non-gRPC h2 traffic
                  # is downgraded and its trailers dropped silently. Nothing
                  # sends those today. If that changes, invert this: match on
                  # the `upgrade` header, pin that route to HTTP/1.1, and let
                  # everything else negotiate.
                  #
                  # Each of the two exists once per dial the sidecar answers
                  # with (dev.ate.egress:dial), and there is no default: a
                  # request it did not answer for has no route.
                  - match:
                      prefix: "/"
                      headers:
                      - name: content-type
                        string_match:
                          prefix: application/grpc
                      dynamic_metadata:
                      - filter: dev.ate.egress
                        path: [{ key: dial }]
                        value: { string_match: { exact: name } }
                    route:
                      cluster: egress_forward_proxy_grpc
                      timeout: 0s
                  - match:
                      prefix: "/"
                      headers:
                      - name: content-type
                        string_match:
                          prefix: application/grpc
                      dynamic_metadata:
                      - filter: dev.ate.egress
                        path: [{ key: dial }]
                        value: { string_match: { exact: address } }
                    route:
                      cluster: egress_original_dst_tls_grpc
                      timeout: 0s
                  - match:
                      prefix: "/"
                      dynamic_metadata:
                      - filter: dev.ate.egress
                        path: [{ key: dial }]
                        value: { string_match: { exact: name } }
                    route:
                      cluster: egress_forward_proxy
                      # The route timeout runs until the response completes, which
                      # for SSE or long-poll is never, so 0.
                      timeout: 0s
                  - match:
                      prefix: "/"
                      dynamic_metadata:
                      - filter: dev.ate.egress
                        path: [{ key: dial }]
                        value: { string_match: { exact: address } }
                    route:
                      cluster: egress_original_dst_tls
                      timeout: 0s
              http_filters:
              {{- if $enabled }}
              - name: envoy.filters.http.ext_proc
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor
                  stat_prefix: obot_audit
                  grpc_service:
                    envoy_grpc:
                      cluster_name: obot_audit
                  message_timeout: 5s
                  failure_mode_allow: false
                  # The audit service turns body processing off for upgrade
                  # requests, which Envoy cannot forward with bodies streamed.
                  allow_mode_override: true
                  request_attributes:
                  - filter_state['dev.ate.actor.identity']
                  processing_mode:
                    request_header_mode: SEND
                    response_header_mode: SEND
                    request_body_mode: STREAMED
                    response_body_mode: STREAMED
                    request_trailer_mode: SEND
                    response_trailer_mode: SEND
                  metadata_options:
                    receiving_namespaces:
                      untyped: [obot.audit]
              {{- end }}
              # Every request on this chain is authorized against the actor's
              # EgressPolicy by the same sidecar; the actor comes from the filter
              # state the CONNECT leg set, never from a header.
              - name: envoy.filters.http.ext_proc
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor
                  stat_prefix: egress_policy_mitm
                  grpc_service:
                    envoy_grpc:
                      cluster_name: ext_proc_server
                    timeout: 5s
                  failure_mode_allow: false
                  # A cache miss in the sidecar is one ateapi round trip; 5s covers that
                  # with room, and a slower answer is a denied request, not an allowed one.
                  message_timeout: 5s
                  request_attributes:
                  - xds.filter_chain_name
                  - filter_state['dev.ate.actor.identity']
                  - filter_state['dev.ate.connect.authority']
                  - filter_state['envoy.network.transport_socket.original_dst_address'].ip
                  - filter_state['envoy.network.transport_socket.original_dst_address'].port
                  - connection.requested_server_name
                  processing_mode:
                    request_header_mode: SEND
                    response_header_mode: SKIP
                    request_body_mode: NONE
                    response_body_mode: NONE
                    request_trailer_mode: SKIP
                    response_trailer_mode: SKIP
                  # The sidecar may add headers but never touch routing: a rewritten
                  # Host would dial a name other than the one policed.
                  mutation_rules:
                    disallow_system: true
                    disallow_is_error: true
                  # The answer (dev.ate.egress:dial) picks the route. Envoy drops
                  # metadata from a namespace not listed here, and a request
                  # with no dial has no route.
                  metadata_options:
                    receiving_namespaces:
                      untyped:
                      - dev.ate.egress
              #ATE_MITM_EXTPROC_FILTER -- see the mitm_listener comment above.
              # Resolves the Host for the by-name route. It does nothing for a
              # route to any other kind of cluster.
              - name: envoy.filters.http.dynamic_forward_proxy
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.dynamic_forward_proxy.v3.FilterConfig
                  dns_cache_config:
                    name: egress_dns_cache
                    # V4_ONLY resolves nothing on an IPv6-only cluster. ALL
                    # returns both families and enables Happy Eyeballs, so
                    # neither family is stranded.
                    dns_lookup_family: ALL
              - name: envoy.filters.http.router
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
          # REQUIRED. Without it an sdsmint outage does not fail a
          # first-contact handshake, it pauses it forever.
          transport_socket_connect_timeout: 5s
          transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                alpn_protocols: ["h2", "http/1.1"]
                custom_tls_certificate_selector:
                  name: on-demand
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.cert_selectors.on_demand_secret.v3.Config
                    config_source:
                      api_config_source:
                        api_type: DELTA_GRPC
                        transport_api_version: V3
                        grpc_services:
                        - envoy_grpc:
                            cluster_name: sds_mint
                    certificate_mapper:
                      name: sni
                      typed_config:
                        "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.cert_mappers.sni.v3.SNI
                        # Reached only by a client that sent no SNI, i.e. one
                        # dialing by IP. It gets a leaf for this name, which
                        # resolves nowhere and so matches no destination it
                        # could have meant -- and since the client prints the
                        # name in its verification error, the name says why.
                        default_value: "sni-required.egress.ate.invalid"  # default_value is required.
              # A resumed session skips certificate selection entirely, which
              # would hand a client a secret minted for a different name.
              disable_stateless_session_resumption: true
              disable_stateful_session_resumption: true

        {{- end }}
        # The cleartext chain: ext_proc decides every request and its answer
        # (dev.ate.egress:dial) picks the route, by name or to the dialed
        # address. Non-HTTP traffic fails HTTP parsing here.
        - name: egress_cleartext
          filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              stat_prefix: egress_cleartext
              # http_inspector steers HTTP/1.0 here; without this the codec
              # answers it 426 before any filter runs.
              http_protocol_options:
                accept_http_10: true
              # Without this Envoy answers an Upgrade itself instead of proxying
              # it, and every WebSocket fails at the handshake.
              upgrade_configs:
              - upgrade_type: websocket
              # The outer HCM's stream_idle_timeout already bounds an idle
              # tunnel.
              stream_idle_timeout: 0s
              access_log:
              {{- if $enabled }}
              - name: envoy.access_loggers.http_grpc
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig
                  common_config:
                    log_name: obot_egress
                    transport_api_version: V3
                    grpc_service:
                      envoy_grpc:
                        cluster_name: obot_audit
              {{- end }}
              - name: envoy.access_loggers.file
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                  path: /dev/stdout
                  log_format:
                    formatters:
                    - name: envoy.formatter.req_without_query
                      typed_config:
                        "@type": type.googleapis.com/envoy.extensions.formatter.req_without_query.v3.ReqWithoutQuery
                    json_format:
                      leg: cleartext
                      time: "%START_TIME%"
                      actor: "%FILTER_STATE(dev.ate.actor.identity:PLAIN)%"
                      authority: "%REQ(:AUTHORITY)%"
                      method: "%REQ(:METHOD)%"
                      path: "%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%"
                      protocol: "%PROTOCOL%"
                      status: "%RESPONSE_CODE%"
                      grpc_status: "%GRPC_STATUS%"
                      flags: "%RESPONSE_FLAGS%"
                      duration_ms: "%DURATION%"
                      bytes_sent: "%BYTES_SENT%"
                      bytes_rcvd: "%BYTES_RECEIVED%"
                      upstream: "%UPSTREAM_HOST%"
                      upstream_failure: "%UPSTREAM_TRANSPORT_FAILURE_REASON%"
                      termination: "%CONNECTION_TERMINATION_DETAILS%"
              route_config:
                name: cleartext
                request_headers_to_remove: [x-obot-egress-key]
                virtual_hosts:
                - name: all
                  domains: ["*"]
                  # One route per dial the sidecar answers with, and no default:
                  # a request it did not answer for has no route.
                  routes:
                  {{- if $enabled }}
                  - match:
                      prefix: "/"
                      dynamic_metadata:
                      - filter: obot.mcp
                        path: [{ key: authorized }]
                        value: { bool_match: true }
                      - filter: dev.ate.egress
                        path: [{ key: dial }]
                        value: { string_match: { exact: name } }
                    route:
                      cluster: obot_mcp
                      timeout: 0s
                  {{- end }}
                  - match:
                      prefix: "/"
                      dynamic_metadata:
                      - filter: dev.ate.egress
                        path: [{ key: dial }]
                        value: { string_match: { exact: name } }
                    route:
                      cluster: egress_forward_proxy_cleartext
                      # The route timeout runs until the response completes, which
                      # for SSE or long-poll is never, so 0.
                      timeout: 0s
                  - match:
                      prefix: "/"
                      dynamic_metadata:
                      - filter: dev.ate.egress
                        path: [{ key: dial }]
                        value: { string_match: { exact: address } }
                    route:
                      cluster: egress_original_dst_cleartext
                      timeout: 0s
              http_filters:
              {{- if $enabled }}
              - name: envoy.filters.http.ext_proc
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor
                  stat_prefix: obot_audit
                  grpc_service:
                    envoy_grpc:
                      cluster_name: obot_audit
                  message_timeout: 5s
                  failure_mode_allow: false
                  # The audit service turns body processing off for upgrade
                  # requests, which Envoy cannot forward with bodies streamed.
                  allow_mode_override: true
                  request_attributes:
                  - filter_state['dev.ate.actor.identity']
                  processing_mode:
                    request_header_mode: SEND
                    response_header_mode: SEND
                    request_body_mode: STREAMED
                    response_body_mode: STREAMED
                    request_trailer_mode: SEND
                    response_trailer_mode: SEND
                  metadata_options:
                    receiving_namespaces:
                      untyped: [obot.audit]
              {{- end }}
              # Every request on this chain is authorized against the actor's
              # EgressPolicy by the same sidecar; the actor comes from the filter
              # state the CONNECT leg set, never from a header.
              - name: envoy.filters.http.ext_proc
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor
                  stat_prefix: egress_policy_cleartext
                  grpc_service:
                    envoy_grpc:
                      cluster_name: ext_proc_server
                    timeout: 5s
                  failure_mode_allow: false
                  # A cache miss in the sidecar is one ateapi round trip; 5s covers that
                  # with room, and a slower answer is a denied request, not an allowed one.
                  message_timeout: 5s
                  request_attributes:
                  - xds.filter_chain_name
                  - filter_state['dev.ate.actor.identity']
                  - filter_state['dev.ate.connect.authority']
                  - filter_state['envoy.network.transport_socket.original_dst_address'].ip
                  - filter_state['envoy.network.transport_socket.original_dst_address'].port
                  - connection.requested_server_name
                  processing_mode:
                    request_header_mode: SEND
                    response_header_mode: SKIP
                    request_body_mode: NONE
                    response_body_mode: NONE
                    request_trailer_mode: SKIP
                    response_trailer_mode: SKIP
                  # The sidecar may add headers but never touch routing: a rewritten
                  # Host would dial a name other than the one policed.
                  mutation_rules:
                    disallow_system: true
                    disallow_is_error: true
                  # The answer (dev.ate.egress:dial) picks the route. Envoy drops
                  # metadata from a namespace not listed here, and a request
                  # with no dial has no route.
                  metadata_options:
                    receiving_namespaces:
                      untyped:
                      - dev.ate.egress
              {{- if $enabled }}
              - name: envoy.filters.http.lua
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
                  default_source_code:
                    inline_string: |
                      local obot_authority = {{ $url.host | lower | quote }}
                      {{- .Files.Get "files/obot-egress.lua" | nindent 22 }}
              {{- end }}
              # Resolves the Host for the by-name route. It does nothing for a
              # route to any other kind of cluster.
              - name: envoy.filters.http.dynamic_forward_proxy
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.dynamic_forward_proxy.v3.FilterConfig
                  dns_cache_config:
                    name: egress_dns_cache
                    # V4_ONLY resolves nothing on an IPv6-only cluster. ALL
                    # returns both families and enables Happy Eyeballs, so
                    # neither family is stranded.
                    dns_lookup_family: ALL
              - name: envoy.filters.http.router
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

        # Only the module's approved SNI verdict selects this chain. It dials
        # the gateway-resolved SNI on the port from the CONNECT policy response.
        - name: egress_passthrough
          filters:
          - name: envoy.filters.network.sni_dynamic_forward_proxy
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.sni_dynamic_forward_proxy.v3.FilterConfig
              dns_cache_config:
                name: egress_dns_cache
                dns_lookup_family: ALL
              port_value: 443
          - name: envoy.filters.network.tcp_proxy
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
              stat_prefix: passthrough
              cluster: egress_forward_proxy_passthrough
              access_log:
              - name: envoy.access_loggers.file
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                  path: /dev/stdout
                  log_format:
                    json_format:
                      leg: passthrough
                      time: "%START_TIME%"
                      actor: "%FILTER_STATE(dev.ate.actor.identity:PLAIN)%"
                      sni: "%REQUESTED_SERVER_NAME%"
                      dialed: "%FILTER_STATE(dev.ate.connect.authority:PLAIN)%"
                      flags: "%RESPONSE_FLAGS%"
                      duration_ms: "%DURATION%"
                      bytes_sent: "%BYTES_SENT%"
                      bytes_rcvd: "%BYTES_RECEIVED%"
                      upstream: "%UPSTREAM_HOST%"
                      upstream_failure: "%UPSTREAM_TRANSPORT_FAILURE_REASON%"
                      termination: "%CONNECTION_TERMINATION_DETAILS%"

      # Kubelet can check readiness without reaching the administration API.
      - name: readiness
        address:
          socket_address: { address: "::", ipv4_compat: true, port_value: 15001 }
        filter_chains:
        - filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              stat_prefix: readiness
              route_config:
                name: readiness
                virtual_hosts:
                - name: readiness
                  domains: ["*"]
                  routes:
                  - match:
                      path: /ready
                      headers:
                      - name: ":method"
                        string_match: { exact: GET }
                    route: { cluster: local_admin, timeout: 1s }
              http_filters:
              - name: envoy.filters.http.router
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

      clusters:
      - name: local_admin
        connect_timeout: 1s
        type: STATIC
        load_assignment:
          cluster_name: local_admin
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  socket_address: { address: "127.0.0.1", port_value: 15000 }
      {{- if $enabled }}
      - name: egress_forward_proxy
        lb_policy: CLUSTER_PROVIDED
        connect_timeout: 5s
        cluster_type:
          name: envoy.clusters.dynamic_forward_proxy
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.clusters.dynamic_forward_proxy.v3.ClusterConfig
            dns_cache_config:
              name: egress_dns_cache
              dns_lookup_family: ALL
        # The MITM must not weaken upstream authentication. Envoy decrypted the
        # actor's TLS with a leaf of its own; it still sends the real SNI here
        # and still validates the real origin's certificate against the public
        # roots. Without this the re-originated request would leave the pod in
        # plaintext.
        #
        # Spelled through typed_extension_protocol_options because the
        # cluster-level upstream_http_protocol_options field is deprecated.
        #
        # max_session_keys: 0 on this and the other upstream TLS clusters: a
        # cluster keeps one client session cache for every host it dials, and
        # Envoy offers the cached session whatever the SNI. Origins that share
        # ticket keys (several Cloudflare zones) resume it and hand back the
        # first host's certificate, which auto_san_validation then rejects.
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            upstream_http_protocol_options:
              auto_sni: true
              auto_san_validation: true
            explicit_http_config:
              http_protocol_options: {}
        transport_socket:
          name: envoy.transport_sockets.tls
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
            max_session_keys: 0
            common_tls_context:
              validation_context:
                trusted_ca:
                  filename: /etc/ssl/certs/ca-certificates.crt

      - name: egress_forward_proxy_grpc
        lb_policy: CLUSTER_PROVIDED
        connect_timeout: 5s
        cluster_type:
          name: envoy.clusters.dynamic_forward_proxy
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.clusters.dynamic_forward_proxy.v3.ClusterConfig
            dns_cache_config:
              name: egress_dns_cache
              dns_lookup_family: ALL
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            upstream_http_protocol_options:
              auto_sni: true
              auto_san_validation: true
            auto_config:
              http_protocol_options: {}
              http2_protocol_options: {}
        transport_socket:
          name: envoy.transport_sockets.tls
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
            max_session_keys: 0
            common_tls_context:
              validation_context:
                trusted_ca:
                  filename: /etc/ssl/certs/ca-certificates.crt

      - name: egress_original_dst_tls
        type: ORIGINAL_DST
        lb_policy: CLUSTER_PROVIDED
        connect_timeout: 5s
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            upstream_http_protocol_options:
              auto_sni: true
              auto_san_validation: true
            explicit_http_config:
              http_protocol_options: {}
        transport_socket:
          name: envoy.transport_sockets.tls
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
            max_session_keys: 0
            common_tls_context:
              validation_context:
                trusted_ca:
                  filename: /etc/ssl/certs/ca-certificates.crt

      - name: egress_original_dst_tls_grpc
        type: ORIGINAL_DST
        lb_policy: CLUSTER_PROVIDED
        connect_timeout: 5s
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            upstream_http_protocol_options:
              auto_sni: true
              auto_san_validation: true
            auto_config:
              http_protocol_options: {}
              http2_protocol_options: {}
        transport_socket:
          name: envoy.transport_sockets.tls
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
            max_session_keys: 0
            common_tls_context:
              validation_context:
                trusted_ca:
                  filename: /etc/ssl/certs/ca-certificates.crt

      - name: sds_mint
        type: STATIC
        connect_timeout: 1s
        # One HTTP/2 connection carrying one DELTA_GRPC stream per name in
        # flight. max_requests is the one that matters, and it is subtler than
        # a burst limit: Envoy does not close the stream once the secret
        # arrives, it holds it open for as long as the secret is live. So this
        # is not "concurrent mints", it is a hard cap on the size of the live
        # secret set. Past the cap Envoy is refused a mint rather than queued
        # for one, which surfaces as a handshake failure and looks exactly like
        # the allowlist denying the name -- silently, at a threshold nothing
        # else in this file mentions. 32768 is deliberately ~3x the
        # 10,000-name target rather than equal to it: the memory guards above
        # are what should bind first, at ~13,200, and a limit that trips before
        # them would replace a diagnosable overload with a mystery. Raise it
        # when you raise the target.
        circuit_breakers:
          thresholds:
          - priority: DEFAULT
            max_connections: 128
            max_pending_requests: 32768
            max_requests: 32768
            max_retries: 8
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            explicit_http_config:
              http2_protocol_options: {}
        load_assignment:
          cluster_name: sds_mint
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  pipe: {path: /var/run/sdsmint/sdsmint.sock}
      {{- end }}
      {{- if $enabled }}
      - name: obot_audit
        type: STRICT_DNS
        connect_timeout: 5s
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            explicit_http_config:
              http2_protocol_options: {}
        load_assignment:
          cluster_name: obot_audit
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{ include "obot.fullname" . }}-egress-credentials.{{ .Release.Namespace }}.svc
                    port_value: 8444
        transport_socket:
          name: envoy.transport_sockets.tls
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
            sni: {{ include "obot.fullname" . }}-egress-credentials.{{ .Release.Namespace }}.svc
            common_tls_context:
              # Obot's audit service requires TLS 1.3.
              tls_params:
                tls_minimum_protocol_version: TLSv1_3
                tls_maximum_protocol_version: TLSv1_3
              alpn_protocols: [h2]
              tls_certificate_sds_secret_configs:
              - name: obot_audit_client
                sds_config:
                  resource_api_version: V3
                  path_config_source:
                    path: /etc/envoy/sds-audit-client.yaml
              validation_context_sds_secret_config:
                name: obot_audit_server
                sds_config:
                  resource_api_version: V3
                  path_config_source:
                    path: /etc/envoy/sds-audit-server.yaml
      - name: obot_mcp
        type: STRICT_DNS
        connect_timeout: 5s
        load_assignment:
          cluster_name: obot_mcp
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{ (splitList ":" $url.host | first) | quote }}
                    port_value: {{ if contains ":" $url.host }}{{ splitList ":" $url.host | last | int }}{{ else }}80{{ end }}
      {{- end }}
      # The only way into inner_listener. internal_upstream copies every
      # shared actor identity, CONNECT authority, and SNI policy to the inner
      # connection.
      - name: inner_internal
        connect_timeout: 1s
        # One CONNECT per connection: the filter state internal_upstream copies
        # belongs to the tunnel this connection was made for, and a reused
        # connection would carry it into the next tunnel.
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            common_http_protocol_options:
              max_requests_per_connection: 1
            explicit_http_config:
              http_protocol_options: {}
        transport_socket:
          name: envoy.transport_sockets.internal_upstream
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.transport_sockets.internal_upstream.v3.InternalUpstreamTransport
            transport_socket:
              name: envoy.transport_sockets.raw_buffer
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.transport_sockets.raw_buffer.v3.RawBuffer
        load_assignment:
          cluster_name: inner_internal
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  envoy_internal_address:
                    server_listener_name: inner_listener

      # ext_proc gRPC server = the atenet router, co-located in this pod as a
      # sidecar and called over localhost (same topology the ingress gateway uses
      # for its dataplane + ext_proc).
      - name: ext_proc_server
        type: STATIC
        lb_policy: ROUND_ROBIN
        connect_timeout: 1s
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            explicit_http_config:
              http2_protocol_options: {}
        load_assignment:
          cluster_name: ext_proc_server
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: 127.0.0.1
                    port_value: 50051

      # The cleartext chain's upstream. The actor sent plaintext; this gateway
      # does not upgrade it to TLS.
      - name: egress_forward_proxy_cleartext
        lb_policy: CLUSTER_PROVIDED
        connect_timeout: 5s
        cluster_type:
          name: envoy.clusters.dynamic_forward_proxy
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.clusters.dynamic_forward_proxy.v3.ClusterConfig
            dns_cache_config:
              name: egress_dns_cache
              dns_lookup_family: ALL
            # Envoy refuses to build a dynamic forward proxy cluster without
            # auto_sni and auto_san_validation unless this is set. There is no
            # upstream TLS here, so both would be no-ops.
            allow_insecure_cluster_options: true
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            use_downstream_protocol_config:
              http_protocol_options: {}
              http2_protocol_options: {}

      # The cleartext chain's by-address upstream: the address the actor
      # dialed, from the same original_dst_address filter state the passthrough
      # chains use. Plaintext, like the forward proxy above.
      - name: egress_original_dst_cleartext
        type: ORIGINAL_DST
        lb_policy: CLUSTER_PROVIDED
        connect_timeout: 5s
        typed_extension_protocol_options:
          envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
            "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
            use_downstream_protocol_config:
              http_protocol_options: {}
              http2_protocol_options: {}

      # Passthrough resolves the approved SNI; the actor supplies only the port.
      - name: egress_forward_proxy_passthrough
        lb_policy: CLUSTER_PROVIDED
        connect_timeout: 5s
        cluster_type:
          name: envoy.clusters.dynamic_forward_proxy
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.clusters.dynamic_forward_proxy.v3.ClusterConfig
            dns_cache_config:
              name: egress_dns_cache
              dns_lookup_family: ALL

  # SDS resource for the gateway serving cert. kubelet rotates the projected
  # volume by swapping its ..data symlink, which fires the watch and makes
  # Envoy re-read the bundle (leaf cert and key concatenated in one file).
  {{- if $enabled }}
  sds-audit-client.yaml: |
    resources:
    - "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret
      name: obot_audit_client
      tls_certificate:
        certificate_chain: { filename: /run/podidentity.podcert.ate.dev/credential-bundle.pem }
        private_key: { filename: /run/podidentity.podcert.ate.dev/credential-bundle.pem }
        watched_directory: { path: /run/podidentity.podcert.ate.dev }
  sds-audit-server.yaml: |
    resources:
    - "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret
      name: obot_audit_server
      validation_context:
        trusted_ca: { filename: /run/servicedns.podcert.ate.dev/trust-bundle.pem }
        watched_directory: { path: /run/servicedns.podcert.ate.dev }
        match_typed_subject_alt_names:
        - san_type: DNS
          matcher:
            exact: {{ include "obot.fullname" . }}-egress-credentials.{{ .Release.Namespace }}.svc
  {{- end }}
  sds-servicedns-cert.yaml: |
    resources:
    - "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret
      name: servicedns_serving_cert
      tls_certificate:
        certificate_chain: { filename: /run/servicedns.podcert.ate.dev/credential-bundle.pem }
        private_key: { filename: /run/servicedns.podcert.ate.dev/credential-bundle.pem }
        watched_directory: { path: /run/servicedns.podcert.ate.dev }
{{ end -}}
