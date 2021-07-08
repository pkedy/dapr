package pubsub

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	subscriptionsapi_v1alpha1 "github.com/dapr/dapr/pkg/apis/subscriptions/v1alpha1"
	subscriptionsapi_v2alpha1 "github.com/dapr/dapr/pkg/apis/subscriptions/v2alpha1"
	"github.com/dapr/dapr/pkg/channel"
	"github.com/dapr/dapr/pkg/expr"
	invokev1 "github.com/dapr/dapr/pkg/messaging/v1"
	operatorv1pb "github.com/dapr/dapr/pkg/proto/operator/v1"
	runtimev1pb "github.com/dapr/dapr/pkg/proto/runtime/v1"
	"github.com/dapr/kit/logger"
)

const (
	getTopicsError         = "error getting topic list from app: %s"
	deserializeTopicsError = "error getting topics from app: %s"
	noSubscriptionsError   = "user app did not subscribe to any topic"
	subscriptionKind       = "Subscription"
)

func GetSubscriptionsHTTP(channel channel.AppChannel, log logger.Logger) ([]Subscription, error) {
	// subscriptionItem is compatible with both v1alpha1 and v2alpha1 structures
	type subscriptionItem struct {
		Subscription
		Route string `json:"route"` // Single route from v1alpha1
	}

	var subscriptions []Subscription
	var subscriptionItems []subscriptionItem

	req := invokev1.NewInvokeMethodRequest("dapr/subscribe")
	req.WithHTTPExtension(http.MethodGet, "")
	req.WithRawData(nil, invokev1.JSONContentType)

	// TODO Propagate Context
	ctx := context.Background()
	resp, err := channel.InvokeMethod(ctx, req)
	if err != nil {
		log.Errorf(getTopicsError, err)
	}

	switch resp.Status().Code {
	case http.StatusOK:
		_, body := resp.RawData()
		if err := json.Unmarshal(body, &subscriptionItems); err != nil {
			log.Errorf(deserializeTopicsError, err)

			return nil, errors.Errorf(deserializeTopicsError, err)
		}
		subscriptions = make([]Subscription, len(subscriptionItems))
		for i, si := range subscriptionItems {
			// Look for single route field and append it as a route struct.
			// This preserves backward compatibility.
			if si.Route != "" {
				si.Routes = append(si.Routes, &Route{
					Path: si.Route,
				})
			}
			subscriptions[i] = si.Subscription
		}
	case http.StatusNotFound:
		log.Debug(noSubscriptionsError)

	default:
		// Unexpected response: both GRPC and HTTP have to log the same level.
		log.Errorf("app returned http status code %v from subscription endpoint", resp.Status().Code)
	}

	log.Debugf("app responded with subscriptions %v", subscriptions)

	return filterSubscriptions(subscriptions, log), nil
}

func filterSubscriptions(subscriptions []Subscription, log logger.Logger) []Subscription {
	for i := len(subscriptions) - 1; i >= 0; i-- {
		if len(subscriptions[i].Routes) == 0 {
			log.Warnf("topic %s has an empty routes. removing from subscriptions list", subscriptions[i].Topic)
			subscriptions = append(subscriptions[:i], subscriptions[i+1:]...)
		}
	}

	return subscriptions
}

func GetSubscriptionsGRPC(channel runtimev1pb.AppCallbackClient, log logger.Logger) ([]Subscription, error) {
	var subscriptions []Subscription

	resp, err := channel.ListTopicSubscriptions(context.Background(), &emptypb.Empty{})
	if err != nil {
		// Unexpected response: both GRPC and HTTP have to log the same level.
		log.Errorf(getTopicsError, err)
	} else {
		if resp == nil || resp.Subscriptions == nil || len(resp.Subscriptions) == 0 {
			log.Debug(noSubscriptionsError)
		} else {
			for _, s := range resp.Subscriptions {
				routes, err := parseRoutingRulesGRPC(s.Routes)
				if err != nil {
					return nil, err
				}
				subscriptions = append(subscriptions, Subscription{
					PubsubName: s.PubsubName,
					Topic:      s.GetTopic(),
					Metadata:   s.GetMetadata(),
					Routes:     routes,
				})
			}
		}
	}

	return subscriptions, nil
}

