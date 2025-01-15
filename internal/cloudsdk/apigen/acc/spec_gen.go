// Package apigen provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen/v2 version v2.0.0 DO NOT EDIT.
package apigen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oapi-codegen/runtime"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

const (
	ApiKeyAuthScopes = "ApiKeyAuth.Scopes"
	BearerAuthScopes = "BearerAuth.Scopes"
)

// Defines values for ControlClaimsClaimsType.
const (
	ServiceAccount ControlClaimsClaimsType = "ServiceAccount"
	User           ControlClaimsClaimsType = "User"
)

// Defines values for TierId.
const (
	BYOC           TierId = "BYOC"
	Benchmark      TierId = "Benchmark"
	DeveloperBasic TierId = "Developer-Basic"
	DeveloperFree  TierId = "Developer-Free"
	DeveloperTest  TierId = "Developer-Test"
	Free           TierId = "Free"
	Invited        TierId = "Invited"
	Standard       TierId = "Standard"
	Test           TierId = "Test"
)

// ControlClaims defines model for ControlClaims.
type ControlClaims struct {
	Claims     ControlClaims_Claims    `json:"claims"`
	ClaimsType ControlClaimsClaimsType `json:"claimsType"`
}

// ControlClaims_Claims defines model for ControlClaims.Claims.
type ControlClaims_Claims struct {
	union json.RawMessage
}

// ControlClaimsClaimsType defines model for ControlClaims.ClaimsType.
type ControlClaimsClaimsType string

// Page defines model for Page.
type Page struct {
	Limit  uint64 `json:"limit"`
	Offset uint64 `json:"offset"`
}

// PrivateLink defines model for PrivateLink.
type PrivateLink struct {
	ConnectionName string             `json:"connectionName"`
	Id             openapi_types.UUID `json:"id"`
	OrgId          openapi_types.UUID `json:"orgId"`
	Region         string             `json:"region"`
	Target         *string            `json:"target,omitempty"`
	TenantId       uint64             `json:"tenantId"`
}

// PrivateLinkArray defines model for PrivateLinkArray.
type PrivateLinkArray = []PrivateLink

// PrivateLinkSizePage defines model for PrivateLinkSizePage.
type PrivateLinkSizePage struct {
	Limit        uint64           `json:"limit"`
	Offset       uint64           `json:"offset"`
	PrivateLinks PrivateLinkArray `json:"privateLinks"`
	Size         uint64           `json:"size"`
}

// Region defines model for Region.
type Region struct {
	AdminUrl      string `json:"adminUrl"`
	Id            uint64 `json:"id"`
	IsBYOCOnly    bool   `json:"isBYOCOnly"`
	IsRegionReady bool   `json:"isRegionReady"`
	PgwebUrl      string `json:"pgwebUrl"`
	Platform      string `json:"platform"`
	RegionName    string `json:"regionName"`
	Url           string `json:"url"`
	UrlV2         string `json:"urlV2"`
}

// RegionArray defines model for RegionArray.
type RegionArray = []Region

// ServiceAccountClaims defines model for ServiceAccountClaims.
type ServiceAccountClaims struct {
	OrgId            openapi_types.UUID `json:"orgId"`
	ServiceAccount   string             `json:"serviceAccount"`
	ServiceAccountId openapi_types.UUID `json:"serviceAccountId"`
}

// Size defines model for Size.
type Size struct {
	Size uint64 `json:"size"`
}

// Tenant defines model for Tenant.
type Tenant struct {
	CreatedAt     time.Time          `json:"createdAt"`
	DeactivatedAt *time.Time         `json:"deactivatedAt,omitempty"`
	Id            uint64             `json:"id"`
	NsId          openapi_types.UUID `json:"nsId"`
	OrgId         openapi_types.UUID `json:"orgId"`
	Region        string             `json:"region"`
	TenantName    string             `json:"tenantName"`
	TierId        TierId             `json:"tierId"`
	TrialBefore   time.Time          `json:"trialBefore"`
	UpdatedAt     time.Time          `json:"updatedAt"`
	UserId        uint64             `json:"userId"`
}

