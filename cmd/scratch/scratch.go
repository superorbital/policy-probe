package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func do() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)

	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to create client config: %w", err)
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return fmt.Errorf("failed to create clienset: %w", err)
	}
	ctx := context.Background()
	pod, err := clientset.CoreV1().Pods("default").Get(ctx, "nginx-deployment-7fb96c846b-vqhnf", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod: %w", err)
	}

	podJS, err := json.Marshal(pod)
	if err != nil {
		return fmt.Errorf("error creating JSON for pod: %w", err)
	}

	copied := pod.DeepCopy()
	copied.Spec.EphemeralContainers = []v1.EphemeralContainer{}
	debugJS, err := json.Marshal(copied)
	if err != nil {
		return fmt.Errorf("error creating JSON for debug container: %w", err)
	}
	patch, err := strategicpatch.CreateTwoWayMergePatch(podJS, debugJS, pod)
	if err != nil {
		return fmt.Errorf("error creating patch to add debug container: %w", err)
	}
	_, err = clientset.CoreV1().Pods(pod.Namespace).Patch(ctx, pod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")
	if err != nil {
		return fmt.Errorf("error adding ephemeral container to pod: %w", err)
	}

	log.Println("did the patch!@")

	return nil
}

func main() {
	if err := do(); err != nil {
		log.Fatal(err)
	}
}
