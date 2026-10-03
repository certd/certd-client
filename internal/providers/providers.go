package providers

import (
	"github.com/certd/certd-client/internal/app_provider"
	"github.com/certd/certd-client/internal/app_provider/apache"
	"github.com/certd/certd-client/internal/app_provider/iis"
	"github.com/certd/certd-client/internal/app_provider/nginx"
)

// Registered returns the providers supported by the target operating system.
func Registered(goos string) *app_provider.Registry {
	items := []app_provider.Provider{nginx.New(), apache.New()}
	if goos == "windows" {
		items = append(items, iis.New())
	}
	return app_provider.NewRegistry(items...)
}
