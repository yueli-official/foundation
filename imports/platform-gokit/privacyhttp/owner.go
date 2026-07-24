// Package privacyhttp bridges the Foundation net/http Owner transport into
// GoFrame while leaving JWT verification and routing to each consumer.
package privacyhttp

import (
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	foundationauth "github.com/yueli-official/foundation/go/auth"
	"github.com/yueli-official/foundation/go/privacy"
	"github.com/yueli-official/foundation/go/privacy/httpadapter"
)

func OwnerHandler(host privacy.OwnerHost, requiredScope string) ghttp.HandlerFunc {
	handler, err := httpadapter.NewHandler(httpadapter.HandlerOptions{Host: host})
	if err != nil {
		panic(err)
	}
	scope := strings.TrimSpace(requiredScope)
	if scope == "" {
		scope = "privacy:owner"
	}
	return func(request *ghttp.Request) {
		principal, ok := foundationauth.FromContext(request.Context())
		if !ok || principal == nil || !principal.HasScope(scope) {
			request.Response.Header().Set("Content-Type", "application/json")
			request.Response.WriteStatus(http.StatusForbidden, `{"error":"privacy owner scope required"}`)
			request.Exit()
			return
		}
		handler.ServeHTTP(request.Response.BufferWriter, request.Request)
		request.Exit()
	}
}
