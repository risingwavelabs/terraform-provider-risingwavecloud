package provider

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
	cloudsdk_mock "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/mock"
	"github.com/stretchr/testify/assert"
)

func createSimpleTestCluster(t *testing.T, name, region, imageTag string, tier apigen_mgmt.TierId, status apigen_mgmt.TenantStatus) *apigen_mgmt.Tenant {
	t.Helper()

	return &apigen_mgmt.Tenant{
		Id:         1,
		ImageTag:   imageTag,
		NsId:       uuid.Must(uuid.NewRandom()),
		Region:     region,
		TenantName: name,
		Tier:       tier,
		Status:     status,
		Resources: apigen_mgmt.TenantResource{
			Components: apigen_mgmt.TenantResourceComponents{
				Compactor: &apigen_mgmt.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
				Compute: &apigen_mgmt.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
				Frontend: &apigen_mgmt.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
				Meta: &apigen_mgmt.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
			},
			MetaStore: &apigen_mgmt.TenantResourceMetaStore{
				Type: apigen_mgmt.Etcd,
				Etcd: &apigen_mgmt.MetaStoreEtcd{
					Resource: apigen_mgmt.ComponentResource{
						ComponentTypeId: "p-1c4g",
						Cpu:             "1",
						Memory:          "4 GB",
						Replica:         1,
					},
					SizeGb: 10,
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
		tier     = apigen_mgmt.Standard
		tenant   = createSimpleTestCluster(t, name, region, imageTag, tier, apigen_mgmt.Failed)
	)

	client := cloudsdk_mock.NewMockCloudClientInterface(ctrl)

	dataHelper := NewMockDataExtractHelperInterface(ctrl)

	dataHelper.EXPECT().
		Get(ctx, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, getter DataGetter, target interface{}) diag.Diagnostics {
			p, ok := target.(*ClusterModel)
			assert.True(t, ok)
			clusterToDataModel(tenant, p)
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
		Return([]apigen_mgmt.AvailableComponentType{
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
		DoAndReturn(func(ctx context.Context, region string, req apigen_mgmt.TenantRequestRequestBody) (*apigen_mgmt.Tenant, error) {
			rtn := *tenant
			rtn.Status = apigen_mgmt.Running
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
