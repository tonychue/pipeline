// Copyright © 2018 Banzai Cloud
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

package ark

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	"github.com/banzaicloud/pipeline/internal/ark/client"
	"github.com/banzaicloud/pipeline/internal/ark/providers/amazon"
	"github.com/banzaicloud/pipeline/internal/ark/providers/azure"
	"github.com/banzaicloud/pipeline/internal/ark/providers/google"
	"github.com/banzaicloud/pipeline/internal/global"
	pkgErrors "github.com/banzaicloud/pipeline/pkg/errors"
	"github.com/banzaicloud/pipeline/pkg/providers"
	"github.com/banzaicloud/pipeline/src/secret"
)

// ChartConfig describes an ARK deployment chart config
type ChartConfig struct {
	Namespace      string
	Chart          string
	Name           string
	Version        string
	ValueOverrides []byte
}

// ValueOverrides describes values to be overridden in a deployment
type ValueOverrides struct {
	Configuration  configuration  `json:"configuration"`
	Credentials    credentials    `json:"credentials"`
	Image          image          `json:"image"`
	RBAC           rbac           `json:"rbac"`
	InitContainers []v1.Container `json:"initContainers"`
	CleanUpCRDs    bool           `json:"cleanUpCRDs"`
}

type rbac struct {
	Create bool `json:"create"`
}

type image struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	PullPolicy string `json:"pullPolicy"`
}

type credentials struct {
	SecretContents secretContents `json:"secretContents"`
}

type secretContents struct {
	azure.Secret
	// formerly Bucket
	Cloud   string `json:"cloud,omitempty"`
	Cluster string `json:"cluster,omitempty"`
}

type configuration struct {
	Provider               string                 `json:"provider"`
	VolumeSnapshotLocation volumeSnapshotLocation `json:"volumeSnapshotLocation"`
	BackupStorageLocation  backupStorageLocation  `json:"backupStorageLocation"`
	RestoreOnlyMode        bool                   `json:"restoreOnlyMode"`
	LogLevel               string                 `json:"logLevel"`
}

type volumeSnapshotLocation struct {
	Name     string                       `json:"name"`
	Provider string                       `json:"provider"`
	Config   volumeSnapshotLocationConfig `json:"config,omitempty"`
}

type volumeSnapshotLocationConfig struct {
	Region        string `json:"region,omitempty"`
	ApiTimeout    string `json:"apiTimeout,omitempty"`
	ResourceGroup string `json:"resourceGroup,omitempty"`
}

type backupStorageLocation struct {
	Name     string                      `json:"name"`
	Provider string                      `json:"provider"`
	Bucket   string                      `json:"bucket"`
	Prefix   string                      `json:"prefix"`
	Config   backupStorageLocationConfig `json:"config,omitempty"`
}

type backupStorageLocationConfig struct {
	Region                  string `json:"region,omitempty"`
	S3ForcePathStyle        string `json:"s3ForcePathStyle,omitempty"`
	S3Url                   string `json:"s3Url,omitempty"`
	KMSKeyId                string `json:"kmsKeyId,omitempty"`
	ResourceGroup           string `json:"resourceGroup,omitempty"`
	StorageAccount          string `json:"storageAccount,omitempty"`
	StorageAccountKeyEnvVar string `json:"storageAccountKeyEnvVar,omitempty"`
}

// ConfigRequest describes an ARK config request
type ConfigRequest struct {
	Cluster       clusterConfig
	ClusterSecret *secret.SecretItemResponse
	Bucket        bucketConfig
	BucketSecret  *secret.SecretItemResponse

	RestoreMode bool
}

type clusterConfig struct {
	Name         string
	Provider     string
	Distribution string
	Location     string
	RBACEnabled  bool

	azureClusterConfig
}

type azureClusterConfig struct {
	ResourceGroup string
}

type bucketConfig struct {
	Name     string
	Prefix   string
	Provider string
	Location string

	azureBucketConfig
}

