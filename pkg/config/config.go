package config

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Probe struct {
	Expect string
	From   FromKinds
	To     ToKinds
}

type FromKinds struct {
	Deployment *ObjectSpec
	Pod        *ObjectSpec
	Probe      *ProbeSpec
}

type ToKinds struct {
	Deployment *ObjectSpec
	Pod        *ObjectSpec
	Probe      *ProbeSpec
	Server     *ServerSpec
	Service    *ObjectSpec
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
