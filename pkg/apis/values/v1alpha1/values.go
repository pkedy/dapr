package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen=false
type Values map[string]interface{}

func (in *Values) DeepCopyInto(out *Values) {
	if in == nil {
		*out = nil
	} else {
		*out = runtime.DeepCopyJSON(*in)
	}
}

func (in *Values) DeepCopy() *Values {
	if in == nil {
		return nil
	}
	out := new(Values)
	in.DeepCopyInto(out)
	return out
}
