package model

import (
	"time"

	"github.com/banzaicloud/pipeline/database"
	pkgCluster "github.com/banzaicloud/pipeline/pkg/cluster"
	pkgErrors "github.com/banzaicloud/pipeline/pkg/errors"
	"github.com/banzaicloud/pipeline/pkg/providers/oracle/cluster"
)

// TableName constants
const (
	ClustersTableName                = "oracle_clusters"
	ClustersNodePoolsTableName       = "oracle_clusters_nodepools"
	ClustersNodePoolSubnetsTableName = "oracle_clusters_nodepools_subnets"
	ClustersNodePoolLabelsTableName  = "oracle_clusters_nodepools_labels"
)

// Cluster describes the Oracle cluster model
type Cluster struct {
	ID             uint   `gorm:"primary_key"`
	Name           string `gorm:"unique_index:idx_modelid_name"`
	Version        string
	VCNID          string
	LBSubnetID1    string
	LBSubnetID2    string
	OCID           string `gorm:"column:ocid"`
	ClusterModelID uint
	NodePools      []*NodePool
	CreatedBy      uint
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Delete         bool `gorm:"-"`
}

// NodePool describes Oracle node pools model of a cluster
type NodePool struct {
	ID                uint   `gorm:"primary_key"`
	Name              string `gorm:"unique_index:idx_clusterid_name"`
	Image             string `gorm:"default:'Oracle-Linux-7.4'"`
	Shape             string `gorm:"default:'VM.Standard1.1'"`
	Version           string `gorm:"default:'v1.10.3'"`
	QuantityPerSubnet uint   `gorm:"default:1"`
	OCID              string `gorm:"column:ocid"`
	ClusterID         uint   `gorm:"unique_index:idx_clusterid_name"`
	Subnets           []*NodePoolSubnet
	Labels            []*NodePoolLabel
	CreatedBy         uint
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Delete            bool `gorm:"-"`
	Add               bool `gorm:"-"`
}

