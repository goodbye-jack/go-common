package http

import (
	"context"
	"fmt"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/utils"
)

type HTTPClient struct {
	tenant         string
	service_name   string
	service_domain string
}

var uniq2client map[string]*HTTPClient = map[string]*HTTPClient{}

func genUniq(tenant, service_name string) string {
	if tenant == utils.TenantAnonymous {
		return service_name
	}
	return fmt.Sprintf("%s_%s", tenant, service_name)
}

func NewHTTPClient(ctx context.Context, tenant, service_name string) *HTTPClient {
	uniq := genUniq(tenant, service_name)
	if client, ok := uniq2client[uniq]; ok {
		return client
	}

	uniq2client[uniq] = &HTTPClient{
		tenant:       tenant,
		service_name: service_name,
	}

	log.Infof("the uniq %s create newHTTPClient %+v", uniq, uniq2client[uniq])
	return uniq2client[uniq]
}

func (c *HTTPClient) Get() {
}

func (c *HTTPClient) Post() {
}

func (c *HTTPClient) Put() {
}

func (c *HTTPClient) Delete() {
}