// TenantProperties defines model for TenantProperties.
type TenantProperties struct {
	Id         uint64             `json:"id"`
	NsId       openapi_types.UUID `json:"nsId"`
	OrgId      openapi_types.UUID `json:"orgId"`
	Region     string             `json:"region"`
	TenantName string             `json:"tenantName"`
	TierId     TierId             `json:"tierId"`
	UserId     uint64             `json:"userId"`
}

// TierId defines model for TierId.
type TierId string

// UserControlClaims defines model for UserControlClaims.
type UserControlClaims struct {
	Email          string             `json:"email"`
	OrgId          openapi_types.UUID `json:"orgId"`
	UserResourceId openapi_types.UUID `json:"userResourceId"`
	Username       string             `json:"username"`
}

// DefaultResponse defines model for DefaultResponse.
type DefaultResponse struct {
	Msg string `json:"msg"`
}

// NotFoundResponse defines model for NotFoundResponse.
type NotFoundResponse struct {
	Msg string `json:"msg"`
}

// GetPrivatelinksParams defines parameters for GetPrivatelinks.
type GetPrivatelinksParams struct {
	Offset *uint64 `form:"offset,omitempty" json:"offset,omitempty"`
	Limit  *uint64 `form:"limit,omitempty" json:"limit,omitempty"`
}

// AsUserControlClaims returns the union data inside the ControlClaims_Claims as a UserControlClaims
func (t ControlClaims_Claims) AsUserControlClaims() (UserControlClaims, error) {
	var body UserControlClaims
	err := json.Unmarshal(t.union, &body)
	return body, err
}

// FromUserControlClaims overwrites any union data inside the ControlClaims_Claims as the provided UserControlClaims
func (t *ControlClaims_Claims) FromUserControlClaims(v UserControlClaims) error {
	b, err := json.Marshal(v)
	t.union = b
	return err
}

// MergeUserControlClaims performs a merge with any union data inside the ControlClaims_Claims, using the provided UserControlClaims
func (t *ControlClaims_Claims) MergeUserControlClaims(v UserControlClaims) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	merged, err := runtime.JsonMerge(t.union, b)
	t.union = merged
	return err
}

// AsServiceAccountClaims returns the union data inside the ControlClaims_Claims as a ServiceAccountClaims
func (t ControlClaims_Claims) AsServiceAccountClaims() (ServiceAccountClaims, error) {
	var body ServiceAccountClaims
	err := json.Unmarshal(t.union, &body)
	return body, err
}

// FromServiceAccountClaims overwrites any union data inside the ControlClaims_Claims as the provided ServiceAccountClaims
func (t *ControlClaims_Claims) FromServiceAccountClaims(v ServiceAccountClaims) error {
	b, err := json.Marshal(v)
	t.union = b
	return err
}

// MergeServiceAccountClaims performs a merge with any union data inside the ControlClaims_Claims, using the provided ServiceAccountClaims
func (t *ControlClaims_Claims) MergeServiceAccountClaims(v ServiceAccountClaims) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	merged, err := runtime.JsonMerge(t.union, b)
	t.union = merged
	return err
}

func (t ControlClaims_Claims) MarshalJSON() ([]byte, error) {
	b, err := t.union.MarshalJSON()
	return b, err
}

func (t *ControlClaims_Claims) UnmarshalJSON(b []byte) error {
	err := t.union.UnmarshalJSON(b)
	return err
}

// RequestEditorFn  is the function signature for the RequestEditor callback function
type RequestEditorFn func(ctx context.Context, req *http.Request) error

// Doer performs HTTP requests.
//
// The standard http.Client implements this interface.
type HttpRequestDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client which conforms to the OpenAPI3 specification for this service.
type Client struct {
	// The endpoint of the server conforming to this interface, with scheme,
	// https://api.deepmap.com for example. This can contain a path relative
	// to the server, such as https://api.deepmap.com/dev-test, and all the
	// paths in the swagger spec will be appended to the server.
	Server string

	// Doer for performing requests, typically a *http.Client with any
	// customized settings, such as certificate chains.
	Client HttpRequestDoer

	// A list of callbacks for modifying requests which are generated before sending over
	// the network.
	RequestEditors []RequestEditorFn
}

