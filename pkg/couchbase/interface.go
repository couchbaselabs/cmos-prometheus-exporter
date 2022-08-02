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
	"crypto/tls"

	"github.com/couchbase/tools-common/cbrest"
)

type NodeCommon interface {
	Close() error
	RestClient() *cbrest.Client
	Credentials() (string, string)
	GetServicePort(svc cbrest.Service) (int, error)
	HasService(svc cbrest.Service) (bool, error)
	Hostname() string
	TLSConfig() *tls.Config
}
