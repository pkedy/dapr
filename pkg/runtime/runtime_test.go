// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package runtime

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dapr/components-contrib/pubsub"
	"github.com/dapr/components-contrib/secretstores"
	components_v1alpha1 "github.com/dapr/dapr/pkg/apis/components/v1alpha1"
	values_v1alpha1 "github.com/dapr/dapr/pkg/apis/values/v1alpha1"
	"github.com/dapr/dapr/pkg/channel"
	http_channel "github.com/dapr/dapr/pkg/channel/http"
	channelt "github.com/dapr/dapr/pkg/channel/testing"
	pubsub_loader "github.com/dapr/dapr/pkg/components/pubsub"
	secretstores_loader "github.com/dapr/dapr/pkg/components/secretstores"
	"github.com/dapr/dapr/pkg/config"
	"github.com/dapr/dapr/pkg/modes"
	daprt "github.com/dapr/dapr/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TestRuntimeConfigID = "consumer0"
)

type MockKubernetesStateStore struct {
}

func (m *MockKubernetesStateStore) Init(metadata secretstores.Metadata) error {
	return nil
}

func (m *MockKubernetesStateStore) GetSecret(req secretstores.GetSecretRequest) (secretstores.GetSecretResponse, error) {
	return secretstores.GetSecretResponse{
		Data: map[string]string{
			"key1":            "value1",
			"_value":          "_value_data",
			"int_value":       "12345",
			"float_value":     "1.5",
			"bool_value":      "true",
			"duration_value":  "5s",
			"timestamp_value": "2002-10-02T10:00:00-05:00",
		},
	}, nil
}

func NewMockKubernetesStore() secretstores.SecretStore {
	return &MockKubernetesStateStore{}
}

func TestNewRuntime(t *testing.T) {
	// act
	r := NewDaprRuntime(&Config{}, &config.Configuration{})

	// assert
	assert.NotNil(t, r, "runtime must be initiated")
}

func TestInitPubSub(t *testing.T) {
	rt := NewTestDaprRuntime(modes.StandaloneMode)

	initMockPubSubForRuntime := func(rt *DaprRuntime) *daprt.MockPubSub {
		mockPubSub := new(daprt.MockPubSub)
		rt.pubSubRegistry.Register(
			pubsub_loader.New("mockPubSub", func() pubsub.PubSub {
				return mockPubSub
			}),
		)

		expectedMetadata := pubsub.Metadata{
			Properties: getFakeProperties(),
		}

		mockPubSub.On("Init", expectedMetadata).Return(nil)
		mockPubSub.On(
			"Subscribe",
			mock.AnythingOfType("pubsub.SubscribeRequest"),
			mock.AnythingOfType("func(*pubsub.NewMessage) error")).Return(nil)

		mockAppChannel := new(channelt.MockAppChannel)
		rt.appChannel = mockAppChannel

		return mockPubSub
	}

	t.Run("subscribe 2 topics", func(t *testing.T) {
		mockPubSub := initMockPubSubForRuntime(rt)

		mockAppChannel := new(channelt.MockAppChannel)
		rt.appChannel = mockAppChannel

		// User App subscribes 2 topics via http app channel
		fakeHTTPResponse := &channel.InvokeResponse{
			Metadata: map[string]string{http_channel.HTTPStatusCode: "200"},
			Data:     []byte("[ \"topic0\", \"topic1\" ]"),
		}

		mockAppChannel.On(
			"InvokeMethod",
			&channel.InvokeRequest{
				Method:   "dapr/subscribe",
				Metadata: map[string]string{http_channel.HTTPVerb: http_channel.Get},
			}).Return(fakeHTTPResponse, nil)

		// act
		err := rt.initPubSub()

		// assert
		assert.Nil(t, err)
		mockPubSub.AssertNumberOfCalls(t, "Init", 1)
		mockPubSub.AssertNumberOfCalls(t, "Subscribe", 2)
		mockAppChannel.AssertNumberOfCalls(t, "InvokeMethod", 1)
	})

	t.Run("subscribe 0 topics unless user app provides topic list", func(t *testing.T) {
		mockPubSub := initMockPubSubForRuntime(rt)

		mockAppChannel := new(channelt.MockAppChannel)
		rt.appChannel = mockAppChannel

		fakeHTTPResponse := &channel.InvokeResponse{
			Metadata: map[string]string{http_channel.HTTPStatusCode: "404"},
			Data:     nil,
		}

		mockAppChannel.On(
			"InvokeMethod",
			&channel.InvokeRequest{
				Method:   "dapr/subscribe",
				Metadata: map[string]string{http_channel.HTTPVerb: http_channel.Get},
			}).Return(fakeHTTPResponse, nil)

		// act
		err := rt.initPubSub()

		// assert
		assert.Nil(t, err)
		mockPubSub.AssertNumberOfCalls(t, "Init", 1)
		mockPubSub.AssertNumberOfCalls(t, "Subscribe", 0)
		mockAppChannel.AssertNumberOfCalls(t, "InvokeMethod", 1)
	})
}

