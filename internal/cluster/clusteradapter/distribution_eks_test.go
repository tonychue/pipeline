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

package clusteradapter

import (
	"context"
	"testing"

	"emperror.dev/errors"
	"github.com/stretchr/testify/require"

	"github.com/banzaicloud/pipeline/internal/cluster"
	"github.com/banzaicloud/pipeline/internal/cluster/distribution/awscommon"
	"github.com/banzaicloud/pipeline/internal/cluster/distribution/eks"
)

func TestEksServiceListNodePools(t *testing.T) {
	exampleEKSNodePools := []awscommon.NodePool{
		{
			Name: "cluster-node-pool-name-2",
			Labels: map[string]string{
				"label-1": "value-1",
				"label-2": "value-2",
			},
			Size: 4,
			Autoscaling: awscommon.Autoscaling{
				Enabled: true,
				MinSize: 1,
				MaxSize: 2,
			},
			VolumeSize:   40,
			InstanceType: "instance-type",
			Image:        "image",
			SpotPrice:    "5",
		},
		{
			Name: "cluster-node-pool-name-3",
			Labels: map[string]string{
				"label-3": "value-3",
			},
			Size: 6,
			Autoscaling: awscommon.Autoscaling{
				Enabled: false,
				MinSize: 0,
				MaxSize: 0,
			},
			VolumeSize:   50,
			InstanceType: "instance-type",
			Image:        "image",
			SpotPrice:    "7",
		},
	}
	exampleNodePools := make([]interface{}, 0, len(exampleEKSNodePools))
	for _, eksNodePool := range exampleEKSNodePools {
		exampleNodePools = append(exampleNodePools, eksNodePool)
	}

	type constructionArgumentType struct {
		service eks.Service
	}
	type functionCallArgumentType struct {
		ctx       context.Context
		clusterID uint
	}
	testCases := []struct {
		caseName              string
		constructionArguments constructionArgumentType
		expectedNodePools     cluster.RawNodePoolList
		expectedNotNilError   bool
		functionCallArguments functionCallArgumentType
		setupMockFunction     func(constructionArgumentType, functionCallArgumentType)
	}{
		{
			caseName: "ServiceListNodePoolsFailed",
			constructionArguments: constructionArgumentType{
				service: &eks.MockService{},
			},
			expectedNodePools:   nil,
			expectedNotNilError: true,
			functionCallArguments: functionCallArgumentType{
				ctx:       context.Background(),
				clusterID: 1,
			},
			setupMockFunction: func(
				constructionArguments constructionArgumentType,
				functionCallArguments functionCallArgumentType,
			) {
				eksServiceMock := constructionArguments.service.(*eks.MockService)
				eksServiceMock.On("ListNodePools", functionCallArguments.ctx, functionCallArguments.clusterID).Return(([]awscommon.NodePool)(nil), errors.NewPlain("ServiceListNodePoolsFailed"))
			},
		},
		{
			caseName: "EKSServiceListNodePoolsSuccess",
			constructionArguments: constructionArgumentType{
				service: &eks.MockService{},
			},
			expectedNodePools:   exampleNodePools,
			expectedNotNilError: false,
			functionCallArguments: functionCallArgumentType{
				ctx:       context.Background(),
				clusterID: 1,
			},
			setupMockFunction: func(
				constructionArguments constructionArgumentType,
				functionCallArguments functionCallArgumentType,
			) {
				eksServiceMock := constructionArguments.service.(*eks.MockService)
				eksServiceMock.On("ListNodePools", functionCallArguments.ctx, functionCallArguments.clusterID).Return(exampleEKSNodePools, (error)(nil))
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			testCase.setupMockFunction(testCase.constructionArguments, testCase.functionCallArguments)

			object := eksService{
				service: testCase.constructionArguments.service,
			}

			actualNodePools, actualError := object.ListNodePools(
				testCase.functionCallArguments.ctx,
				testCase.functionCallArguments.clusterID,
			)

			require.Truef(t, (actualError != nil) == testCase.expectedNotNilError,
				"error value doesn't match the expectation, is expected: %+v, actual error value: %+v", testCase.expectedNotNilError, actualError)
			require.Equal(t, testCase.expectedNodePools, actualNodePools)
		})
	}
}
