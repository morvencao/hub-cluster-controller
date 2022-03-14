package packagemanifest

import (
	"context"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// packageManifestController reconciles packagemanifests.packages.operators.coreos.com.
type packageManifestController struct {
	cache         resourceapply.ResourceCache
	dynamicClient dynamic.Interface
	eventRecorder events.Recorder
}

type packageManifest struct {
}

// NewPackageManifestController creates a new package controller
func NewPackageManifestController(
	dynamicClient dynamic.Interface,
	packageGenericInformer informers.GenericInformer,
	recorder events.Recorder) factory.Controller {
	c := &packageManifestController{
		dynamicClient: dynamicClient,
		cache:         resourceapply.NewResourceCache(),
		eventRecorder: recorder.WithComponentSuffix("package-manifest-controller"),
	}
	return factory.New().
		WithFilteredEventsInformersQueueKeyFunc(
			func(obj runtime.Object) string {
				key, _ := cache.MetaNamespaceKeyFunc(obj)
				return key
			},
			func(obj interface{}) bool {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err != nil {
					return false
				}
				_, name, err := cache.SplitMetaNamespaceKey(key)
				if err != nil {
					// ignore addon whose key is not in format: namespace/name
					return false
				}
				// only enqueue when the name is advanced-cluster-management
				if name == "advanced-cluster-management" {
					return true
				}
				return false
			}, packageGenericInformer.Informer()).
		WithSync(c.sync).
		ToController("PackageManifestController", recorder)
}

func (c *packageManifestController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		// ignore addon whose key is not in format: namespace/name
		return err
	}
	klog.V(2).Infof("Reconciling for packagemanifest %s in namespace %s", name, namespace)

	obj, err := c.dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "packages.operators.coreos.com",
		Version:  "v1",
		Resource: "packagemanifests"}).
		Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	statusObj := obj.Object["status"].(map[string]interface{})

	if statusObj["catalogSource"].(string) != "redhat-operators" {
		return nil
	}

	defaultChannel := statusObj["defaultChannel"].(string)
	klog.V(2).Infof("the defaultChannel is %s", defaultChannel)

	currentCSV := ""
	channels := statusObj["channels"].([]interface{})

	for _, channel := range channels {
		if channel.(map[string]interface{})["name"].(string) == defaultChannel {
			currentCSV = channel.(map[string]interface{})["currentCSV"].(string)
		}
	}
	klog.V(2).Infof("the currentCSV is %s", currentCSV)

	//the PackageManifest is changed, need to store this new value
	SetPackageManifest(&PackageManifest{
		DefaultChannel: defaultChannel,
		CurrentCSV:     currentCSV,
	})
	return nil
}
