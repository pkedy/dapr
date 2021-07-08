package pubsub

import (
	"github.com/dapr/dapr/pkg/expr"
)

type Subscription struct {
	PubsubName string            `json:"pubsubname"`
	Topic      string            `json:"topic"`
	Metadata   map[string]string `json:"metadata"`
	Routes     []*Route          `json:"routes,omitempty"`
	Scopes     []string          `json:"scopes"`
}

type Route struct {
	Match *expr.Expr `json:"match"`
	Path  string     `json:"path"`
}
