package httpapi

import (
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	c "github.com/erranliu/SteerLine/internal/product/httpclient"
)

var Open = c.Open

type ClientError = c.ClientError

func productPorts(client *productclient.Client) Ports {
	return Ports{Query: client, Commands: client, Authority: client, NativeViewer: client}
}