// ClientOption allows setting custom parameters during construction
type ClientOption func(*Client) error

// Creates a new Client, with reasonable defaults
func NewClient(server string, opts ...ClientOption) (*Client, error) {
	// create a client with sane default values
	client := Client{
		Server: server,
	}
	// mutate client and add all optional params
	for _, o := range opts {
		if err := o(&client); err != nil {
			return nil, err
		}
	}
	// ensure the server URL always has a trailing slash
	if !strings.HasSuffix(client.Server, "/") {
		client.Server += "/"
	}
	// create httpClient, if not already present
	if client.Client == nil {
		client.Client = &http.Client{}
	}
	return &client, nil
}

// WithHTTPClient allows overriding the default Doer, which is
// automatically created using http.Client. This is useful for tests.
func WithHTTPClient(doer HttpRequestDoer) ClientOption {
	return func(c *Client) error {
		c.Client = doer
		return nil
	}
}

// WithRequestEditorFn allows setting up a callback function, which will be
// called right before sending the request. This can be used to mutate the request.
func WithRequestEditorFn(fn RequestEditorFn) ClientOption {
	return func(c *Client) error {
		c.RequestEditors = append(c.RequestEditors, fn)
		return nil
	}
}

// The interface specification for the client above.
type ClientInterface interface {
	// GetAuthPing request
	GetAuthPing(ctx context.Context, reqEditors ...RequestEditorFn) (*http.Response, error)

	// GetPrivatelinks request
	GetPrivatelinks(ctx context.Context, params *GetPrivatelinksParams, reqEditors ...RequestEditorFn) (*http.Response, error)

	// GetRegions request
	GetRegions(ctx context.Context, reqEditors ...RequestEditorFn) (*http.Response, error)

	// GetTenantNsID request
	GetTenantNsID(ctx context.Context, nsID openapi_types.UUID, reqEditors ...RequestEditorFn) (*http.Response, error)
}

