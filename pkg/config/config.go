package config

import (
	"k8s.io/apimachinery/pkg/types"
)

type Probe struct {
	Expect string
	From   FromKinds
	To     ToKinds
}

type FromKinds struct {
	Deployment *types.NamespacedName
	Probe      *ProbeSpec
}

type ToKinds struct {
	Deployment *types.NamespacedName
	Pod        *types.NamespacedName
	Probe      *ProbeSpec
	Server     *ServerSpec
	Service    *types.NamespacedName
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
