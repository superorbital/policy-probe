package api

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestSuite struct {
	Spec TestSuiteSpec `json:"spec,omitempty"`
}
type TestSuiteSpec struct {
	TestCases []TestCaseSpec `json:"test_cases,omitempty"`
}

type TestCaseSpec struct {
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
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
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

type Destination interface{}
type Source interface {
	AssertReachable(to Destination)
	AssertUnreachable(to Destination)
	Config() *SourceConfig
}

type ProtocolType string

const (
	TCPProtocolType ProtocolType = "tcp"
	UDPProtocolType ProtocolType = "udp"
)

type SourceConfig struct {
	Address  string
	Port     int
	Protocol ProtocolType
	Message  string
}

type DestinationConfig struct {
	Address  string
	Port     int
	Protocol ProtocolType
	Message  string
}
