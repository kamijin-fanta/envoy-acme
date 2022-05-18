# envoy-acme

Obtain the certificate from Let's encrypt and configure it on the Envoy Proxy through SDS.

Currently, only DNS01 using Lego is supported. https://go-acme.github.io/lego/dns/


## Commands usage

`envoy-acme --help`

```
NAME:
   envoy-acme - A new cli application

USAGE:
   envoy-acme [global options] command [command options] [arguments...]

COMMANDS:
   start    start sds server
   export   export cert, keys file from store
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --log-level value            (default: "info") [$LOG_LEVEL]
   --log-format value           (default: "text") [$LOG_FORMAT]
   --store value                (default: "consul") [$STORE]
   --store-file-base value      (default: "./data") [$STORE_FILE_BASE]
   --store-consul-prefix value  (default: "envoy-acme/default") [$STORE_CONSUL_PREFIX]
   --help, -h                   show help (default: false)
```

`envoy-acme start --help`

```
NAME:
   envoy-acme start - start sds server

USAGE:
   envoy-acme start [command options] [arguments...]

OPTIONS:
   --ca-dir value            (default: "https://acme-staging-v02.api.letsencrypt.org/directory") [$CA_DIR]
   --cert-days value         (default: 25) [$CERT_DAYS]
   --xds-listen value        (default: "127.0.0.1:20000") [$XDS_LISTEN]
   --interval value          (default: 1h0m0s) [$INTERVAL]
   --lock-timeout value      (default: 10m0s) [$LOCK_TIMEOUT]
   --config value, -c value  (default: "sites.yaml") [$CONFIG_FILE]
   --metrics-listen value    (default: "127.0.0.1:20001") [$METRICS_LISTEN]
   --help, -h                show help (default: false)
```

`envoy-acme start --help`

```
NAME:
   envoy-acme export - export cert, keys file from store

USAGE:
   envoy-acme export [command options] [arguments...]

OPTIONS:
   --name value  target configure name
   --dest value  output directory (default: ".")
   --help, -h    show help (default: false)
```

## Configs

### Sites config

```yaml
# site.yaml
sites:
  - name: setting-names     # It will be the setting name of SDS
    provider: route53   # DNS-01 provider name in Lego
    email: test@you.com     # Your email address
    domains:                # Target domains
      - "example.com"
      - "*.example.com"
    legoenv:                # Environment variables required by the provider
      - AWS_REGION=US-EAST-1=********-****-****-****-**********
      - AWS_ACCESS_KEY_ID=XXXXXXXXXXXXXXXXX
      - AWS_SECRET_ACCESS_KEY=****************************************************************
      - AWS_HOSTED_ZONE_ID=XXXXXXXXXXXXXXXX

```

### Dot env file

```env
.env

LOG_LEVEL=debug  # For more information --help
```

### Example envoy.yaml

```yaml
static_resources:
  listeners:
  - name: listener_0
    address:
      socket_address: { address: 0.0.0.0, port_value: 80 }
    filter_chains:
    - filters:
      - name: envoy.http_connection_manager
        config:
          stat_prefix: ingress_http
          route_config:
            name: route
            virtual_hosts:
            - name: app_service
              domains: ["*"]
              routes:
              - match: { prefix: "/" }
                direct_response:
                  status: 200
                  body:
                    inline_string: hello envoy
          http_filters:
          - name: envoy.router
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
          common_tls_context:
            tls_certificate_sds_secret_configs:
            - name: "setting-name"
              sds_config:
                resource_api_version: v3
                api_config_source:
                  api_type: GRPC
                  transport_api_version: v3
                  grpc_services:
                    envoy_grpc:
                      cluster_name: envoy_acme_sds_cluster
  clusters:
  - name: envoy_acme_sds_cluster
    connect_timeout: 0.25s
    http2_protocol_options: {}
    load_assignment:
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address: {address: 127.0.0.1, port_value: 20000 }
```