func (c *Client) GetAuthPing(ctx context.Context, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewGetAuthPingRequest(c.Server)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *Client) GetPrivatelinks(ctx context.Context, params *GetPrivatelinksParams, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewGetPrivatelinksRequest(c.Server, params)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *Client) GetRegions(ctx context.Context, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewGetRegionsRequest(c.Server)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *Client) GetTenantNsID(ctx context.Context, nsID openapi_types.UUID, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewGetTenantNsIDRequest(c.Server, nsID)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

// NewGetAuthPingRequest generates requests for GetAuthPing
func NewGetAuthPingRequest(server string) (*http.Request, error) {
	var err error

	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	operationPath := fmt.Sprintf("/auth/ping")
	if operationPath[0] == '/' {
		operationPath = "." + operationPath
	}

	queryURL, err := serverURL.Parse(operationPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", queryURL.String(), nil)
	if err != nil {
		return nil, err
	}

	return req, nil
}

// NewGetPrivatelinksRequest generates requests for GetPrivatelinks
func NewGetPrivatelinksRequest(server string, params *GetPrivatelinksParams) (*http.Request, error) {
	var err error

	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	operationPath := fmt.Sprintf("/privatelinks")
	if operationPath[0] == '/' {
		operationPath = "." + operationPath
	}

	queryURL, err := serverURL.Parse(operationPath)
	if err != nil {
		return nil, err
	}

	if params != nil {
		queryValues := queryURL.Query()

		if params.Offset != nil {

			if queryFrag, err := runtime.StyleParamWithLocation("form", true, "offset", runtime.ParamLocationQuery, *params.Offset); err != nil {
				return nil, err
			} else if parsed, err := url.ParseQuery(queryFrag); err != nil {
				return nil, err
			} else {
				for k, v := range parsed {
					for _, v2 := range v {
						queryValues.Add(k, v2)
					}
				}
			}

		}

		if params.Limit != nil {

			if queryFrag, err := runtime.StyleParamWithLocation("form", true, "limit", runtime.ParamLocationQuery, *params.Limit); err != nil {
				return nil, err
			} else if parsed, err := url.ParseQuery(queryFrag); err != nil {
				return nil, err
			} else {
				for k, v := range parsed {
					for _, v2 := range v {
						queryValues.Add(k, v2)
					}
				}
			}

		}

		queryURL.RawQuery = queryValues.Encode()
	}

	req, err := http.NewRequest("GET", queryURL.String(), nil)
	if err != nil {
		return nil, err
	}

	return req, nil
}

// NewGetRegionsRequest generates requests for GetRegions
func NewGetRegionsRequest(server string) (*http.Request, error) {
	var err error

	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	operationPath := fmt.Sprintf("/regions")
	if operationPath[0] == '/' {
		operationPath = "." + operationPath
	}

	queryURL, err := serverURL.Parse(operationPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", queryURL.String(), nil)
	if err != nil {
		return nil, err
	}

	return req, nil
}

// NewGetTenantNsIDRequest generates requests for GetTenantNsID
func NewGetTenantNsIDRequest(server string, nsID openapi_types.UUID) (*http.Request, error) {
	var err error

	var pathParam0 string

	pathParam0, err = runtime.StyleParamWithLocation("simple", false, "nsID", runtime.ParamLocationPath, nsID)
	if err != nil {
		return nil, err
	}

	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	operationPath := fmt.Sprintf("/tenant/%s", pathParam0)
	if operationPath[0] == '/' {
		operationPath = "." + operationPath
	}

	queryURL, err := serverURL.Parse(operationPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", queryURL.String(), nil)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (c *Client) applyEditors(ctx context.Context, req *http.Request, additionalEditors []RequestEditorFn) error {
	for _, r := range c.RequestEditors {
		if err := r(ctx, req); err != nil {
			return err
		}
	}
	for _, r := range additionalEditors {
		if err := r(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// ClientWithResponses builds on ClientInterface to offer response payloads
type ClientWithResponses struct {
	ClientInterface
}

// NewClientWithResponses creates a new ClientWithResponses, which wraps
// Client with return type handling
func NewClientWithResponses(server string, opts ...ClientOption) (*ClientWithResponses, error) {
	client, err := NewClient(server, opts...)
	if err != nil {
		return nil, err
	}
	return &ClientWithResponses{client}, nil
}

// WithBaseURL overrides the baseURL.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) error {
		newBaseURL, err := url.Parse(baseURL)
		if err != nil {
			return err
		}
		c.Server = newBaseURL.String()
		return nil
	}
}

// ClientWithResponsesInterface is the interface specification for the client with responses above.
type ClientWithResponsesInterface interface {
	// GetAuthPingWithResponse request
	GetAuthPingWithResponse(ctx context.Context, reqEditors ...RequestEditorFn) (*GetAuthPingResponse, error)

	// GetPrivatelinksWithResponse request
	GetPrivatelinksWithResponse(ctx context.Context, params *GetPrivatelinksParams, reqEditors ...RequestEditorFn) (*GetPrivatelinksResponse, error)

	// GetRegionsWithResponse request
	GetRegionsWithResponse(ctx context.Context, reqEditors ...RequestEditorFn) (*GetRegionsResponse, error)

	// GetTenantNsIDWithResponse request
	GetTenantNsIDWithResponse(ctx context.Context, nsID openapi_types.UUID, reqEditors ...RequestEditorFn) (*GetTenantNsIDResponse, error)
}

type GetAuthPingResponse struct {
	Body         []byte
	HTTPResponse *http.Response
	JSON200      *ControlClaims
	JSON401      *DefaultResponse
}

// Status returns HTTPResponse.Status
func (r GetAuthPingResponse) Status() string {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.Status
	}
	return http.StatusText(0)
}

// StatusCode returns HTTPResponse.StatusCode
func (r GetAuthPingResponse) StatusCode() int {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.StatusCode
	}
	return 0
}

type GetPrivatelinksResponse struct {
	Body         []byte
	HTTPResponse *http.Response
	JSON200      *PrivateLinkSizePage
}

// Status returns HTTPResponse.Status
func (r GetPrivatelinksResponse) Status() string {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.Status
	}
	return http.StatusText(0)
}

// StatusCode returns HTTPResponse.StatusCode
func (r GetPrivatelinksResponse) StatusCode() int {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.StatusCode
	}
	return 0
}

type GetRegionsResponse struct {
	Body         []byte
	HTTPResponse *http.Response
	JSON200      *RegionArray
}

// Status returns HTTPResponse.Status
func (r GetRegionsResponse) Status() string {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.Status
	}
	return http.StatusText(0)
}

