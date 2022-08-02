// Copyright 2022 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package couchbase

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/couchbase/tools-common/aprov"
	"github.com/couchbase/tools-common/cbrest"
	"go.uber.org/zap"

	"github.com/couchbaselabs/cmos-prometheus-exporter/pkg/meta"
)

type Node struct {
	hostname  string
	creds     aprov.Provider
	rest      *cbrest.Client
	ccm       *cbrest.ClusterConfigManager
	logger    *zap.SugaredLogger
	tlsConfig *tls.Config
}

func (n *Node) Hostname() string {
	return n.hostname
}

func (n *Node) Close() error {
	n.rest.Close()
	return nil
}

func (n *Node) TLSConfig() *tls.Config {
	return n.tlsConfig
}

type BootstrapNodeOptions struct {
	ConnectionString                    string
	Username, Password                  string
	CACertFile, ClientCertFile, KeyFile string
	InsecureSkipVerify                  bool
}

func BootstrapNode(logger *zap.SugaredLogger, opts BootstrapNodeOptions) (*Node, error) {
	creds := &aprov.Static{
		UserAgent: fmt.Sprintf("cmos-exporter/%s", meta.Version),
		Username:  opts.Username,
		Password:  opts.Password,
	}
	var tlsConfig *tls.Config
	if opts.CACertFile != "" {
		caCertData, err := ioutil.ReadFile(opts.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CACertFile %q: %w", opts.CACertFile, err)
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(caCertData); !ok {
			return nil, fmt.Errorf("failed to parse CACertFile %q", opts.CACertFile)
		}
		tlsConfig = &tls.Config{
			RootCAs: pool,
		}
	} else if opts.InsecureSkipVerify {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: opts.InsecureSkipVerify,
		}
	} else {
		systemPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to load system cert pool: %w", err)
		}
		tlsConfig = &tls.Config{
			RootCAs: systemPool,
		}
	}
	if opts.ClientCertFile != "" && opts.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(opts.ClientCertFile, opts.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read client certs: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	client, err := cbrest.NewClient(cbrest.ClientOptions{
		ConnectionString: opts.ConnectionString,
		Provider:         creds,
		TLSConfig:        tlsConfig,
		DisableCCP:       true,
		ConnectionMode:   cbrest.ConnectionModeThisNodeOnly,
	})
	if err != nil {
		return nil, err
	}
	nodes := client.Nodes()
	if len(nodes) < 1 {
		return nil, fmt.Errorf("internal error: cluster has no nodes")
	}
	return &Node{
		hostname:  nodes[0].Hostname,
		rest:      client,
		creds:     creds,
		ccm:       cbrest.NewClusterConfigManager(),
		logger:    logger.Named(fmt.Sprintf("node[%s]", nodes[0].Hostname)),
		tlsConfig: tlsConfig,
	}, nil
}

func (n *Node) RestClient() *cbrest.Client {
	return n.rest
}

func (n *Node) Credentials() (string, string) {
	return n.creds.GetCredentials("")
}

func (n *Node) updateClusterConfig() error {
	res, err := n.rest.Do(context.TODO(), &cbrest.Request{
		Method:             "GET",
		Endpoint:           cbrest.EndpointNodesServices,
		Service:            cbrest.ServiceManagement,
		ExpectedStatusCode: http.StatusOK,
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var result cbrest.ClusterConfig
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	result.FilterOtherNodes()
	return n.ccm.Update(&result)
}

func (n *Node) GetServicePort(service cbrest.Service) (int, error) {
	cc := n.ccm.GetClusterConfig()
	if cc == nil {
		if err := n.updateClusterConfig(); err != nil {
			return -1, err
		}
		cc = n.ccm.GetClusterConfig()
	}
	return int(cc.BootstrapNode().GetPort(service, n.rest.TLS(), n.rest.AltAddr())), nil
}

func (n *Node) HasService(service cbrest.Service) (bool, error) {
	cc := n.ccm.GetClusterConfig()
	if cc == nil {
		if err := n.updateClusterConfig(); err != nil {
			return false, err
		}
		cc = n.ccm.GetClusterConfig()
	}
	return cc.BootstrapNode().GetPort(service, n.rest.TLS(), n.rest.AltAddr()) > 0, nil
}