func TestInitSecretStores(t *testing.T) {
	t.Run("init with no store", func(t *testing.T) {
		rt := NewTestDaprRuntime(modes.StandaloneMode)
		err := rt.initSecretStores()
		assert.Nil(t, err)
	})

	t.Run("init with store", func(t *testing.T) {
		rt := NewTestDaprRuntime(modes.StandaloneMode)
		m := NewMockKubernetesStore()
		rt.secretStoresRegistry.Register(
			secretstores_loader.New("kubernetesMock", func() secretstores.SecretStore {
				return m
			}))

		rt.components = append(rt.components, components_v1alpha1.Component{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "kubernetesMock",
			},
			Spec: components_v1alpha1.ComponentSpec{
				Type: "secretstores.kubernetesMock",
			},
		})

		err := rt.initSecretStores()
		assert.Nil(t, err)
	})

	t.Run("secret store is registered", func(t *testing.T) {
		rt := NewTestDaprRuntime(modes.StandaloneMode)
		m := NewMockKubernetesStore()
		rt.secretStoresRegistry.Register(
			secretstores_loader.New("kubernetesMock", func() secretstores.SecretStore {
				return m
			}),
		)

		rt.components = append(rt.components, components_v1alpha1.Component{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "kubernetesMock",
			},
			Spec: components_v1alpha1.ComponentSpec{
				Type: "secretstores.kubernetesMock",
			},
		})

		rt.initSecretStores()
		assert.NotNil(t, rt.secretStores["kubernetesMock"])
	})

	t.Run("get secret store", func(t *testing.T) {
		rt := NewTestDaprRuntime(modes.StandaloneMode)
		m := NewMockKubernetesStore()
		rt.secretStoresRegistry.Register(
			secretstores_loader.New("kubernetesMock", func() secretstores.SecretStore {
				return m
			}),
		)

		rt.components = append(rt.components, components_v1alpha1.Component{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "kubernetesMock",
			},
			Spec: components_v1alpha1.ComponentSpec{
				Type: "secretstores.kubernetesMock",
			},
		})

		rt.initSecretStores()
		s := rt.getSecretStore("kubernetesMock")
		assert.NotNil(t, s)
	})
}

func TestMetadataItemsToPropertiesConversion(t *testing.T) {
	rt := NewTestDaprRuntime(modes.StandaloneMode)
	items := values_v1alpha1.Values{
		"a": "b",
	}
	m := rt.convertValuesToProperties(items)
	assert.Equal(t, 1, len(m))
	assert.Equal(t, "b", m["a"])
}