type azureBucketConfig struct {
	StorageAccount string
	ResourceGroup  string
}

// GetChartConfig get a ChartConfig
func GetChartConfig() ChartConfig {
	return ChartConfig{
		Name:      "velero",
		Namespace: global.Config.Cluster.DisasterRecovery.Namespace,
		Chart:     global.Config.Cluster.DisasterRecovery.Charts.Ark.Chart,
		Version:   global.Config.Cluster.DisasterRecovery.Charts.Ark.Version,
	}
}

// Get gets helm deployment value overrides
func (req ConfigRequest) Get() (values ValueOverrides, err error) {
	var provider string
	switch req.Bucket.Provider {
	case providers.Amazon:
		provider = amazon.BackupStorageProvider
	case providers.Azure:
		provider = azure.BackupStorageProvider
	case providers.Google:
		provider = google.BackupStorageProvider
	default:
		return values, pkgErrors.ErrorNotSupportedCloudType
	}

	vsl, err := req.getVolumeSnapshotLocation()
	if err != nil {
		return values, err
	}

	bsp, err := req.getBackupStorageLocation()
	if err != nil {
		return values, err
	}

	cred, err := req.getCredentials()
	if err != nil {
		return values, err
	}

	initContainers := make([]v1.Container, 0, 2)

	if bsp.Provider == amazon.BackupStorageProvider || vsl.Provider == amazon.PersistentVolumeProvider {
		pluginImage := fmt.Sprintf("%s/%s", global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.AwsPluginImage.Repository,
			global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.AwsPluginImage.Tag)

		initContainers = append(initContainers, v1.Container{
			Name:            "velero-plugin-for-aws",
			Image:           pluginImage,
			ImagePullPolicy: getPullPolicy(global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.AwsPluginImage.PullPolicy),
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "plugins",
					MountPath: "/target",
				},
			},
		})
	}

	if bsp.Provider == google.BackupStorageProvider || vsl.Provider == google.PersistentVolumeProvider {
		pluginImage := fmt.Sprintf("%s/%s", global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.GcpPluginImage.Repository,
			global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.GcpPluginImage.Tag)

		initContainers = append(initContainers, v1.Container{
			Name:            "velero-plugin-for-gcp",
			Image:           pluginImage,
			ImagePullPolicy: getPullPolicy(global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.GcpPluginImage.PullPolicy),
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "plugins",
					MountPath: "/target",
				},
			},
		})
	}

	if bsp.Provider == azure.BackupStorageProvider || vsl.Provider == azure.PersistentVolumeProvider {
		pluginImage := fmt.Sprintf("%s/%s", global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.AzurePluginImage.Repository,
			global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.AzurePluginImage.Tag)

		initContainers = append(initContainers, v1.Container{
			Name:            "velero-plugin-for-azure",
			Image:           pluginImage,
			ImagePullPolicy: getPullPolicy(global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.AzurePluginImage.PullPolicy),
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "plugins",
					MountPath: "/target",
				},
			},
		})
	}

	return ValueOverrides{
		Configuration: configuration{
			Provider:               provider,
			VolumeSnapshotLocation: vsl,
			BackupStorageLocation:  bsp,
			RestoreOnlyMode:        req.RestoreMode,
			LogLevel:               "debug",
		},
		RBAC: rbac{
			Create: req.Cluster.RBACEnabled,
		},
		Credentials: cred,
		Image: image{
			Repository: global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.Image.Repository,
			Tag:        global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.Image.Tag,
			PullPolicy: global.Config.Cluster.DisasterRecovery.Charts.Ark.Values.Image.PullPolicy,
		},
		InitContainers: initContainers,
		CleanUpCRDs:    true,
	}, nil
}

