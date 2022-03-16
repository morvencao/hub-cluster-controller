package cluster

import (
	"context"
	"encoding/json"

	appv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

const MULTICLUSTER_CHANNEL_NAME = "multicluster-operators-channel"

func createChannelManagedClusterView(namespace string) *unstructured.Unstructured {

	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "view.open-cluster-management.io/v1beta1",
		"kind":       "ManagedClusterView",
		"metadata": map[string]interface{}{
			"name":      MULTICLUSTER_CHANNEL_NAME,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"scope": map[string]interface{}{
				"name":      MULTICLUSTER_CHANNEL_NAME,
				"namespace": "open-cluster-management",
				"resource":  "deployments",
			},
		},
	}}
}

func ApplyChannelManagedClusterView(ctx context.Context,
	dynamicClient dynamic.Interface, managedClusterName string) (*unstructured.Unstructured, error) {

	crResource := schema.GroupVersionResource{
		Group:    "view.open-cluster-management.io",
		Version:  "v1beta1",
		Resource: "managedclusterviews"}
	desiredChannel := createChannelManagedClusterView(managedClusterName)

	existingChannel, err := dynamicClient.Resource(crResource).
		Namespace(managedClusterName).
		Get(ctx, MULTICLUSTER_CHANNEL_NAME, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		klog.V(2).Infof("creating multicluster-operators-channel managedclusterviews in %s namespace", managedClusterName)
		existingChannel, err = dynamicClient.Resource(crResource).
			Namespace(managedClusterName).
			Create(ctx, desiredChannel, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}

	updated, err := ensureManagedClusterView(existingChannel, desiredChannel)
	if err != nil {
		return nil, err
	}
	if updated {
		existingChannel, err = dynamicClient.Resource(crResource).
			Namespace(managedClusterName).
			Update(ctx, desiredChannel, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
	}

	return existingChannel, nil
}

func ensureManagedClusterView(existing, desired *unstructured.Unstructured) (bool, error) {
	// compare the ManagedClusterView
	existingBytes, err := json.Marshal(existing.Object["spec"])
	if err != nil {
		return false, err
	}
	desiredBytes, err := json.Marshal(desired.Object["spec"])
	if err != nil {
		return false, err
	}
	if string(existingBytes) != string(desiredBytes) {
		return true, nil
	}
	return false, nil
}

func isChannelReady(channel *unstructured.Unstructured) (bool, error) {
	if channel == nil {
		return false, nil
	}
	statusObj := channel.Object["status"]
	if statusObj == nil {
		return false, nil
	}
	resultObj := statusObj.(map[string]interface{})["result"]
	if resultObj == nil {
		return false, nil
	}

	jsonStr, err := json.Marshal(resultObj)
	if err != nil {
		return false, err
	}
	var deploy (*appv1.Deployment)
	err = json.Unmarshal(jsonStr, &deploy)
	if err != nil {
		return false, err
	}
	return deploy.Status.ReadyReplicas == deploy.Status.Replicas, nil
}