package handlers

import (
	"context"
	"fmt"
	"time"

	components_v1alpha1 "github.com/dapr/dapr/pkg/apis/components/v1alpha1"
	values_v1alpha1 "github.com/dapr/dapr/pkg/apis/values/v1alpha1"
	pb "github.com/dapr/dapr/pkg/proto/daprinternal"
	pb_values "github.com/dapr/dapr/pkg/proto/values"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// ComponentsHandler handles the lifetime management of Component CRDs
type ComponentsHandler struct {
	kubeClient kubernetes.Interface
}

// NewComponentsHandler returns a new component handler
func NewComponentsHandler(client kubernetes.Interface) *ComponentsHandler {
	return &ComponentsHandler{
		kubeClient: client,
	}
}

// Init performs any startup tasks needed
func (c *ComponentsHandler) Init() error {
	return nil
}

// ObjectUpdated handles updated crd operations
func (c *ComponentsHandler) ObjectUpdated(old interface{}, new interface{}) {
}

// ObjectDeleted handles deleted crd operations
func (c *ComponentsHandler) ObjectDeleted(obj interface{}) {
	log.Info("notified about a component delete")
}

// ObjectCreated handles created crd operations
func (c *ComponentsHandler) ObjectCreated(obj interface{}) {
	log.Info("notified about a component update")

	component := obj.(*components_v1alpha1.Component)
	err := c.publishComponentToDaprRuntimes(component)
	if err != nil {
		log.Errorf("error from ObjectCreated: %s", err)
	}
}

func (c *ComponentsHandler) publishComponentToDaprRuntimes(component *components_v1alpha1.Component) error {
	payload := pb.Component{
		Auth: &pb.ComponentAuth{
			SecretStore: component.Auth.SecretStore,
		},
		Metadata: &pb.ComponentMetadata{
			Name:      component.ObjectMeta.Name,
			Namespace: component.GetNamespace(),
		},
		Spec: &pb.ComponentSpec{
			Type: component.Spec.Type,
		},
	}

	payload.Spec.Config = convertValues(component.Spec.Config)

	services, err := c.kubeClient.CoreV1().Services(meta_v1.NamespaceAll).List(meta_v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{daprEnabledAnnotationKey: "true"}).String(),
	})
	if err != nil {
		return err
	}

	for _, s := range services.Items {
		svcName := s.GetName()

		log.Infof("updating dapr pod selected by service: %s", svcName)
		endpoints, err := c.kubeClient.CoreV1().Endpoints(s.GetNamespace()).Get(svcName, meta_v1.GetOptions{})
		if err != nil {
			log.Errorf("error getting endpoints for service %s: %s", svcName, err)
			continue
		}
		go c.publishComponentToService(payload, endpoints)
	}

	return nil
}

func (c *ComponentsHandler) publishComponentToService(component pb.Component, endpoints *corev1.Endpoints) {
	if endpoints != nil && len(endpoints.Subsets) > 0 {
		for _, a := range endpoints.Subsets[0].Addresses {
			address := fmt.Sprintf("%s:%s", a.IP, fmt.Sprintf("%v", daprSidecarGRPCPort))
			go c.updateDaprRuntime(component, address)
		}
	}
}

func (c *ComponentsHandler) updateDaprRuntime(component pb.Component, address string) {
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Errorf("gRPC connection failure: %s", err)
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	client := pb.NewDaprInternalClient(conn)
	_, err = client.UpdateComponent(ctx, &component)
	if err != nil {
		log.Warnf("error updating Dapr Runtime with component: %s", err)
	}
}

func convertValues(values values_v1alpha1.Values) *pb_values.Values {
	if values == nil {
		return nil
	}

	var vals pb_values.Values
	for k, v := range values {
		switch v := v.(type) {
		case string:
			vals.ValueMap[k] = &pb_values.Values_Value{
				Value: &pb_values.Values_Value_StringValue{StringValue: v},
			}
		case []string:
			vals.ValueMap[k] = &pb_values.Values_Value{
				Value: &pb_values.Values_Value_StringList{StringList: &pb_values.Values_StringList{Items: v}},
			}
		case int64:
			vals.ValueMap[k] = &pb_values.Values_Value{
				Value: &pb_values.Values_Value_Int64Value{Int64Value: v},
			}
		case []int64:
			vals.ValueMap[k] = &pb_values.Values_Value{
				Value: &pb_values.Values_Value_Int64List{Int64List: &pb_values.Values_Int64List{Items: v}},
			}
		case float64:
			vals.ValueMap[k] = &pb_values.Values_Value{
				Value: &pb_values.Values_Value_DoubleValue{DoubleValue: v},
			}
		case []float64:
			vals.ValueMap[k] = &pb_values.Values_Value{
				Value: &pb_values.Values_Value_DoubleList{DoubleList: &pb_values.Values_DoubleList{Items: v}},
			}
		case bool:
			vals.ValueMap[k] = &pb_values.Values_Value{
				Value: &pb_values.Values_Value_BoolValue{BoolValue: v},
			}
		case []bool:
			vals.ValueMap[k] = &pb_values.Values_Value{
				Value: &pb_values.Values_Value_BoolList{BoolList: &pb_values.Values_BoolList{Items: v}},
			}
		case []byte:
			vals.ValueMap[k] = &pb_values.Values_Value{
				Value: &pb_values.Values_Value_BytesValue{BytesValue: v},
			}

			// TODO: Convery time.Duration & time.Time
			// Use gogo?
			// case time.Time:
			// 	vals.ValueMap[k] = &pb_values.Values_Value{
			// 		Value: &pb_values.Values_Value_TimestampValue{TimestampValue: v},
			// 	}
			// case []time.Time:
			// 	vals.ValueMap[k] = &pb_values.Values_Value{
			// 		Value: &pb_values.Values_Value_TimestampList{TimestampList: &pb_values.Values_TimestampList{Items: v}},
			// 	}
			// case time.Duration:
			// 	vals.ValueMap[k] = &pb_values.Values_Value{
			// 		Value: &pb_values.Values_Value_DurationValue{DurationValue: v},
			// 	}
			// case []time.Duration:
			// 	vals.ValueMap[k] = &pb_values.Values_Value{
			// 		Value: &pb_values.Values_Value_DurationList{DurationList: &pb_values.Values_DurationList{Items: v}},
			// 	}
		}
	}
	return &vals
}