// StatusCode returns HTTPResponse.StatusCode
func (r GetRegionsResponse) StatusCode() int {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.StatusCode
	}
	return 0
}

type GetTenantNsIDResponse struct {
	Body         []byte
	HTTPResponse *http.Response
	JSON200      *Tenant
	JSON404      *NotFoundResponse
}

// Status returns HTTPResponse.Status
func (r GetTenantNsIDResponse) Status() string {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.Status
	}
	return http.StatusText(0)
}

// StatusCode returns HTTPResponse.StatusCode
func (r GetTenantNsIDResponse) StatusCode() int {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.StatusCode
	}
	return 0
}

// GetAuthPingWithResponse request returning *GetAuthPingResponse
func (c *ClientWithResponses) GetAuthPingWithResponse(ctx context.Context, reqEditors ...RequestEditorFn) (*GetAuthPingResponse, error) {
	rsp, err := c.GetAuthPing(ctx, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParseGetAuthPingResponse(rsp)
}

// GetPrivatelinksWithResponse request returning *GetPrivatelinksResponse
func (c *ClientWithResponses) GetPrivatelinksWithResponse(ctx context.Context, params *GetPrivatelinksParams, reqEditors ...RequestEditorFn) (*GetPrivatelinksResponse, error) {
	rsp, err := c.GetPrivatelinks(ctx, params, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParseGetPrivatelinksResponse(rsp)
}

// GetRegionsWithResponse request returning *GetRegionsResponse
func (c *ClientWithResponses) GetRegionsWithResponse(ctx context.Context, reqEditors ...RequestEditorFn) (*GetRegionsResponse, error) {
	rsp, err := c.GetRegions(ctx, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParseGetRegionsResponse(rsp)
}

// GetTenantNsIDWithResponse request returning *GetTenantNsIDResponse
func (c *ClientWithResponses) GetTenantNsIDWithResponse(ctx context.Context, nsID openapi_types.UUID, reqEditors ...RequestEditorFn) (*GetTenantNsIDResponse, error) {
	rsp, err := c.GetTenantNsID(ctx, nsID, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParseGetTenantNsIDResponse(rsp)
}

// ParseGetAuthPingResponse parses an HTTP response from a GetAuthPingWithResponse call
func ParseGetAuthPingResponse(rsp *http.Response) (*GetAuthPingResponse, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return nil, err
	}

	response := &GetAuthPingResponse{
		Body:         bodyBytes,
		HTTPResponse: rsp,
	}

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest ControlClaims
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON200 = &dest

	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 401:
		var dest DefaultResponse
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON401 = &dest

	}

	return response, nil
}

// ParseGetPrivatelinksResponse parses an HTTP response from a GetPrivatelinksWithResponse call
func ParseGetPrivatelinksResponse(rsp *http.Response) (*GetPrivatelinksResponse, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return nil, err
	}

	response := &GetPrivatelinksResponse{
		Body:         bodyBytes,
		HTTPResponse: rsp,
	}

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest PrivateLinkSizePage
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON200 = &dest

	}

	return response, nil
}

// ParseGetRegionsResponse parses an HTTP response from a GetRegionsWithResponse call
func ParseGetRegionsResponse(rsp *http.Response) (*GetRegionsResponse, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return nil, err
	}

	response := &GetRegionsResponse{
		Body:         bodyBytes,
		HTTPResponse: rsp,
	}

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest RegionArray
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON200 = &dest

	}

	return response, nil
}

// ParseGetTenantNsIDResponse parses an HTTP response from a GetTenantNsIDWithResponse call
func ParseGetTenantNsIDResponse(rsp *http.Response) (*GetTenantNsIDResponse, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return nil, err
	}

	response := &GetTenantNsIDResponse{
		Body:         bodyBytes,
		HTTPResponse: rsp,
	}

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest Tenant
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON200 = &dest

	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 404:
		var dest NotFoundResponse
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON404 = &dest

	}

	return response, nil
}
