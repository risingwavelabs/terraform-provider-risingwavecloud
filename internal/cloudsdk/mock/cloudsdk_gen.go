// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk (interfaces: CloudClientInterface)

// Package mock is a generated GoMock package.
package mock

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	uuid "github.com/google/uuid"
	cloudsdk "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
)

// MockCloudClientInterface is a mock of CloudClientInterface interface.
type MockCloudClientInterface struct {
	ctrl     *gomock.Controller
	recorder *MockCloudClientInterfaceMockRecorder
}

// MockCloudClientInterfaceMockRecorder is the mock recorder for MockCloudClientInterface.
type MockCloudClientInterfaceMockRecorder struct {
	mock *MockCloudClientInterface
}

// NewMockCloudClientInterface creates a new mock instance.
func NewMockCloudClientInterface(ctrl *gomock.Controller) *MockCloudClientInterface {
	mock := &MockCloudClientInterface{ctrl: ctrl}
	mock.recorder = &MockCloudClientInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCloudClientInterface) EXPECT() *MockCloudClientInterfaceMockRecorder {
	return m.recorder
}

// CreateCluserUser mocks base method.
func (m *MockCloudClientInterface) CreateCluserUser(arg0 context.Context, arg1 uuid.UUID, arg2, arg3 string, arg4, arg5 bool) (*apigen.DBUser, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateCluserUser", arg0, arg1, arg2, arg3, arg4, arg5)
	ret0, _ := ret[0].(*apigen.DBUser)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateCluserUser indicates an expected call of CreateCluserUser.
func (mr *MockCloudClientInterfaceMockRecorder) CreateCluserUser(arg0, arg1, arg2, arg3, arg4, arg5 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateCluserUser", reflect.TypeOf((*MockCloudClientInterface)(nil).CreateCluserUser), arg0, arg1, arg2, arg3, arg4, arg5)
}

// CreateClusterAwait mocks base method.
func (m *MockCloudClientInterface) CreateClusterAwait(arg0 context.Context, arg1 string, arg2 apigen.TenantRequestRequestBody) (*apigen.Tenant, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateClusterAwait", arg0, arg1, arg2)
	ret0, _ := ret[0].(*apigen.Tenant)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateClusterAwait indicates an expected call of CreateClusterAwait.
func (mr *MockCloudClientInterfaceMockRecorder) CreateClusterAwait(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateClusterAwait", reflect.TypeOf((*MockCloudClientInterface)(nil).CreateClusterAwait), arg0, arg1, arg2)
}

// CreatePrivateLinkAwait mocks base method.
func (m *MockCloudClientInterface) CreatePrivateLinkAwait(arg0 context.Context, arg1 uuid.UUID, arg2 apigen.PostPrivateLinkRequestBody) (*cloudsdk.PrivateLinkInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreatePrivateLinkAwait", arg0, arg1, arg2)
	ret0, _ := ret[0].(*cloudsdk.PrivateLinkInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreatePrivateLinkAwait indicates an expected call of CreatePrivateLinkAwait.
func (mr *MockCloudClientInterfaceMockRecorder) CreatePrivateLinkAwait(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreatePrivateLinkAwait", reflect.TypeOf((*MockCloudClientInterface)(nil).CreatePrivateLinkAwait), arg0, arg1, arg2)
}

// DeleteClusterByNsIDAwait mocks base method.
func (m *MockCloudClientInterface) DeleteClusterByNsIDAwait(arg0 context.Context, arg1 uuid.UUID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteClusterByNsIDAwait", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteClusterByNsIDAwait indicates an expected call of DeleteClusterByNsIDAwait.
func (mr *MockCloudClientInterfaceMockRecorder) DeleteClusterByNsIDAwait(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteClusterByNsIDAwait", reflect.TypeOf((*MockCloudClientInterface)(nil).DeleteClusterByNsIDAwait), arg0, arg1)
}

// DeleteClusterUser mocks base method.
func (m *MockCloudClientInterface) DeleteClusterUser(arg0 context.Context, arg1 uuid.UUID, arg2 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteClusterUser", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteClusterUser indicates an expected call of DeleteClusterUser.
func (mr *MockCloudClientInterfaceMockRecorder) DeleteClusterUser(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteClusterUser", reflect.TypeOf((*MockCloudClientInterface)(nil).DeleteClusterUser), arg0, arg1, arg2)
}

// DeletePrivateLinkAwait mocks base method.
func (m *MockCloudClientInterface) DeletePrivateLinkAwait(arg0 context.Context, arg1, arg2 uuid.UUID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeletePrivateLinkAwait", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeletePrivateLinkAwait indicates an expected call of DeletePrivateLinkAwait.
func (mr *MockCloudClientInterfaceMockRecorder) DeletePrivateLinkAwait(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeletePrivateLinkAwait", reflect.TypeOf((*MockCloudClientInterface)(nil).DeletePrivateLinkAwait), arg0, arg1, arg2)
}

// GetAvailableComponentTypes mocks base method.
func (m *MockCloudClientInterface) GetAvailableComponentTypes(arg0 context.Context, arg1 string, arg2 apigen.TierId, arg3 string) ([]apigen.AvailableComponentType, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAvailableComponentTypes", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].([]apigen.AvailableComponentType)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAvailableComponentTypes indicates an expected call of GetAvailableComponentTypes.
func (mr *MockCloudClientInterfaceMockRecorder) GetAvailableComponentTypes(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAvailableComponentTypes", reflect.TypeOf((*MockCloudClientInterface)(nil).GetAvailableComponentTypes), arg0, arg1, arg2, arg3)
}

// GetClusterByNsID mocks base method.
func (m *MockCloudClientInterface) GetClusterByNsID(arg0 context.Context, arg1 uuid.UUID) (*apigen.Tenant, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClusterByNsID", arg0, arg1)
	ret0, _ := ret[0].(*apigen.Tenant)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClusterByNsID indicates an expected call of GetClusterByNsID.
func (mr *MockCloudClientInterfaceMockRecorder) GetClusterByNsID(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClusterByNsID", reflect.TypeOf((*MockCloudClientInterface)(nil).GetClusterByNsID), arg0, arg1)
}

// GetClusterByRegionAndName mocks base method.
func (m *MockCloudClientInterface) GetClusterByRegionAndName(arg0 context.Context, arg1, arg2 string) (*apigen.Tenant, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClusterByRegionAndName", arg0, arg1, arg2)
	ret0, _ := ret[0].(*apigen.Tenant)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClusterByRegionAndName indicates an expected call of GetClusterByRegionAndName.
func (mr *MockCloudClientInterfaceMockRecorder) GetClusterByRegionAndName(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClusterByRegionAndName", reflect.TypeOf((*MockCloudClientInterface)(nil).GetClusterByRegionAndName), arg0, arg1, arg2)
}

// GetClusterUser mocks base method.
func (m *MockCloudClientInterface) GetClusterUser(arg0 context.Context, arg1 uuid.UUID, arg2 string) (*apigen.DBUser, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClusterUser", arg0, arg1, arg2)
	ret0, _ := ret[0].(*apigen.DBUser)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClusterUser indicates an expected call of GetClusterUser.
func (mr *MockCloudClientInterfaceMockRecorder) GetClusterUser(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClusterUser", reflect.TypeOf((*MockCloudClientInterface)(nil).GetClusterUser), arg0, arg1, arg2)
}

// GetPrivateLink mocks base method.
func (m *MockCloudClientInterface) GetPrivateLink(arg0 context.Context, arg1 uuid.UUID) (*cloudsdk.PrivateLinkInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPrivateLink", arg0, arg1)
	ret0, _ := ret[0].(*cloudsdk.PrivateLinkInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPrivateLink indicates an expected call of GetPrivateLink.
func (mr *MockCloudClientInterfaceMockRecorder) GetPrivateLink(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPrivateLink", reflect.TypeOf((*MockCloudClientInterface)(nil).GetPrivateLink), arg0, arg1)
}

// GetPrivateLinkByName mocks base method.
func (m *MockCloudClientInterface) GetPrivateLinkByName(arg0 context.Context, arg1 string) (*cloudsdk.PrivateLinkInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPrivateLinkByName", arg0, arg1)
	ret0, _ := ret[0].(*cloudsdk.PrivateLinkInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPrivateLinkByName indicates an expected call of GetPrivateLinkByName.
func (mr *MockCloudClientInterfaceMockRecorder) GetPrivateLinkByName(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPrivateLinkByName", reflect.TypeOf((*MockCloudClientInterface)(nil).GetPrivateLinkByName), arg0, arg1)
}

// GetPrivateLinks mocks base method.
func (m *MockCloudClientInterface) GetPrivateLinks(arg0 context.Context) ([]cloudsdk.PrivateLinkInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPrivateLinks", arg0)
	ret0, _ := ret[0].([]cloudsdk.PrivateLinkInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPrivateLinks indicates an expected call of GetPrivateLinks.
func (mr *MockCloudClientInterfaceMockRecorder) GetPrivateLinks(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPrivateLinks", reflect.TypeOf((*MockCloudClientInterface)(nil).GetPrivateLinks), arg0)
}

// GetTiers mocks base method.
func (m *MockCloudClientInterface) GetTiers(arg0 context.Context, arg1 string) ([]apigen.Tier, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTiers", arg0, arg1)
	ret0, _ := ret[0].([]apigen.Tier)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTiers indicates an expected call of GetTiers.
func (mr *MockCloudClientInterfaceMockRecorder) GetTiers(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTiers", reflect.TypeOf((*MockCloudClientInterface)(nil).GetTiers), arg0, arg1)
}

// IsTenantNameExist mocks base method.
func (m *MockCloudClientInterface) IsTenantNameExist(arg0 context.Context, arg1, arg2 string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsTenantNameExist", arg0, arg1, arg2)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// IsTenantNameExist indicates an expected call of IsTenantNameExist.
func (mr *MockCloudClientInterfaceMockRecorder) IsTenantNameExist(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsTenantNameExist", reflect.TypeOf((*MockCloudClientInterface)(nil).IsTenantNameExist), arg0, arg1, arg2)
}

// Ping mocks base method.
func (m *MockCloudClientInterface) Ping(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Ping", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Ping indicates an expected call of Ping.
func (mr *MockCloudClientInterfaceMockRecorder) Ping(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Ping", reflect.TypeOf((*MockCloudClientInterface)(nil).Ping), arg0)
}

// UpdateClusterImageByNsIDAwait mocks base method.
func (m *MockCloudClientInterface) UpdateClusterImageByNsIDAwait(arg0 context.Context, arg1 uuid.UUID, arg2 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateClusterImageByNsIDAwait", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateClusterImageByNsIDAwait indicates an expected call of UpdateClusterImageByNsIDAwait.
func (mr *MockCloudClientInterfaceMockRecorder) UpdateClusterImageByNsIDAwait(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateClusterImageByNsIDAwait", reflect.TypeOf((*MockCloudClientInterface)(nil).UpdateClusterImageByNsIDAwait), arg0, arg1, arg2)
}

// UpdateClusterResourcesByNsIDAwait mocks base method.
func (m *MockCloudClientInterface) UpdateClusterResourcesByNsIDAwait(arg0 context.Context, arg1 uuid.UUID, arg2 apigen.PostTenantResourcesRequestBody) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateClusterResourcesByNsIDAwait", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateClusterResourcesByNsIDAwait indicates an expected call of UpdateClusterResourcesByNsIDAwait.
func (mr *MockCloudClientInterfaceMockRecorder) UpdateClusterResourcesByNsIDAwait(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateClusterResourcesByNsIDAwait", reflect.TypeOf((*MockCloudClientInterface)(nil).UpdateClusterResourcesByNsIDAwait), arg0, arg1, arg2)
}

// UpdateClusterUserPassword mocks base method.
func (m *MockCloudClientInterface) UpdateClusterUserPassword(arg0 context.Context, arg1 uuid.UUID, arg2, arg3 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateClusterUserPassword", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateClusterUserPassword indicates an expected call of UpdateClusterUserPassword.
func (mr *MockCloudClientInterfaceMockRecorder) UpdateClusterUserPassword(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateClusterUserPassword", reflect.TypeOf((*MockCloudClientInterface)(nil).UpdateClusterUserPassword), arg0, arg1, arg2, arg3)
}

// UpdateEtcdConfigByNsIDAwait mocks base method.
func (m *MockCloudClientInterface) UpdateEtcdConfigByNsIDAwait(arg0 context.Context, arg1 uuid.UUID, arg2 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateEtcdConfigByNsIDAwait", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateEtcdConfigByNsIDAwait indicates an expected call of UpdateEtcdConfigByNsIDAwait.
func (mr *MockCloudClientInterfaceMockRecorder) UpdateEtcdConfigByNsIDAwait(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateEtcdConfigByNsIDAwait", reflect.TypeOf((*MockCloudClientInterface)(nil).UpdateEtcdConfigByNsIDAwait), arg0, arg1, arg2)
}

// UpdateRisingWaveConfigByNsIDAwait mocks base method.
func (m *MockCloudClientInterface) UpdateRisingWaveConfigByNsIDAwait(arg0 context.Context, arg1 uuid.UUID, arg2 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateRisingWaveConfigByNsIDAwait", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateRisingWaveConfigByNsIDAwait indicates an expected call of UpdateRisingWaveConfigByNsIDAwait.
func (mr *MockCloudClientInterfaceMockRecorder) UpdateRisingWaveConfigByNsIDAwait(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateRisingWaveConfigByNsIDAwait", reflect.TypeOf((*MockCloudClientInterface)(nil).UpdateRisingWaveConfigByNsIDAwait), arg0, arg1, arg2)
}
