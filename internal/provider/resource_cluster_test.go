package provider

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	apigen_mgmtv1 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v1"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	cloudsdk_mock "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/mock"
	"github.com/stretchr/testify/assert"
)

func createSimpleTestCluster(t *testing.T, name, region, imageTag string, tier apigen_mgmtv2.TierId, status apigen_mgmtv2.TenantStatus) *apigen_mgmtv2.Tenant {
	t.Helper()

	return &apigen_mgmtv2.Tenant{
		Id:         1,
		ImageTag:   imageTag,
		NsId:       uuid.Must(uuid.NewRandom()),
		Region:     region,
		TenantName: name,
		Tier:       tier,
		Status:     status,
		Resources: apigen_mgmtv2.TenantResource{
			Components: apigen_mgmtv2.TenantResourceComponents{
				Compactor: &apigen_mgmtv2.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
				Compute: &apigen_mgmtv2.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
				Frontend: &apigen_mgmtv2.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
				Meta: &apigen_mgmtv2.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
			},
		},
	}
}

func TestClusterCreate_previous_creation_failed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var (
		ctx      = context.Background()
		name     = "test-cluster"
		region   = "us-west-2"
		imageTag = "v1.10.0"
		tier     = apigen_mgmtv2.Standard
		tenant   = createSimpleTestCluster(t, name, region, imageTag, tier, apigen_mgmtv2.Failed)
	)

	client := cloudsdk_mock.NewMockCloudClientInterface(ctrl)

	dataHelper := NewMockDataExtractHelperInterface(ctrl)

	dataHelper.EXPECT().
		Get(ctx, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, getter DataGetter, target interface{}) diag.Diagnostics {
			p, ok := target.(*ClusterModel)
			assert.True(t, ok)
			clusterToDataModel(tenant, nil, p)
			return nil
		})
	dataHelper.EXPECT().
		Set(ctx, gomock.Any(), gomock.Any()).
		Return(nil)

	client.
		EXPECT().
		GetClusterByRegionAndName(ctx, region, name).
		Return(tenant, nil)

	client.
		EXPECT().
		DeleteClusterByNsIDAwait(ctx, tenant.NsId).
		Return(nil)

	client.
		EXPECT().
		GetAvailableComponentTypes(ctx, region, tier, gomock.Any()).
		Return([]apigen_mgmtv1.AvailableComponentType{
			{
				Id:      "p-1c4g",
				Maximum: 3,
				Cpu:     "1",
				Memory:  "4 GB",
			},
		}, nil).
		Times(5)

	client.
		EXPECT().
		CreateClusterAwait(ctx, region, gomock.Any()).
		DoAndReturn(func(ctx context.Context, region string, req apigen_mgmtv2.TenantRequestRequestBody) (*apigen_mgmtv2.Tenant, error) {
			rtn := *tenant
			rtn.Status = apigen_mgmtv2.Running
			return &rtn, nil
		})

	p := &ClusterResource{
		client:     client,
		dataHelper: dataHelper,
	}

	p.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{},
	}, &resource.CreateResponse{
		State: tfsdk.State{},
	})
}
