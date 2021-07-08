package pubsub

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	subscriptionsapi_v1alpha1 "github.com/dapr/dapr/pkg/apis/subscriptions/v1alpha1"
	"github.com/dapr/kit/logger"
)

var log = logger.NewLogger("dapr.test")

func TestFilterSubscriptions(t *testing.T) {
	subs := []Subscription{
		{
			Topic: "topic0",
			Routes: []*Route{
				{
					Path: "topic0",
				},
			},
		},
		{
			Topic: "topic1",
		},
		{
			Topic: "topic1",
			Routes: []*Route{
				{
					Path: "custom/topic1",
				},
			},
		},
	}

	subs = filterSubscriptions(subs, log)
	if assert.Len(t, subs, 2) {
		assert.Equal(t, "topic0", subs[0].Topic)
		assert.Equal(t, "topic1", subs[1].Topic)
		if assert.Len(t, subs[1].Routes, 1) {
			assert.Equal(t, "custom/topic1", subs[1].Routes[0].Path)
		}
	}
}

func testDeclarativeSubscription() subscriptionsapi_v1alpha1.Subscription {
	return subscriptionsapi_v1alpha1.Subscription{
		TypeMeta: v1.TypeMeta{
			Kind:       "Subscription",
			APIVersion: "v1alpha1",
		},
		Spec: subscriptionsapi_v1alpha1.SubscriptionSpec{
			Topic:      "topic1",
			Route:      "myroute",
			Pubsubname: "pubsub",
			Metadata: map[string]string{
				"testName": "testValue",
			},
		},
	}
}

func writeSubscriptionToDisk(subscription subscriptionsapi_v1alpha1.Subscription, filePath string) {
	b, _ := yaml.Marshal(subscription)
	ioutil.WriteFile(filePath, b, 0600)
}

func TestDeclarativeSubscriptions(t *testing.T) {
	dir := filepath.Join(".", "components")
	os.Mkdir(dir, 0777)
	defer os.RemoveAll(dir)

	t.Run("load single valid subscription", func(t *testing.T) {
		s := testDeclarativeSubscription()
		s.Scopes = []string{"scope1"}

		filePath := filepath.Join(".", "components", "sub.yaml")
		writeSubscriptionToDisk(s, filePath)

		subs := DeclarativeSelfHosted(dir, log)
		if assert.Len(t, subs, 1) {
			assert.Equal(t, "topic1", subs[0].Topic)
			if assert.Len(t, subs[0].Routes, 1) {
				assert.Equal(t, "myroute", subs[0].Routes[0].Path)
			}
			assert.Equal(t, "pubsub", subs[0].PubsubName)
			assert.Equal(t, "scope1", subs[0].Scopes[0])
			assert.Equal(t, "testValue", subs[0].Metadata["testName"])
		}
	})

	t.Run("load multiple subscriptions", func(t *testing.T) {
		for i := 0; i < 1; i++ {
			s := testDeclarativeSubscription()
			s.Spec.Topic = fmt.Sprintf("%v", i)
			s.Spec.Route = fmt.Sprintf("%v", i)
			s.Spec.Pubsubname = fmt.Sprintf("%v", i)
			s.Spec.Metadata = map[string]string{
				"testName": fmt.Sprintf("%v", i),
			}
			s.Scopes = []string{fmt.Sprintf("%v", i)}

			writeSubscriptionToDisk(s, fmt.Sprintf("%s/%v", dir, i))
		}

		subs := DeclarativeSelfHosted(dir, log)
		if assert.Len(t, subs, 2) {
			for i := 0; i < 1; i++ {
				assert.Equal(t, fmt.Sprintf("%v", i), subs[i].Topic)
				if assert.Equal(t, 1, len(subs[i].Routes)) {
					assert.Equal(t, fmt.Sprintf("%v", i), subs[i].Routes[0].Path)
				}
				assert.Equal(t, fmt.Sprintf("%v", i), subs[i].PubsubName)
				assert.Equal(t, fmt.Sprintf("%v", i), subs[i].Scopes[0])
				assert.Equal(t, fmt.Sprintf("%v", i), subs[i].Metadata["testName"])
			}
		}
	})

	t.Run("no subscriptions loaded", func(t *testing.T) {
		os.RemoveAll(dir)

		s := testDeclarativeSubscription()
		s.Scopes = []string{"scope1"}

		writeSubscriptionToDisk(s, dir)

		subs := DeclarativeSelfHosted(dir, log)
		assert.Len(t, subs, 0)
	})
}
