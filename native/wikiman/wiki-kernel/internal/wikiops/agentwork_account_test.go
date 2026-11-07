package wikiops

import (
	"context"
	"errors"
	"testing"
)

func TestAgentWorkRejectsAccountAndOfficialCloudEndpoints(t *testing.T) {
	for _, endpoint := range []string{"/api/account/deviceLogin", "/api/account/deviceCloud", "/api/account/deviceRepository", "/api/cloud/getCloudSpace", "/api/sync/performSync"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := UI(context.Background(), nil, UIRequest{Endpoint: endpoint}, UIView{Mode: "wikiDesktop"})
			if !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("account endpoint accepted: %v", err)
			}
		})
	}
}
