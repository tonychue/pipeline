// Copyright © 2020 Banzai Cloud
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

package pkeservice

import (
	"context"
	"time"

	"github.com/banzaicloud/pipeline/internal/cluster"
	"logur.dev/logur"
)

type Cluster = cluster.Cluster

type NodeStatus struct {
	// name of node
	Name string

	// name of nodepool
	NodePool string

	// ip address of node (where the other nodes can reach it)
	Ip string

	// detailed description about the current bootstrapping status (including the cause of the failure)
	Message string

	// the current phase of the bootstrap process
	Phase string

	// if the installation process is finished (either with success or failure)
	Finished bool

	// if a fatal failure occurred (i.e. the node will not come up)
	Failure bool

	// exact time of event
	Timestamp time.Time
}

// +kit:endpoint:errorStrategy=service
// +testify:mock

// Service provides an interface to PKE specific endpoints
type Service interface {
	// RegisterNodeStatus registers status reported by a node
	RegisterNodeStatus(ctx context.Context, clusterIdentifier cluster.Identifier, nodeStatus NodeStatus) (err error)
}

type service struct {
	clusters  Store
	processes process
	logger    logur.Logger
}

func (s service) RegisterNodeStatus(ctx context.Context, clusterIdentifier cluster.Identifier, nodeStatus NodeStatus) (err error) {
	s.logger.Info("node status update", map[string]interface{}{
		"clusterID":  clusterIdentifier.ClusterID,
		"nodeName":   nodeStatus.Name,
		"nodeIP":     nodeStatus.Ip,
		"nodePool":   nodeStatus.NodePool,
		"remoteTime": nodeStatus.Timestamp,
		"phase":      nodeStatus.Phase,
		"message":    nodeStatus.Message,
	})

	return nil
}

// Store allows looking up clusters form persistent storage
type Store interface {
	// GetCluster returns a generic Cluster.
	// Returns an error with the NotFound behavior when the cluster cannot be found.
	GetCluster(ctx context.Context, id uint) (Cluster, error)
}

// NewService returns a new Service instance
func NewService(
	clusters cluster.Store,
	logger logur.Logger,
) Service {
	return service{
		clusters: clusters,
		logger:   logger,
	}
}
