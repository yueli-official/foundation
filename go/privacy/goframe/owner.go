// Package privacygoframe adapts the framework-independent Privacy Owner
// transport to GoFrame. Authentication and routing remain caller-owned.
package privacygoframe

import (
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/yueli-official/foundation/go/privacy"
	"github.com/yueli-official/foundation/go/privacy/httpadapter"
)

func NewOwnerHandler(host privacy.OwnerHost) (ghttp.HandlerFunc, error) {
	handler, err := httpadapter.NewHandler(httpadapter.HandlerOptions{Host: host})
	if err != nil {
		return nil, err
	}
	return func(request *ghttp.Request) {
		handler.ServeHTTP(request.Response.BufferWriter, request.Request)
		request.Exit()
	}, nil
}