func (req ConfigRequest) getVolumeSnapshotLocation() (volumeSnapshotLocation, error) {
	var config volumeSnapshotLocation
	var vslconfig volumeSnapshotLocationConfig
	var pvcProvider string

	switch req.Cluster.Provider {
	case providers.Amazon:
		pvcProvider = amazon.PersistentVolumeProvider
		vslconfig.Region = req.Cluster.Location
	case providers.Azure:
		pvcProvider = azure.PersistentVolumeProvider
		vslconfig.ApiTimeout = "3m0s"
		vslconfig.ResourceGroup = azure.GetAzureClusterResourceGroupName(req.Cluster.Distribution, req.Cluster.ResourceGroup, req.Cluster.Name, req.Cluster.Location)
	case providers.Google:
		pvcProvider = google.PersistentVolumeProvider
	default:
		return config, pkgErrors.ErrorNotSupportedCloudType
	}

	return volumeSnapshotLocation{
		Name:     client.DefaultVolumeSnapshotLocationName,
		Provider: pvcProvider,
		Config:   vslconfig,
	}, nil
}

func (req ConfigRequest) getBackupStorageLocation() (backupStorageLocation, error) {
	config := backupStorageLocation{
		Name:   client.DefaultBackupStorageLocationName,
		Bucket: req.Bucket.Name,
		Prefix: req.Bucket.Prefix,
	}

	switch req.Bucket.Provider {
	case providers.Amazon:
		config.Provider = amazon.BackupStorageProvider
		config.Config.Region = req.Bucket.Location

	case providers.Azure:
		config.Provider = azure.BackupStorageProvider
		config.Config.StorageAccount = req.Bucket.StorageAccount
		config.Config.ResourceGroup = req.Bucket.ResourceGroup
		config.Config.StorageAccountKeyEnvVar = "AZURE_STORAGE_KEY"

	case providers.Google:
		config.Provider = google.BackupStorageProvider

	default:
		return config, pkgErrors.ErrorNotSupportedCloudType
	}

	return config, nil
}

func (req ConfigRequest) getCredentials() (credentials, error) {
	var config credentials
	var BucketSecretContents string
	var ClusterSecretContents string
	var err error

	switch req.Cluster.Provider {
	case providers.Amazon:
		ClusterSecretContents, err = amazon.GetSecret(req.ClusterSecret)
		if err != nil {
			return config, err
		}
	case providers.Google:
		ClusterSecretContents, err = google.GetSecret(req.ClusterSecret)
		if err != nil {
			return config, err
		}
	case providers.Azure:
		crgName := azure.GetAzureClusterResourceGroupName(req.Cluster.Distribution, req.Cluster.ResourceGroup, req.Cluster.Name, req.Cluster.Location)
		ClusterSecretContents, err = azure.GetSecretForCluster(req.ClusterSecret, crgName)
		if err != nil {
			return config, err
		}
	default:
		return config, pkgErrors.ErrorNotSupportedCloudType
	}

	switch req.Bucket.Provider {
	case providers.Amazon:
		BucketSecretContents, err = amazon.GetSecret(req.BucketSecret)
		if err != nil {
			return config, err
		}
	case providers.Google:
		BucketSecretContents, err = google.GetSecret(req.BucketSecret)
		if err != nil {
			return config, err
		}
	case providers.Azure:
		crgName := azure.GetAzureClusterResourceGroupName(req.Cluster.Distribution, req.Cluster.ResourceGroup, req.Cluster.Name, req.Cluster.Location)
		BucketSecretContents, err = azure.GetSecretForBucket(req.BucketSecret, req.Bucket.StorageAccount, req.Bucket.ResourceGroup, crgName)
		if err != nil {
			return config, err
		}
	default:
		return config, pkgErrors.ErrorNotSupportedCloudType
	}

	return credentials{
		SecretContents: secretContents{
			Cluster: ClusterSecretContents,
			Cloud:   BucketSecretContents,
		},
	}, err
}

func getPullPolicy(pullPolicy string) v1.PullPolicy {
	switch pullPolicy {
	case string(v1.PullAlways):
		return v1.PullAlways

	case string(v1.PullNever):
		return v1.PullNever
	default:
		return v1.PullIfNotPresent
	}
}
