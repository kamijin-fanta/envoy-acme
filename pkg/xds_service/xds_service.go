package xds_service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/kamijin-fanta/envoy-acme/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"net"
	"time"
)

var (
	xdsStreamOpenCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: common.PrometheusNamespace,
		Name:      "xds_stream_open",
	})
)

type XdsService struct {
	logger *logrus.Entry
}

func NewXdsService(logger *logrus.Logger) *XdsService {
	svc := &XdsService{
		logger: logger.WithField("component", "xds_service"),
	}
	return svc
}

var _ cache.NodeHash = &StandardNodeHash{}

type StandardNodeHash struct{}

func (s *StandardNodeHash) ID(node *envoy_config_core_v3.Node) string {
	return "default"
}
func (x *XdsService) RunServer(ctx context.Context, listener net.Listener, update chan *common.Notification) error {
	callback := server.CallbackFuncs{
		StreamOpenFunc: func(ctx context.Context, i int64, s string) error {
			md, ok := metadata.FromIncomingContext(ctx)
			if ok {
				by, _ := json.Marshal(md)
				x.logger.WithField("request", string(by)).Debugf("stream request")
			}
			xdsStreamOpenCounter.Inc()
			return nil
		},
		StreamClosedFunc:   nil,
		StreamRequestFunc:  nil,
		StreamResponseFunc: nil,
		FetchRequestFunc:   nil,
		FetchResponseFunc:  nil,
	}
	snapshotCache := cache.NewSnapshotCache(false, &StandardNodeHash{}, nil)
	srv := server.NewServer(ctx, snapshotCache, callback)

	go func() {
		for {
			upstreams := <-update
			err := snapshotCache.SetSnapshot("default", generateSnapshot(upstreams))
			if err != nil {
				panic(err)
			}
		}
	}()

	grpcServer := grpc.NewServer()
	envoy_service_secret_v3.RegisterSecretDiscoveryServiceServer(grpcServer, srv)

	x.logger.WithField("addr", listener.Addr().String()).Info("start server")
	return grpcServer.Serve(listener)
}

func generateSnapshot(notification *common.Notification) cache.Snapshot {
	var resources []types.Resource

	for _, cert := range notification.Certificates {
		secret := &envoy_extensions_transport_sockets_tls_v3.Secret{
			Name: cert.Domain,
			Type: &envoy_extensions_transport_sockets_tls_v3.Secret_TlsCertificate{
				TlsCertificate: &envoy_extensions_transport_sockets_tls_v3.TlsCertificate{
					CertificateChain: &envoy_config_core_v3.DataSource{
						Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
							InlineBytes: cert.Certificate,
						},
					},
					PrivateKey: &envoy_config_core_v3.DataSource{
						Specifier: &envoy_config_core_v3.DataSource_InlineBytes{
							InlineBytes: cert.PrivateKey,
						},
					},
				},
			},
		}
		resources = append(resources, secret)
	}

	return cache.NewSnapshot(fmt.Sprintf("%d", time.Now().Unix()), nil, nil, nil, nil, nil, resources)
}