func TestProcessComponentSecrets(t *testing.T) {
	mockBinding := components_v1alpha1.Component{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "mockBinding",
		},
		Spec: components_v1alpha1.ComponentSpec{
			Type: "bindings.mock",
			Config: values_v1alpha1.Values{
				"a": map[string]interface{}{
					"secret_key":  "key1",
					"secret_name": "name1",
				},
				"b": "value2",
			},
		},
		Auth: components_v1alpha1.Auth{
			SecretStore: "kubernetes",
		},
	}

	t.Run("Standalone Mode", func(t *testing.T) {
		mockBinding.Spec.Config["a"] = map[string]interface{}{
			"secret_key":  "key1",
			"secret_name": "name1",
		}

		rt := NewTestDaprRuntime(modes.StandaloneMode)
		m := NewMockKubernetesStore()
		rt.secretStoresRegistry.Register(
			secretstores_loader.New("kubernetes", func() secretstores.SecretStore {
				return m
			}),
		)

		// add Kubernetes component manually
		rt.components = append(rt.components, components_v1alpha1.Component{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "kubernetes",
			},
			Spec: components_v1alpha1.ComponentSpec{
				Type: "secretstores.kubernetes",
			},
		})

		rt.initSecretStores()

		mod := rt.processComponentSecrets(mockBinding)
		assert.Equal(t, "value1", mod.Spec.Config["a"])
	})

	ts, _ := time.Parse(time.RFC3339Nano, "2002-10-02T10:00:00-05:00")

	tests := []struct {
		name     string
		config   map[string]interface{}
		expected interface{}
	}{
		{
			"Kubernetes Mode",
			map[string]interface{}{
				"secret_key":  "key1",
				"secret_name": "name1",
			},
			"value1",
		},
		{
			"Look up name only",
			map[string]interface{}{
				"secret_name": "name1",
			},
			"_value_data",
		},
		{
			"Look up integer value",
			map[string]interface{}{
				"secret_key":      "int_value",
				"secret_name":     "name1",
				"secret_datatype": "integer",
			},
			int64(12345),
		},
		{
			"Look up float value",
			map[string]interface{}{
				"secret_key":      "float_value",
				"secret_name":     "name1",
				"secret_datatype": "float",
			},
			float64(1.5),
		},
		{
			"Look up boolean value",
			map[string]interface{}{
				"secret_key":      "bool_value",
				"secret_name":     "name1",
				"secret_datatype": "boolean",
			},
			true,
		},
		{
			"Look up duration value",
			map[string]interface{}{
				"secret_key":      "duration_value",
				"secret_name":     "name1",
				"secret_datatype": "duration",
			},
			time.Duration(5 * time.Second),
		},
		{
			"Look up timestamp value",
			map[string]interface{}{
				"secret_key":      "timestamp_value",
				"secret_name":     "name1",
				"secret_datatype": "timestamp",
			},
			ts,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockBinding.Spec.Config["a"] = tt.config

			rt := NewTestDaprRuntime(modes.KubernetesMode)
			m := NewMockKubernetesStore()
			rt.secretStoresRegistry.Register(
				secretstores_loader.New("kubernetes", func() secretstores.SecretStore {
					return m
				}),
			)

			// initSecretStore appends Kubernetes component even if kubernetes component is not added
			err := rt.initSecretStores()
			assert.NoError(t, err)

			mod := rt.processComponentSecrets(mockBinding)
			assert.Equal(t, tt.expected, mod.Spec.Config["a"])
		})
	}
}

// Test InitSecretStore if secretstore.* refers to Kubernetes secret store
func TestInitSecretStoresInKubernetesMode(t *testing.T) {
	fakeSecretStoreWithAuth := components_v1alpha1.Component{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "fakeSecretStore",
		},
		Spec: components_v1alpha1.ComponentSpec{
			Type: "secretstores.fake.secretstore",
			Config: values_v1alpha1.Values{
				"a": map[string]interface{}{
					"secret_key":  "key1",
					"secret_name": "name1",
				},
				"b": "value2",
			},
		},
		Auth: components_v1alpha1.Auth{
			SecretStore: "kubernetes",
		},
	}

	rt := NewTestDaprRuntime(modes.KubernetesMode)
	rt.components = append(rt.components, fakeSecretStoreWithAuth)

	m := NewMockKubernetesStore()
	rt.secretStoresRegistry.Register(
		secretstores_loader.New("kubernetes", func() secretstores.SecretStore {
			return m
		}),
	)

	err := rt.initSecretStores()
	assert.NoError(t, err)
	assert.Equal(t, "value1", fakeSecretStoreWithAuth.Spec.Config["a"])
}

