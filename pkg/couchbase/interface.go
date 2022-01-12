package couchbase

import "github.com/couchbase/tools-common/cbrest"

type NodeCommon interface {
	Close() error
	RestClient() *cbrest.Client
	Credentials() (string, string)
	GetServicePort(svc cbrest.Service) (int, error)
	HasService(svc cbrest.Service) (bool, error)
	Hostname() string
}
