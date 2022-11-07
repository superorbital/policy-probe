package api

import (
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestSuite struct {
	Spec TestSuiteSpec `json:"spec,omitempty"`
}
type TestSuiteSpec struct {
	TestCases []TestCase `json:"test_cases,omitempty"`
}

type TestCase struct {
	Description string     `json:"description,omitempty"`
	Expect      ExpectType `json:"expect,omitempty"`
	From        FromKinds  `json:"from,omitempty"`
	To          ToKinds    `json:"to,omitempty"`
}

type FromKinds struct {
	Probe *ProbeSpec `json:"probe,omitempty"`
}

type ToKinds struct {
	Probe   *ProbeSpec `json:"probe,omitempty"`
	Service *ObjectSpec
	Server  *ServerSpec
}

type ExpectType string

const (
	PassExpectType ExpectType = "Pass"
	FailExpectType ExpectType = "Fail"
)

type Probe struct {
	Expect string
	From   FromKinds
	To     ToKinds
}

type ProbeSpec struct {
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Source      *ProbeConfig      `json:"source,omitempty"`
}

type ServerSpec struct {
	Host     string
	Port     int
	Protocol string
}

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
	AssertReachable()
	AssertUnreachable()
	Config() *ProbeConfig
	Delete() error
}

type ProtocolType string

const (
	TCPProtocolType ProtocolType = "tcp"
	UDPProtocolType ProtocolType = "udp"
)

type ProbeConfig struct {
	Address  string        `json:"address,omitempty"`
	Port     int           `json:"port,omitempty"`
	Protocol ProtocolType  `json:"protocol,omitempty"`
	Message  string        `json:"message,omitempty"`
	Interval time.Duration `json:"interval,omitempty"`
}