func TestOnNewPublishedMessage(t *testing.T) {
	testPubSubMessage := &pubsub.NewMessage{
		Topic: "topic1",
		Data:  []byte("Test Message"),
	}

	expectedRequest := &channel.InvokeRequest{
		Method:   testPubSubMessage.Topic,
		Payload:  testPubSubMessage.Data,
		Metadata: map[string]string{http_channel.HTTPVerb: http_channel.Post, http_channel.ContentType: pubsub.ContentType},
	}

	rt := NewTestDaprRuntime(modes.StandaloneMode)

	t.Run("succeeded to publish message to user app", func(t *testing.T) {
		mockAppChannel := new(channelt.MockAppChannel)
		rt.appChannel = mockAppChannel

		fakeHTTPResponse := &channel.InvokeResponse{
			Metadata: map[string]string{http_channel.HTTPStatusCode: "200"},
			Data:     []byte("OK"),
		}

		mockAppChannel.On("InvokeMethod", expectedRequest).Return(fakeHTTPResponse, nil)

		// act
		err := rt.publishMessageHTTP(testPubSubMessage)

		// assert
		assert.Nil(t, err)
		mockAppChannel.AssertNumberOfCalls(t, "InvokeMethod", 1)
	})

	t.Run("failed to publish message to user app", func(t *testing.T) {
		mockAppChannel := new(channelt.MockAppChannel)
		rt.appChannel = mockAppChannel

		clientError := errors.New("Internal Error")

		fakeHTPResponse := &channel.InvokeResponse{
			Metadata: map[string]string{http_channel.HTTPStatusCode: "500"},
			Data:     []byte(clientError.Error()),
		}

		expectedClientError := fmt.Errorf("error from app consumer: Internal Error")

		mockAppChannel.On("InvokeMethod", expectedRequest).Return(fakeHTPResponse, clientError)

		// act
		err := rt.publishMessageHTTP(testPubSubMessage)

		// assert
		assert.Equal(t, expectedClientError, err)
		mockAppChannel.AssertNumberOfCalls(t, "InvokeMethod", 1)
	})
}

func getFakeProperties() map[string]string {
	return map[string]string{
		"host":       "localhost",
		"password":   "fakePassword",
		"consumerID": TestRuntimeConfigID,
	}
}

func getFakeConfigValues() values_v1alpha1.Values {
	return values_v1alpha1.Values{
		"host":       "localhost",
		"password":   "fakePassword",
		"consumerID": TestRuntimeConfigID,
	}
}

func NewTestDaprRuntime(mode modes.DaprMode) *DaprRuntime {
	testRuntimeConfig := NewRuntimeConfig(
		TestRuntimeConfigID,
		"10.10.10.12",
		"10.10.10.11",
		DefaultAllowedOrigins,
		"globalConfig",
		DefaultComponentsPath,
		string(HTTPProtocol),
		string(mode),
		DefaultDaprHTTPPort,
		DefaultDaprGRPCPort,
		1024,
		DefaultProfilePort,
		false,
		-1)

	rt := NewDaprRuntime(testRuntimeConfig, &config.Configuration{})
	rt.components = []components_v1alpha1.Component{
		{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "Components",
			},
			Spec: components_v1alpha1.ComponentSpec{
				Type:   "pubsub.mockPubSub",
				Config: getFakeConfigValues(),
			},
		},
	}

	return rt
}