// NodePoolSubnet describes subnets for a NodePool
type NodePoolSubnet struct {
	ID         uint   `gorm:"primary_key"`
	SubnetID   string `gorm:"unique_index:idx_nodepoolid_subnetid"`
	NodePoolID uint   `gorm:"unique_index:idx_nodepoolid_subnetid"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NodePoolLabel stores labels for node pools
type NodePoolLabel struct {
	ID         uint   `gorm:"primary_key"`
	Name       string `gorm:"unique_index:idx_nodepoolid_name"`
	Value      string
	NodePoolID uint `gorm:"unique_index:idx_nodepoolid_name"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TableName sets the Clusters table name
func (Cluster) TableName() string {
	return ClustersTableName
}

// TableName sets the NodePools table name
func (NodePool) TableName() string {
	return ClustersNodePoolsTableName
}

// TableName sets the NodePoolSubnets table name
func (NodePoolSubnet) TableName() string {
	return ClustersNodePoolSubnetsTableName
}

// TableName sets the NodePoolLabels table name
func (NodePoolLabel) TableName() string {
	return ClustersNodePoolLabelsTableName
}

// CreateModelFromCreateRequest create model from create request
func CreateModelFromCreateRequest(r *pkgCluster.CreateClusterRequest, userId uint) (cluster Cluster, err error) {

	cluster.Name = r.Name

	return CreateModelFromRequest(cluster, r.Properties.CreateClusterOKE, userId)
}

// CreateModelFromUpdateRequest create model from update request
func CreateModelFromUpdateRequest(current Cluster, r *pkgCluster.UpdateClusterRequest, userId uint) (cluster Cluster, err error) {

	return CreateModelFromRequest(current, r.UpdateProperties.OKE, userId)
}

// CreateModelFromRequest creates model from request
func CreateModelFromRequest(model Cluster, r *cluster.Cluster, userID uint) (cluster Cluster, err error) {

	model.Version = r.Version
	model.CreatedBy = userID

	// reqest values only used when creating
	if model.ID == 0 {
		model.VCNID = r.GetVCNID()
		model.LBSubnetID1 = r.GetLBSubnetID1()
		model.LBSubnetID2 = r.GetLBSubnetID2()
		model.CreatedBy = userID
	}

	// there should be at least 1 node pool defined
	if len(r.NodePools) == 0 {
		return cluster, pkgErrors.ErrorNodePoolNotProvided
	}

	nodePools := make([]*NodePool, 0)
	for name, data := range r.NodePools {
		nodePool := model.GetNodePoolByName(name)
		if nodePool.ID == 0 {
			nodePool.Name = name
			nodePool.Image = data.Image
			nodePool.Shape = data.Shape
			nodePool.Add = true
		} else {
			nodePool.Subnets = make([]*NodePoolSubnet, 0)
			nodePool.Labels = make([]*NodePoolLabel, 0)
		}
		nodePool.CreatedBy = userID
		nodePool.Version = data.Version
		nodePool.QuantityPerSubnet = data.GetQuantityPerSubnet()

		for _, subnetID := range data.GetSubnetIDs() {
			nodePool.Subnets = append(nodePool.Subnets, &NodePoolSubnet{
				SubnetID: subnetID,
			})
		}

		for name, value := range data.Labels {
			nodePool.Labels = append(nodePool.Labels, &NodePoolLabel{
				Name:  name,
				Value: value,
			})
		}

		nodePools = append(nodePools, nodePool)
	}

	for _, np := range model.NodePools {
		if r.NodePools[np.Name] == nil {
			np.Delete = true
			nodePools = append(nodePools, np)
		}
	}

	model.NodePools = nodePools

	return model, err
}

// GetNodePoolByName gets a NodePool from the []NodePools by name
func (c *Cluster) GetNodePoolByName(name string) *NodePool {

	for _, np := range c.NodePools {
		if np.Name == name {
			return np
		}
	}

	return &NodePool{}
}

// Cleanup removes node pools
func (c *Cluster) Cleanup() error {

	log.Info("Cleanup oracle nodepool... delete all node pools")

	err := c.RemoveNodePools()
	if err != nil {
		return err
	}

	db := database.GetDB()
	return db.Delete(&c).Error
}

// BeforeDelete deletes all subnets and labels belongs to the nodepool
func (d *NodePool) BeforeDelete() error {
	log.Info("BeforeDelete oracle nodepool... delete all subnets and labels")

	var nodePoolSubnets []*NodePoolSubnet
	var nodePoolLabels []*NodePoolLabel

	err := database.GetDB().Where(NodePoolSubnet{
		NodePoolID: d.ID,
	}).Find(&nodePoolSubnets).Delete(&nodePoolSubnets).Error
	if err != nil {
		return err
	}

	return database.GetDB().Where(NodePoolLabel{
		NodePoolID: d.ID,
	}).Find(&nodePoolLabels).Delete(&nodePoolLabels).Error
}

// RemoveNodePools delete node pool records from the database
func (c *Cluster) RemoveNodePools() error {

	if c.ID == 0 {
		return nil
	}

	var nodePools []*NodePool
	err := database.GetDB().Where(NodePool{
		ClusterID: c.ID,
	}).Find(&nodePools).Delete(&nodePools).Error
	if err != nil {
		log.Errorf("Error during deleting saved nodepools: %s", err.Error())
	}

	return nil
}

// BeforeSave clears nodepools
func (c *Cluster) BeforeSave() error {
	log.Info("BeforeSave oracle cluster...")

	c.RemoveNodePools()

	log.Info("BeforeSave oracle cluster...done")

	return nil
}

// GetClusterRequestFromModel converts cluster model from database and to Cluster
func (c *Cluster) GetClusterRequestFromModel() *cluster.Cluster {

	nodePools := make(map[string]*cluster.NodePool)
	if c.NodePools != nil {
		for _, np := range c.NodePools {
			nodePools[np.Name] = &cluster.NodePool{
				Version: np.Version,
				Image:   np.Image,
				Count:   uint(int(np.QuantityPerSubnet) * len(np.Subnets)),
				Shape:   np.Shape,
			}
			nodePools[np.Name].Labels = make(map[string]string, 0)
			for _, l := range np.Labels {
				nodePools[np.Name].Labels[l.Name] = l.Value
			}
		}
	}

	return &cluster.Cluster{
		Version:   c.Version,
		NodePools: nodePools,
	}
}