// DeclarativeSelfHosted loads subscriptions from the given components path.
func DeclarativeSelfHosted(componentsPath string, log logger.Logger) []Subscription {
	var subs []Subscription

	if _, err := os.Stat(componentsPath); os.IsNotExist(err) {
		return subs
	}

	files, err := ioutil.ReadDir(componentsPath)
	if err != nil {
		log.Errorf("failed to read subscriptions from path %s: %s", err)
		return subs
	}

	for _, f := range files {
		if !f.IsDir() {
			filePath := filepath.Join(componentsPath, f.Name())
			b, err := ioutil.ReadFile(filePath)
			if err != nil {
				log.Errorf("failed to read file %s: %s", filePath, err)
				continue
			}

			subs, err = appendSubscription(subs, b)
			if err != nil {
				log.Warnf("failed to add subscription from file %s: %s", filePath, err)
				continue
			}
		}
	}

	return subs
}

func marshalSubscription(b []byte) (*Subscription, error) {
	// Parse only the type metadata first in order
	// to filter out non-Subscriptions without other errors.
	type typeInfo struct {
		metav1.TypeMeta `json:",inline"`
	}

	var ti typeInfo
	if err := yaml.Unmarshal(b, &ti); err != nil {
		return nil, err
	}

	if ti.Kind != subscriptionKind {
		return nil, nil
	}

	switch ti.APIVersion {
	case "v2alpha1":
		var sub subscriptionsapi_v2alpha1.Subscription
		if err := yaml.Unmarshal(b, &sub); err != nil {
			return nil, err
		}

		routes, err := parseRoutingRulesYAML(sub.Spec.Routes)
		if err != nil {
			return nil, err
		}

		return &Subscription{
			Topic:      sub.Spec.Topic,
			PubsubName: sub.Spec.Pubsubname,
			Routes:     routes,
			Metadata:   sub.Spec.Metadata,
			Scopes:     sub.Scopes,
		}, nil

	default:
		// assume "v1alpha1" for backward compatibility as this was
		// not checked before the introduction of "v2alpha".
		var sub subscriptionsapi_v1alpha1.Subscription
		if err := yaml.Unmarshal(b, &sub); err != nil {
			return nil, err
		}

		return &Subscription{
			Topic:      sub.Spec.Topic,
			PubsubName: sub.Spec.Pubsubname,
			Routes: []*Route{
				{
					Path: sub.Spec.Route,
				},
			},
			Metadata: sub.Spec.Metadata,
			Scopes:   sub.Scopes,
		}, nil
	}
}

func parseRoutingRulesYAML(routes []subscriptionsapi_v2alpha1.Route) ([]*Route, error) {
	r := make([]*Route, len(routes))

	for i, route := range routes {
		rr, err := createRoutingRule(route.Match, route.Path)
		if err != nil {
			return nil, err
		}
		r[i] = rr
	}

	return r, nil
}

func parseRoutingRulesGRPC(routes []*runtimev1pb.TopicRoute) ([]*Route, error) {
	r := make([]*Route, 0, len(routes)+1)

	for _, route := range routes {
		rr, err := createRoutingRule(route.Match, route.Path)
		if err != nil {
			return nil, err
		}
		r = append(r, rr)
	}

	// gRPC has automatically specifies a default route
	// if none are returned.
	if len(r) == 0 {
		r = append(r, &Route{
			Path: "",
		})
	}

	return r, nil
}

func createRoutingRule(match, path string) (*Route, error) {
	var e *expr.Expr
	matchTrimmed := strings.TrimSpace(match)
	if matchTrimmed != "" {
		e = &expr.Expr{}
		if err := e.DecodeString(matchTrimmed); err != nil {
			return nil, err
		}
	}

	return &Route{
		Match: e,
		Path:  path,
	}, nil
}

// DeclarativeKubernetes loads subscriptions from the operator when running in Kubernetes.
func DeclarativeKubernetes(client operatorv1pb.OperatorClient, log logger.Logger) []Subscription {
	var subs []Subscription
	resp, err := client.ListSubscriptions(context.TODO(), &emptypb.Empty{})
	if err != nil {
		log.Errorf("failed to list subscriptions from operator: %s", err)

		return subs
	}

	for _, s := range resp.Subscriptions {
		subs, err = appendSubscription(subs, s)
		if err != nil {
			log.Warnf("failed to add subscription from operator: %s", err)
			continue
		}
	}

	return subs
}

func appendSubscription(list []Subscription, subBytes []byte) ([]Subscription, error) {
	sub, err := marshalSubscription(subBytes)
	if err != nil {
		return nil, err
	}

	if sub != nil {
		list = append(list, *sub)
	}

	return list, nil
}
