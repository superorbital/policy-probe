package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestSuite struct {
	Spec TestSuiteSpec `json:"spec,omitempty"`
}
type TestSuiteSpec struct {
	TestCases  []TestCase `json:"test_cases,omitempty"`
	ProbeImage string     `json:"probe_image,omitempty"`
}

type TestCase struct {
	Description string      `json:"description,omitempty"`
	Expect      ExpectType  `json:"expect,omitempty"`
	From        FromKinds   `json:"from,omitempty"`
	To          ProbeConfig `json:"to,omitempty"`
}

type FromKinds struct {
	Deployment *ObjectSpec `json:"deployment,omitempty"`
}

type ExpectType string

const (
	PassExpectType ExpectType = "Pass"
	FailExpectType ExpectType = "Fail"
)

type ObjectSpec struct {
	Namespace string
	Name      *string
	*metav1.LabelSelector
	networkingv1.NetworkPolicyPort
}

type Sink interface {
	Delete() error
}

type Source interface {
	AssertReachable(context.Context) error
	AssertUnreachable(context.Context) error
}

type ProtocolType string

func (p ProtocolType) String() string {
	return string(p)
}

const (
	TCPProtocolType ProtocolType = "tcp"
	UDPProtocolType ProtocolType = "udp"
)

type ProbeConfig struct {
	Address  string        `json:"address,omitempty" mapstructure:",omitempty"`
	Protocol ProtocolType  `json:"protocol,omitempty" mapstructure:",omitempty"`
	Message  string        `json:"message,omitempty" mapstructure:",omitempty"`
	Interval time.Duration `json:"interval,omitempty" mapstructure:",omitempty"`
}

func (p ProbeConfig) ToEnv() []v1.EnvVar {
	result := &map[string]interface{}{}
	err := mapstructure.Decode(p, &result)
	if err != nil {
		zap.L().Fatal("failed to decode", zap.Error(err))
	}
	var env []v1.EnvVar
	for k, v := range *result {
		k := strings.ToUpper(k)
		switch v := v.(type) {
		case string:
			env = append(env, v1.EnvVar{
				Name:  k,
				Value: v,
			})
		case fmt.Stringer:
			env = append(env, v1.EnvVar{
				Name:  k,
				Value: v.String(),
			})
		default:
			zap.L().Fatal("unknown type", zap.String("key", k), zap.String("type", fmt.Sprintf("%T", v)))
		}
	}

	return env
}
