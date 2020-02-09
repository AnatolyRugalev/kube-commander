package discovery

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

type Client struct {
	*discovery.DiscoveryClient
}

func NewClient(r rest.Interface) *Client {
	return &Client{
		DiscoveryClient: discovery.NewDiscoveryClient(r),
	}
}
