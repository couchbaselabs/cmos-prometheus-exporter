package couchbase

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/couchbase/tools-common/aprov"
	"github.com/couchbase/tools-common/cbrest"
	"go.uber.org/zap"
	"io/ioutil"
)

type Node struct {
	hostname   string
	creds      aprov.Provider
	rest       *cbrest.Client
	ccm        *cbrest.ClusterConfigManager
	pollCtx    context.Context
	pollCancel context.CancelFunc
	logger     *zap.SugaredLogger
}

func (n *Node) Hostname() string {
	return n.hostname
}

func (n *Node) Close() error {
	n.pollCancel()
	n.rest.Close()
	return nil
}

func BootstrapNode(logger *zap.SugaredLogger, node, username, password string, mgmtPort int) (*Node, error) {
	creds := &aprov.Static{
		UserAgent: "yacpe/0.0.1",
		Username:  username,
		Password:  password,
	}
	client, err := cbrest.NewClient(cbrest.ClientOptions{
		ConnectionString: fmt.Sprintf("couchbase://%s:%d", node, mgmtPort),
		Provider:         creds,
		TLSConfig:        nil,
		DisableCCP:       true,
		ConnectionMode:   cbrest.ConnectionModeThisNodeOnly,
	})
	if err != nil {
		return nil, err
	}
	return &Node{
		hostname: node,
		rest:     client,
		creds:    creds,
		ccm:      cbrest.NewClusterConfigManager(),
		logger:   logger.Named(fmt.Sprintf("node[%s]", node)),
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
		Method:   "GET",
		Endpoint: cbrest.EndpointNodesServices,
		Service:  cbrest.ServiceManagement,
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

func (n *Node) pollClusterConfig() {
	for {
		n.ccm.WaitUntilExpired(n.pollCtx)
		if n.pollCtx.Err() != nil {
			return
		}
		if err := n.updateClusterConfig(); err != nil {
			n.logger.Fatalw("Failed to update cluster config", "err", err)
		}
	}
}

func (n *Node) GetServicePort(service cbrest.Service) (int, error) {
	cc := n.ccm.GetClusterConfig()
	if cc == nil {
		if err := n.updateClusterConfig(); err != nil {
			return -1, err
		}
		cc = n.ccm.GetClusterConfig()
	}
	return int(cc.BootstrapNode().GetPort(service, false, false)), nil
}

func (n *Node) HasService(service cbrest.Service) (bool, error) {
	cc := n.ccm.GetClusterConfig()
	if cc == nil {
		if err := n.updateClusterConfig(); err != nil {
			return false, err
		}
		cc = n.ccm.GetClusterConfig()
	}
	return cc.BootstrapNode().GetPort(service, false, false) > 0, nil
}
